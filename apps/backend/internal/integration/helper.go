//go:build integration

// Package integration holds end-to-end tests that drive Harbormaster's
// domain processors against a real, per-test MinIO server. The whole
// package is gated behind the `integration` build tag so the default
// `go test ./...` invocation excludes these files.
//
// Invocation:
//
//	HARBORMASTER_INTEGRATION=1 go test -tags=integration \
//	    -count=1 ./internal/integration/...
//
// The HARBORMASTER_INTEGRATION=1 environment variable is the
// belt-and-suspenders gate: even when the build tag is set, the tests
// skip themselves unless the env var is also present. This keeps an
// accidental `go test -tags=integration ./...` invocation from needing a
// MinIO at all — the tests skip with a clear "set HARBORMASTER_INTEGRATION=1
// to enable" message.
//
// Each test gets its own MinIO, from one of two sources:
//
//   - HARBORMASTER_MINIO_BINARY set: a `minio server` process started from
//     that binary on a free loopback port with a fresh temp data dir. No
//     Docker needed; this is what the nightly workflow uses, because the
//     Forgejo runners have no Docker daemon.
//   - otherwise: a testcontainers-go MinIO container (needs Docker), image
//     from HARBORMASTER_MINIO_IMAGE or defaultMinIOImage.
//
// Once HARBORMASTER_INTEGRATION=1 is set, a MinIO that fails to start
// FAILS the test. It used to skip, which let the nightly report green
// with every test skipped for as long as its image was unpullable.
package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	madmin "github.com/minio/madmin-go/v4"
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
	"github.com/jtumidanski/Harbormaster/internal/policies"
	"github.com/jtumidanski/Harbormaster/internal/users"
)

// envEnable is the env-var gate. Tests skip themselves when this is unset.
const envEnable = "HARBORMASTER_INTEGRATION"

// envMinIOImage lets the nightly matrix (or a local operator) override
// the pinned MinIO image without editing source. Falls back to the
// default constant below when unset.
const envMinIOImage = "HARBORMASTER_MINIO_IMAGE"

// envMinIOBinary, when set, is the path to a `minio server`-compatible
// binary. setup() runs it as a local process instead of a testcontainer.
const envMinIOBinary = "HARBORMASTER_MINIO_BINARY"

// defaultMinIOImage is the pinned MinIO release. Pinning prevents a
// surprise CI failure when MinIO ships a backwards-incompatible
// admin-API tweak; bump deliberately and re-run the suite. The tag is
// one of the pgsty/minio "RELEASE.<timestamp>" tags: MinIO no longer
// publishes pullable images (quay.io/minio/minio and docker.io/minio/minio
// both 401 anonymous pulls), and pgsty/minio is the community fork that
// still cuts releases. The nightly workflow does not use this image; it
// runs binaries via HARBORMASTER_MINIO_BINARY.
const defaultMinIOImage = "pgsty/minio:RELEASE.2026-08-04T00-00-00Z"

// processRootUser / processRootPassword are the root credentials a
// binary-mode server is started with. Throwaway: the server listens on
// loopback only and its data dir is deleted with the test.
const (
	processRootUser     = "harbormaster"
	processRootPassword = "harbormaster-integration"
)

// minioImageFor returns the image the testcontainer should run. The
// HARBORMASTER_MINIO_IMAGE env var wins so a local operator can try
// another release against the same suite; otherwise the default constant
// is used.
func minioImageFor() string {
	if v := os.Getenv(envMinIOImage); v != "" {
		return v
	}
	return defaultMinIOImage
}

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

	Buckets         *buckets.Processor
	Objects         *objects.Processor
	Lifecycle       *lifecycle.Processor
	Empty           *bucketempty.Service
	Audit           *audit.Processor
	Users           *users.Processor
	ServiceAccounts *users.ServiceAccountProcessor
	PolicyMat       *policies.Materializer
	DB              *gorm.DB
}

// minioServer is a running per-test MinIO: its endpoint URL and root
// credentials.
type minioServer struct {
	EndpointURL string
	AccessKey   string
	SecretKey   string
}

// startMinIO starts a per-test MinIO from the configured source (see the
// package doc) and registers its teardown. It fails the test if the
// server cannot be started.
func startMinIO(ctx context.Context, t *testing.T) minioServer {
	t.Helper()
	if bin := os.Getenv(envMinIOBinary); bin != "" {
		return startMinIOProcess(ctx, t, bin)
	}
	return startMinIOContainer(ctx, t, minioImageFor())
}

// startMinIOContainer runs image via testcontainers-go.
func startMinIOContainer(ctx context.Context, t *testing.T, image string) minioServer {
	t.Helper()

	container, err := tcminio.Run(ctx, image)
	if err != nil {
		t.Fatalf("start MinIO testcontainer %q (Docker not reachable? set %s to run without Docker): %v",
			image, envMinIOBinary, err)
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
	return minioServer{EndpointURL: rawURL, AccessKey: container.Username, SecretKey: container.Password}
}

// startMinIOProcess runs `bin server <tempdir>` on a free loopback port and
// waits for /minio/health/ready. The server's combined output goes to a file
// in the test's temp dir and is included in the failure message if it never
// becomes ready.
func startMinIOProcess(ctx context.Context, t *testing.T, bin string) minioServer {
	t.Helper()

	addr := freeLoopbackAddr(t)
	dataDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "minio.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create MinIO log file: %v", err)
	}

	cmd := exec.Command(bin, "server", dataDir, "--address", addr, "--quiet")
	cmd.Env = append(os.Environ(),
		"MINIO_ROOT_USER="+processRootUser,
		"MINIO_ROOT_PASSWORD="+processRootPassword,
		"MINIO_BROWSER=off",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start MinIO process %q: %v", bin, err)
	}
	// exited is closed once the process has been reaped; waitErr is safe to
	// read after that. A closed channel (rather than a sent value) lets both
	// waitMinIOReady and the cleanup below observe the exit.
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	// Registered after t.TempDir, so it runs first: the process is gone
	// before its data dir is removed.
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-exited
		_ = logFile.Close()
	})

	endpoint := "http://" + addr
	if err := waitMinIOReady(ctx, endpoint, exited, func() error { return waitErr }); err != nil {
		out, _ := os.ReadFile(logPath)
		t.Fatalf("MinIO process %q never became ready at %s: %v\n--- server output ---\n%s", bin, endpoint, err, out)
	}
	return minioServer{EndpointURL: endpoint, AccessKey: processRootUser, SecretKey: processRootPassword}
}

// freeLoopbackAddr returns a 127.0.0.1 host:port that was free a moment ago.
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// waitMinIOReady polls endpoint's readiness probe until it answers 200, the
// process exits (exited closes; exitErr then reports why), or 60s pass.
func waitMinIOReady(ctx context.Context, endpoint string, exited <-chan struct{}, exitErr func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/minio/health/ready", nil)
		if err != nil {
			return err
		}
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-exited:
			return fmt.Errorf("process exited: %v", exitErr())
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// setup boots a fresh MinIO (see startMinIO), opens a temp SQLite DB for
// audit/job rows, builds the live domain processors against the pool,
// and registers cleanup hooks. Tests skip only when the
// HARBORMASTER_INTEGRATION env var is unset; a MinIO that cannot be
// started fails the test.
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

	srv := startMinIO(ctx, t)

	pool := hmminio.NewEmpty()
	if err := pool.Rebuild(hmminio.Credentials{
		EndpointURL: srv.EndpointURL,
		AccessKey:   srv.AccessKey,
		SecretKey:   srv.SecretKey,
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

	// Users + service accounts: a single shared policy materializer is
	// reused so a backup-target policy materialised via the users path is
	// visible to the service-accounts path (mirrors cmd/harbormaster/serve.go).
	policyMat := &policies.Materializer{
		Admin: func(ctx context.Context) (policies.PolicyAdmin, error) {
			madm, _, err := pool.Get(ctx)
			if err != nil {
				return nil, err
			}
			return madm, nil
		},
	}
	usersProc := users.NewProcessor(newUsersClientGetter(pool), policyMat).WithAudit(auditProc)
	saProc := users.NewServiceAccountProcessor(newSAClientGetter(pool), policyMat).WithAudit(auditProc)

	return &TestEnv{
		Pool:            pool,
		MC:              mc,
		Adm:             adm,
		Buckets:         bucketProc,
		Objects:         objectsProc,
		Lifecycle:       lifecycleProc,
		Empty:           emptyService,
		Audit:           auditProc,
		Users:           usersProc,
		ServiceAccounts: saProc,
		PolicyMat:       policyMat,
		DB:              gdb,
	}, ctx
}

// newUsersClientGetter mirrors cmd/harbormaster.newUsersClientGetter:
// the live *madmin.AdminClient satisfies users.AdminClient by structural
// typing, so no per-method adapter is needed.
func newUsersClientGetter(pool *hmminio.Pool) users.ClientGetter {
	return users.NewClientGetter(func(ctx context.Context) (users.AdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
}

// newSAClientGetter mirrors cmd/harbormaster.newSAClientGetter.
func newSAClientGetter(pool *hmminio.Pool) users.SAClientGetter {
	return users.NewSAClientGetter(func(ctx context.Context) (users.SAAdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
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

// ListObjectVersions mirrors cmd/harbormaster.objectS3Adapter, draining the
// high-level Client.ListObjects channel (WithVersions=true) into a slice,
// capping at maxScan. The bool return is "truncated". A cancelable context is
// derived and cancelled via defer so the minio-go producer goroutine is torn
// down on every return path.
func (a integrationObjectS3) ListObjectVersions(ctx context.Context, bucket, key string, maxScan int) ([]miniogo.ObjectInfo, bool, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := a.ListObjects(cctx, bucket, miniogo.ListObjectsOptions{
		Prefix:       key,
		WithVersions: true,
	})
	out := make([]miniogo.ObjectInfo, 0, 16)
	truncated := false
	for info := range ch {
		if info.Err != nil {
			return nil, false, info.Err
		}
		if len(out) >= maxScan {
			truncated = true
			break
		}
		out = append(out, info)
	}
	return out, truncated, nil
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
