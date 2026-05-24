//go:build integration

// Package integration holds end-to-end tests that drive Harbormaster's
// domain processors against a real MinIO server spun up via
// testcontainers-go. The whole package is gated behind the `integration`
// build tag so the default `go test ./...` invocation excludes these
// files (they require Docker and add ~30s of container-startup latency
// to every run).
//
// Invocation:
//
//	HARBORMASTER_INTEGRATION=1 go test -tags=integration \
//	    -count=1 ./internal/integration/...
//
// The HARBORMASTER_INTEGRATION=1 environment variable is the
// belt-and-suspenders gate: even when the build tag is set, the tests
// skip themselves unless the env var is also present. This keeps an
// accidental `go test -tags=integration ./...` invocation in a CI
// pipeline without Docker from failing noisily — instead the tests skip
// with a clear "set HARBORMASTER_INTEGRATION=1 to enable" message.
//
// If Docker is unreachable (or the testcontainers reaper cannot start),
// setup() calls t.Skipf so the tests skip cleanly rather than failing.
package integration

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/db"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	"github.com/jtumidanski/Harbormaster/internal/lifecycle"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
	"github.com/jtumidanski/Harbormaster/internal/objects"
)

// envEnable is the env-var gate. Tests skip themselves when this is unset.
const envEnable = "HARBORMASTER_INTEGRATION"

// minioImage is the pinned MinIO release. Pinning prevents a surprise
// CI failure when MinIO ships a backwards-incompatible admin-API tweak;
// bump deliberately and re-run the suite. The tag is one of the
// quay.io/minio/minio "RELEASE.<timestamp>" rolling tags.
const minioImage = "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z"

// TestEnv bundles the live MinIO clients and the wired-up domain
// processors a test needs. Each *_integration_test.go file calls setup()
// and uses the returned env to drive a happy-path scenario.
type TestEnv struct {
	Pool *hmminio.Pool

	// MC and Adm are exposed for the rare test that needs to assert
	// MinIO-side state directly (e.g. confirming an object was actually
	// removed); domain logic should still run through Buckets/Objects/etc.
	MC  *miniogo.Client
	Adm *madmin.AdminClient

	Buckets   *buckets.Processor
	Objects   *objects.Processor
	Lifecycle *lifecycle.Processor
	Empty     *bucketempty.Service
	Audit     *audit.Processor
	DB        *gorm.DB
}

// setup boots a fresh MinIO testcontainer, opens a temp SQLite DB for
// audit/job rows, builds the live domain processors against the pool,
// and registers cleanup hooks. Tests skip via t.Skipf when:
//
//   - the HARBORMASTER_INTEGRATION env var is unset, or
//   - the testcontainer fails to start (typically: Docker not
//     reachable, no docker.sock, or the reaper image cannot be pulled).
//
// The returned context inherits a 5-minute deadline so a runaway test
// cannot block CI forever.
func setup(t *testing.T) (*TestEnv, context.Context) {
	t.Helper()

	if os.Getenv(envEnable) == "" {
		t.Skipf("integration tests gated by %s=1; skipping", envEnable)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	container, err := tcminio.Run(ctx, minioImage)
	if err != nil {
		t.Skipf("MinIO testcontainer unavailable (Docker not reachable?): %v", err)
	}
	t.Cleanup(func() {
		// Use a fresh context so cleanup runs even when the test's
		// context has already been cancelled.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Terminate(stopCtx)
	})

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("get MinIO connection string: %v", err)
	}
	// container.ConnectionString returns "host:port" on this module
	// version; normalise to a full http URL so hmminio.Pool's URL
	// parser accepts it.
	rawURL := endpoint
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	if _, err := url.Parse(rawURL); err != nil {
		t.Fatalf("invalid endpoint URL %q: %v", rawURL, err)
	}

	pool := hmminio.NewEmpty()
	if err := pool.Rebuild(hmminio.Credentials{
		EndpointURL: rawURL,
		AccessKey:   container.Username,
		SecretKey:   container.Password,
	}); err != nil {
		t.Fatalf("pool.Rebuild: %v", err)
	}
	adm, mc, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("pool.Get: %v", err)
	}

	// Audit + jobs DB: a fresh per-test SQLite file under t.TempDir so
	// runs are isolated and the file gets cleaned up automatically. The
	// PRAGMAs and the MaxOpenConns=1 clamp mirror internal/db.Open so the
	// integration suite uses the same single-writer posture production
	// does (which avoids spurious "database is locked" / "disk I/O error"
	// failures under concurrent goroutines).
	dbPath := filepath.Join(t.TempDir(), "harbormaster-integration.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)",
		dbPath,
	)
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sdb, err := gdb.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sdb.SetMaxOpenConns(1)
	sdb.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sdb.Close() })
	if err := db.Migrate(gdb); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	auditProc := audit.NewProcessor(gdb, 90*24*time.Hour)

	// bucketempty wiring mirrors cmd/harbormaster/audit_adapter.go so
	// the worker emits the same audit shape the production server does.
	emptyAudit := integrationBucketEmptyAudit{p: auditProc}
	emptyService := bucketempty.New(gdb, pool, emptyAudit)

	lifecycleProc := lifecycle.NewProcessor(newLifecycleClientGetter(pool)).WithAudit(auditProc)

	bucketProc := buckets.NewProcessor(newBucketClientGetter(pool)).
		WithAudit(auditProc).
		WithLifecycle(integrationLifecycleAdapter{lc: lifecycleProc})

	objectsProc := objects.NewProcessor(newObjectClientGetter(pool), objects.ProcessorConfig{
		UploadMaxBytes:    100 * 1024 * 1024,
		ShareLinkMaxTTL:   7 * 24 * time.Hour,
		DownloadProxyMode: "proxy",
	}).WithAudit(auditProc)

	return &TestEnv{
		Pool:      pool,
		MC:        mc,
		Adm:       adm,
		Buckets:   bucketProc,
		Objects:   objectsProc,
		Lifecycle: lifecycleProc,
		Empty:     emptyService,
		Audit:     auditProc,
		DB:        gdb,
	}, ctx
}

// integrationBucketEmptyAudit mirrors cmd/harbormaster.bucketEmptyAuditAdapter
// so the bucketempty service can emit audit rows through the same
// audit.Processor the rest of the test wiring uses.
type integrationBucketEmptyAudit struct {
	p *audit.Processor
}

// Record satisfies bucketempty.AuditRecorder.
func (a integrationBucketEmptyAudit) Record(ctx context.Context, action, target, outcome string,
	payload map[string]any, errMsg string,
) {
	if a.p == nil {
		return
	}
	_ = a.p.Record(ctx, audit.Event{
		Action:         action,
		TargetType:     "bucket",
		TargetID:       target,
		Outcome:        outcome,
		ErrorMessage:   errMsg,
		PayloadSummary: payload,
	})
}

// integrationBucketAdmin wraps a *madmin.AdminClient so it satisfies the
// buckets.AdminClient interface (BucketUsageInfo is synthesised from
// DataUsageInfo, just as cmd/harbormaster.bucketAdminAdapter does).
type integrationBucketAdmin struct {
	*madmin.AdminClient
}

// BucketUsageInfo returns the usage row for bucket, or the zero value
// when the scanner has not seen the bucket yet.
func (a integrationBucketAdmin) BucketUsageInfo(ctx context.Context, bucket string) (madmin.BucketUsageInfo, error) {
	info, err := a.AdminClient.DataUsageInfo(ctx)
	if err != nil {
		return madmin.BucketUsageInfo{}, err
	}
	return info.BucketsUsage[bucket], nil
}

// newBucketClientGetter mirrors cmd/harbormaster.newBucketClientGetter.
func newBucketClientGetter(pool *hmminio.Pool) buckets.ClientGetter {
	return buckets.NewClientGetter(func(ctx context.Context) (buckets.AdminClient, buckets.S3Client, error) {
		madm, mc, err := pool.Get(ctx)
		if err != nil {
			return nil, nil, err
		}
		return integrationBucketAdmin{AdminClient: madm}, mc, nil
	})
}

// integrationObjectS3 mirrors cmd/harbormaster.objectS3Adapter, wrapping
// a *miniogo.Client so ListObjectsV2 routes through miniogo.Core.
type integrationObjectS3 struct {
	*miniogo.Client
}

// ListObjectsV2 routes through miniogo.Core because Client.ListObjects
// hides the continuation token.
func (a integrationObjectS3) ListObjectsV2(bucket, prefix, startAfter, continuationToken, delimiter string, maxKeys int) (miniogo.ListBucketV2Result, error) {
	core := miniogo.Core{Client: a.Client}
	return core.ListObjectsV2(bucket, prefix, startAfter, continuationToken, delimiter, maxKeys)
}

// GetObject narrows *miniogo.Object's return type to io.ReadCloser, the
// shape objects.S3Client expects.
func (a integrationObjectS3) GetObject(ctx context.Context, bucket, object string, opts miniogo.GetObjectOptions) (io.ReadCloser, error) {
	return a.Client.GetObject(ctx, bucket, object, opts)
}

// newObjectClientGetter mirrors cmd/harbormaster.newObjectClientGetter.
func newObjectClientGetter(pool *hmminio.Pool) objects.ClientGetter {
	return objects.NewClientGetter(func(ctx context.Context) (objects.S3Client, error) {
		_, mc, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return integrationObjectS3{Client: mc}, nil
	})
}

// integrationLifecycleS3 mirrors cmd/harbormaster.lifecycleS3Adapter.
type integrationLifecycleS3 struct {
	*miniogo.Client
}

// newLifecycleClientGetter mirrors cmd/harbormaster.newLifecycleClientGetter.
func newLifecycleClientGetter(pool *hmminio.Pool) lifecycle.ClientGetter {
	return lifecycle.NewClientGetter(func(ctx context.Context) (lifecycle.S3Client, error) {
		_, mc, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return integrationLifecycleS3{Client: mc}, nil
	})
}

// integrationLifecycleAdapter mirrors cmd/harbormaster.bucketLifecycleAdapter.
type integrationLifecycleAdapter struct {
	lc *lifecycle.Processor
}

// Create satisfies buckets.LifecycleCreator.
func (a integrationLifecycleAdapter) Create(ctx context.Context, bucket string, days int, prefix string) error {
	if a.lc == nil {
		return nil
	}
	_, err := a.lc.Create(ctx, bucket, days, prefix, "", "")
	return err
}
