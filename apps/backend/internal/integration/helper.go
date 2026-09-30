//go:build integration

// Package integration holds end-to-end tests that drive Harbormaster's
// domain processors against a real, per-test object store: MinIO (the
// default) or RustFS. The whole package is gated behind the `integration`
// build tag so the default `go test ./...` invocation excludes these files.
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
// running server at all — the tests skip with a clear "set
// HARBORMASTER_INTEGRATION=1 to enable" message.
//
// HARBORMASTER_IT_TARGET selects which store the suite runs against:
// "minio" (the default) or "rustfs". The same tests run unchanged against
// both; a call that works on one and fails on the other is a finding, not
// a flake. Any other value is a fatal configuration error.
//
// Each test gets its own server, from one of two sources per target:
//
//   - HARBORMASTER_MINIO_BINARY / HARBORMASTER_RUSTFS_BINARY set: a
//     server process started from that binary on a free loopback port
//     with a fresh temp data dir. No Docker needed; this is what the
//     nightly workflow uses, because the Forgejo runners have no Docker
//     daemon.
//   - otherwise: a testcontainers-go container (needs Docker), image from
//     HARBORMASTER_MINIO_IMAGE / HARBORMASTER_RUSTFS_IMAGE or the matching
//     default*Image constant.
//
// Once HARBORMASTER_INTEGRATION=1 is set, a process-mode server that fails
// to start FAILS the test — it used to skip, which let the nightly report
// green with every test skipped for as long as its image was unpullable.
// Container mode may still skip when Docker itself is unreachable, since
// that's a property of the local/CI environment rather than the server
// under test.
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
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/db"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	"github.com/jtumidanski/Harbormaster/internal/lifecycle"
	"github.com/jtumidanski/Harbormaster/internal/objects"
	"github.com/jtumidanski/Harbormaster/internal/objectstore"
	"github.com/jtumidanski/Harbormaster/internal/policies"
	"github.com/jtumidanski/Harbormaster/internal/users"
)

// envEnable is the env-var gate. Tests skip themselves when this is unset.
const envEnable = "HARBORMASTER_INTEGRATION"

// envTarget selects the object-store implementation the suite runs
// against: "minio" (default) or "rustfs".
const envTarget = "HARBORMASTER_IT_TARGET"

// envMinIOImage lets the nightly matrix (or a local operator) override
// the pinned MinIO image without editing source. Falls back to the
// default constant below when unset.
const envMinIOImage = "HARBORMASTER_MINIO_IMAGE"

// envMinIOBinary, when set, is the path to a `minio server`-compatible
// binary. setup() runs it as a local process instead of a testcontainer.
const envMinIOBinary = "HARBORMASTER_MINIO_BINARY"

// envRustFSImage lets the nightly matrix (or a local operator) override
// the pinned RustFS image without editing source. Falls back to the
// default constant below when unset.
const envRustFSImage = "HARBORMASTER_RUSTFS_IMAGE"

// envRustFSBinary, when set, is the path to a `rustfs`-compatible binary.
// setup() runs it as a local process instead of a testcontainer.
const envRustFSBinary = "HARBORMASTER_RUSTFS_BINARY"

// defaultMinIOImage is the pinned MinIO release. Pinning prevents a
// surprise CI failure when MinIO ships a backwards-incompatible
// admin-API tweak; bump deliberately and re-run the suite. The tag is
// one of the pgsty/minio "RELEASE.<timestamp>" tags: MinIO no longer
// publishes pullable images (quay.io/minio/minio and docker.io/minio/minio
// both 401 anonymous pulls), and pgsty/minio is the community fork that
// still cuts releases. The nightly workflow does not use this image; it
// runs binaries via HARBORMASTER_MINIO_BINARY.
const defaultMinIOImage = "pgsty/minio:RELEASE.2026-08-04T00-00-00Z"

// defaultRustFSImage is the pinned RustFS release (1.0.0 GA, 2026-09-16).
// The nightly workflow does not use this image; it runs binaries via
// HARBORMASTER_RUSTFS_BINARY.
const defaultRustFSImage = "rustfs/rustfs:1.0.0"

// processRootUser / processRootPassword are the root credentials a
// binary-mode MinIO server is started with. Throwaway: the server listens
// on loopback only and its data dir is deleted with the test.
const (
	processRootUser     = "harbormaster"
	processRootPassword = "harbormaster-integration"
)

// rustfsProcessAccessKey / rustfsProcessSecretKey are the root credentials
// a binary-mode RustFS server is started with. Same throwaway posture as
// the MinIO process credentials above.
const (
	rustfsProcessAccessKey = "harbormaster"
	rustfsProcessSecretKey = "harbormaster-integration"
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

// rustfsImageFor mirrors minioImageFor for the RustFS target.
func rustfsImageFor() string {
	if v := os.Getenv(envRustFSImage); v != "" {
		return v
	}
	return defaultRustFSImage
}

// TestEnv bundles the live MinIO clients and the wired-up domain
// processors a test needs. Each *_integration_test.go file calls setup()
// and uses the returned env to drive a happy-path scenario.
type TestEnv struct {
	Pool *objectstore.Pool

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

// minioServer is a running per-test object store: its endpoint URL and
// root credentials. The name predates RustFS support; it now also
// describes a RustFS server.
type minioServer struct {
	EndpointURL string
	AccessKey   string
	SecretKey   string
}

// startTarget starts a per-test object store from the configured target
// and source (see the package doc) and registers its teardown. It fails
// the test if the server cannot be started, except for container-mode
// Docker-unreachable failures, which skip.
func startTarget(ctx context.Context, t *testing.T) minioServer {
	t.Helper()
	switch tgt := os.Getenv(envTarget); tgt {
	case "", "minio":
		return startMinIO(ctx, t)
	case "rustfs":
		return startRustFS(ctx, t)
	default:
		t.Fatalf("%s=%q: want minio or rustfs", envTarget, tgt)
		panic("unreachable")
	}
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
	// version; normalise to a full http URL so objectstore.Pool's URL
	// parser accepts it.
	rawURL := normaliseEndpoint(t, endpoint)
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

	cmd := exec.Command(bin, "server", dataDir, "--address", addr, "--quiet")
	cmd.Env = append(os.Environ(),
		"MINIO_ROOT_USER="+processRootUser,
		"MINIO_ROOT_PASSWORD="+processRootPassword,
		"MINIO_BROWSER=off",
	)

	endpoint := "http://" + addr
	waitProcessReady(ctx, t, cmd, bin, endpoint+"/minio/health/ready")
	return minioServer{EndpointURL: endpoint, AccessKey: processRootUser, SecretKey: processRootPassword}
}

// startRustFS starts a per-test RustFS from the configured source (see the
// package doc) and registers its teardown. It fails the test if the
// server cannot be started.
func startRustFS(ctx context.Context, t *testing.T) minioServer {
	t.Helper()
	if bin := os.Getenv(envRustFSBinary); bin != "" {
		return startRustFSProcess(ctx, t, bin)
	}
	return startRustFSContainer(ctx, t, rustfsImageFor())
}

// startRustFSContainer runs the RustFS image with a fixed root pair and
// waits for its readiness endpoint. RustFS reads everything from env;
// there is no testcontainers module for it, so this is a
// GenericContainer.
func startRustFSContainer(ctx context.Context, t *testing.T, image string) minioServer {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"RUSTFS_ACCESS_KEY":                 rustfsProcessAccessKey,
			"RUSTFS_SECRET_KEY":                 rustfsProcessSecretKey,
			"RUSTFS_VOLUMES":                    "/data",
			"RUSTFS_ADDRESS":                    ":9000",
			"RUSTFS_CONSOLE_ENABLE":             "false",
			"RUSTFS_OBS_METRICS_EXPORT_ENABLED": "false",
		},
		WaitingFor: wait.ForHTTP("/health/ready").WithPort("9000/tcp").WithStartupTimeout(90 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("start RustFS testcontainer %q (Docker not reachable? set %s to run without Docker): %v",
			image, envRustFSBinary, err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Terminate(stopCtx)
	})

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("get RustFS connection string: %v", err)
	}
	rawURL := normaliseEndpoint(t, endpoint)
	return minioServer{EndpointURL: rawURL, AccessKey: rustfsProcessAccessKey, SecretKey: rustfsProcessSecretKey}
}

// startRustFSProcess runs `bin` on a free loopback port, pointed at a
// fresh temp data dir via RUSTFS_VOLUMES, and waits for /health/ready.
// The server's combined output goes to a file in the test's temp dir and
// is included in the failure message if it never becomes ready.
func startRustFSProcess(ctx context.Context, t *testing.T, bin string) minioServer {
	t.Helper()

	addr := freeLoopbackAddr(t)
	dataDir := t.TempDir()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"RUSTFS_ACCESS_KEY="+rustfsProcessAccessKey,
		"RUSTFS_SECRET_KEY="+rustfsProcessSecretKey,
		"RUSTFS_VOLUMES="+dataDir,
		"RUSTFS_ADDRESS="+addr,
		"RUSTFS_CONSOLE_ENABLE=false",
		"RUSTFS_OBS_METRICS_EXPORT_ENABLED=false",
	)

	endpoint := "http://" + addr
	waitProcessReady(ctx, t, cmd, bin, endpoint+"/health/ready")
	return minioServer{EndpointURL: endpoint, AccessKey: rustfsProcessAccessKey, SecretKey: rustfsProcessSecretKey}
}

// normaliseEndpoint prefixes endpoint with "http://" when it has no
// scheme yet, and fails the test if the result does not parse as a URL.
// testcontainers connection-string / Endpoint helpers return bare
// "host:port" on the module versions this package uses.
func normaliseEndpoint(t *testing.T, endpoint string) string {
	t.Helper()
	rawURL := endpoint
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	if _, err := url.Parse(rawURL); err != nil {
		t.Fatalf("invalid endpoint URL %q: %v", rawURL, err)
	}
	return rawURL
}

// waitProcessReady starts cmd (writing its combined output to a log file
// in the test's temp dir), registers its teardown, and blocks until
// readyURL answers 200, the process exits, or the readiness timeout
// elapses. It fails the test on any of those problems, including the
// dump of the server's log output.
func waitProcessReady(ctx context.Context, t *testing.T, cmd *exec.Cmd, bin, readyURL string) {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), filepath.Base(bin)+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create %s log file: %v", bin, err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start process %q: %v", bin, err)
	}
	// exited is closed once the process has been reaped; waitErr is safe to
	// read after that. A closed channel (rather than a sent value) lets both
	// the readiness poll and the cleanup below observe the exit.
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

	if err := waitReady(ctx, readyURL, exited, func() error { return waitErr }); err != nil {
		out, _ := os.ReadFile(logPath)
		t.Fatalf("process %q never became ready at %s: %v\n--- server output ---\n%s", bin, readyURL, err, out)
	}
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

// waitReady polls readyURL until it answers 200, the process exits
// (exited closes; exitErr then reports why), or 60s pass.
func waitReady(ctx context.Context, readyURL string, exited <-chan struct{}, exitErr func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
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

// setup boots a fresh object store (see startTarget), opens a temp SQLite
// DB for audit/job rows, builds the live domain processors against the
// pool, and registers cleanup hooks. Tests skip only when the
// HARBORMASTER_INTEGRATION env var is unset; a server that cannot be
// started fails the test (process mode) or skips (container mode, when
// Docker itself is unreachable).
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

	srv := startTarget(ctx, t)

	pool := objectstore.NewEmpty()
	if err := pool.Rebuild(objectstore.Credentials{
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
func newUsersClientGetter(pool *objectstore.Pool) users.ClientGetter {
	return users.NewClientGetter(func(ctx context.Context) (users.AdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
}

// newSAClientGetter mirrors cmd/harbormaster.newSAClientGetter.
func newSAClientGetter(pool *objectstore.Pool) users.SAClientGetter {
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

// BucketUsageInfo delegates to objectstore.BucketUsage, which tolerates both
// MinIO's camelCase and RustFS's snake_case census. A missing bucket surfaces
// as the zero value plus nil error so the processor's tolerant usage-fetch
// path treats it as "scanner has not seen this bucket yet".
func (a integrationBucketAdmin) BucketUsageInfo(ctx context.Context, bucket string) (madmin.BucketUsageInfo, error) {
	return objectstore.BucketUsage(ctx, a.AdminClient, bucket)
}

// newBucketClientGetter mirrors cmd/harbormaster.newBucketClientGetter.
func newBucketClientGetter(pool *objectstore.Pool) buckets.ClientGetter {
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
func newObjectClientGetter(pool *objectstore.Pool) objects.ClientGetter {
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
func newLifecycleClientGetter(pool *objectstore.Pool) lifecycle.ClientGetter {
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
