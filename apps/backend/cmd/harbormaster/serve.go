package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/dashboard"
	"github.com/jtumidanski/Harbormaster/internal/db"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	"github.com/jtumidanski/Harbormaster/internal/lifecycle"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
	"github.com/jtumidanski/Harbormaster/internal/objects"
	"github.com/jtumidanski/Harbormaster/internal/observability/log"
	"github.com/jtumidanski/Harbormaster/internal/policies"
	"github.com/jtumidanski/Harbormaster/internal/server"
	"github.com/jtumidanski/Harbormaster/internal/setup"
	"github.com/jtumidanski/Harbormaster/internal/users"
)

func newServeCmd(out io.Writer) *cobra.Command {
	c := &cobra.Command{
		Use:   "serve",
		Short: "Run the Harbormaster HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), out)
		},
	}
	return c
}

func runServe(ctx context.Context, _ io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	logger, err := log.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return err
	}
	gdb, sdb, err := db.Open(cfg)
	if err != nil {
		return err
	}
	defer sdb.Close()
	if err := db.Migrate(gdb); err != nil {
		return err
	}
	keyBytes, fp, err := crypto.LoadKey(cfg.EncryptionKeyFile)
	if err != nil {
		return err
	}
	cipher, err := crypto.New(keyBytes)
	if err != nil {
		return err
	}

	// Fingerprint check
	var stored string
	gdb.Raw(`SELECT value FROM app_settings WHERE key = ?`, "encryption_key_fingerprint").Scan(&stored)
	switch {
	case stored == "":
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gdb.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)`,
			"encryption_key_fingerprint", fp, now).Error; err != nil {
			return err
		}
	case stored != fp:
		return fmt.Errorf("encryption key fingerprint mismatch (stored=%s, current=%s); refusing to start", stored, fp)
	}
	auditProc := audit.NewProcessor(gdb, cfg.AuditRetention)
	go audit.StartRetentionSweeper(ctx, auditProc, 24*time.Hour)

	// --- M2 wiring: auth, connection pool, setup --------------------------
	authProc := auth.NewProcessor(gdb).WithAudit(auditProc)
	limiter := auth.NewLoginRateLimiter(5*time.Minute, 5)
	pool := hmminio.NewEmpty()
	connProc := connection.NewProcessor(gdb, cipher, pool)
	connProc.Audit = auditProc
	// The in-memory pool starts empty on every boot. Rebuild it from the
	// persisted connection (if setup has run) so a restart serves MinIO-backed
	// requests without waiting for a PUT /connection. A failure here is logged,
	// not fatal: readiness no longer depends on MinIO, so a bad/undecryptable
	// connection must not brick the server — the operator can re-configure it.
	if err := connProc.HydratePool(ctx); err != nil {
		logger.Warn().Err(err).Msg("connection: failed to hydrate minio pool at boot; MinIO features unavailable until reconfigured")
	}
	setupProc := &setup.Processor{
		DB:       gdb,
		Cipher:   cipher,
		AuthProc: authProc,
		ConnProc: connProc,
		McPath:   cfg.McConfigPath,
	}

	// --- M3 wiring: bucket domain + empty-bucket job service --------------
	// The bucketempty service shares the audit log with the rest of the
	// app via a tiny shape adapter (see audit_adapter.go).
	bucketAudit := bucketEmptyAuditAdapter{p: auditProc}

	// Crash recovery: flip any state='running' rows left over from a prior
	// process to state='error' so the partial unique index does not block
	// new jobs. A failure here is logged but non-fatal — orphaned rows
	// simply re-surface on the next boot.
	if orphaned, err := bucketempty.OrphanRunningAtStartup(gdb, bucketAudit); err != nil {
		logger.Warn().Err(err).Msg("bucketempty: orphan-cleanup at startup failed; continuing")
	} else if len(orphaned) > 0 {
		logger.Info().Int("count", len(orphaned)).Msg("bucketempty: marked stale running jobs as orphaned")
	}

	emptyService := bucketempty.New(gdb, pool, bucketAudit)
	emptyHandler := &buckets.EmptyHandler{Service: emptyService}

	// --- lifecycle (T3.13 + T3.23) ----------------------------------------
	// Lifecycle is constructed BEFORE buckets so the bucket processor can
	// take a LifecycleCreator handle for the template-on-create path
	// (T3.21). Audit wiring (T3.23) goes on both.
	lifecycleProc := lifecycle.NewProcessor(newLifecycleClientGetter(pool)).
		WithLogger(logger).
		WithAudit(auditProc)

	bucketProc := buckets.NewProcessor(newBucketClientGetter(pool)).
		WithLogger(logger).
		WithAudit(auditProc).
		WithLifecycle(bucketLifecycleAdapter{lc: lifecycleProc})

	// --- objects (T3.11 + T3.23) ------------------------------------------
	// The object processor reads its byte/TTL/mode knobs from the same
	// config struct everything else uses, then bolts onto the pool via the
	// sibling adapter in audit_adapter.go. T3.23 wires the audit handle so
	// the object handlers emit per-action rows.
	objectsProc := objects.NewProcessor(newObjectClientGetter(pool), objects.ProcessorConfig{
		UploadMaxBytes:    cfg.UploadMaxBytes,
		ShareLinkMaxTTL:   cfg.ShareLinkMaxTTL,
		DownloadProxyMode: cfg.DownloadProxyMode,
	}).WithLogger(logger).WithAudit(auditProc)

	// --- M4 wiring: policy materializer + users + service accounts -------
	// The materializer is a thin wrapper that calls AddCannedPolicy on the
	// live admin client. Both the users and service-accounts processors
	// share the materializer so a backup-target policy materialised by one
	// path is visible to the other.
	policyMat := &policies.Materializer{
		Admin: func(ctx context.Context) (policies.PolicyAdmin, error) {
			madm, _, err := pool.Get(ctx)
			if err != nil {
				return nil, err
			}
			return madm, nil
		},
	}
	usersProc := users.NewProcessor(newUsersClientGetter(pool), policyMat).
		WithLogger(logger).
		WithAudit(auditProc)
	saProc := users.NewServiceAccountProcessor(newSAClientGetter(pool), policyMat).
		WithLogger(logger).
		WithAudit(auditProc)

	// --- M5 wiring: dashboard aggregator -----------------------------------
	// The dashboard fan-out hits madmin.ServerInfo via a PoolGetter adapter
	// (see audit_adapter.go) alongside the live buckets + audit processors.
	// No state is shared across requests; constructing a single Processor
	// at boot is purely to give Routes() a stable handle.
	dashboardProc := dashboard.NewProcessor(
		newDashboardPoolGetter(pool),
		bucketProc,
		auditProc,
	)

	style := apierror.StyleAction
	csrfCookieName := "harbormaster_csrf"

	authDeps := auth.HandlerDeps{
		Processor:         authProc,
		RateLimiter:       limiter,
		SessionCookieName: cfg.SessionCookieName,
		CSRFCookieName:    csrfCookieName,
		BasePath:          cfg.BasePath,
		SessionTimeout:    cfg.SessionTimeout,
		Secure:            cfg.SessionCookieSecure,
	}

	publicRoutes := func(r chi.Router) {
		r.Group(func(g chi.Router) {
			setup.Routes(setupProc)(g)
			authDeps.PublicRoutes()(g)
		})
	}

	protectedRoutes := func(r chi.Router) {
		r.Group(func(g chi.Router) {
			g.Use(auth.RequireSession(cfg.SessionCookieName, authProc, style))
			g.Use(auth.RequireCSRF(csrfCookieName, style))
			authDeps.ProtectedRoutes()(g)
			connection.Routes(connProc)(g)
			// The bucket Routes() omit the empty handler here — that
			// endpoint is mounted under StreamingAPIRoutes below so the
			// long-lived SSE stream is not killed by the 30s timeout.
			buckets.Routes(bucketProc, nil)(g)
			// T3.11: object listing/upload/delete/download + share-link
			// minting. Mounted under the same /api/v1 protected surface;
			// the routes self-prefix with /buckets/{bucket}/objects.
			objects.Routes(objectsProc)(g)
			// T3.13: lifecycle rules. Mounts /buckets/{name}/lifecycle-rules
			// (collection list+create, single delete) under the protected
			// API surface.
			lifecycle.Routes(lifecycleProc)(g)
			// M4: users, service-accounts, policy-templates. The Routes
			// function mounts the /users + /service-accounts + /policy-
			// templates surface in one go; both processors share the
			// materializer wired above.
			users.Routes(usersProc, saProc)(g)
			// M5: dashboard aggregate (action-style /dashboard) and
			// audit-event query collection (JSON:API /audit-events).
			dashboard.Routes(dashboardProc)(g)
			audit.Routes(auditProc)(g)
		})
	}

	// Streaming routes share the session+CSRF guards but bypass the 30s
	// per-request timeout. POST is a write so CSRF still applies.
	streamingRoutes := func(r chi.Router) {
		r.Group(func(g chi.Router) {
			g.Use(auth.RequireSession(cfg.SessionCookieName, authProc, style))
			g.Use(auth.RequireCSRF(csrfCookieName, style))
			g.Post("/buckets/{name}/empty", emptyHandler.ServeHTTP)
		})
	}

	// Readiness reflects only this instance's ability to serve HTTP and reach
	// its own database — deliberately NOT MinIO. A MinIO outage must never
	// pull the pod from the Service and lock operators out of the login page;
	// MinIO health is surfaced on the dashboard instead.
	ready := dbReadiness(sdb)

	// TODO(T2.17): add E2E test once setup.Probe is stubbable.
	s := server.New(cfg, server.Deps{
		Logger:             logger,
		APIRoutes:          []func(chi.Router){publicRoutes, protectedRoutes},
		StreamingAPIRoutes: []func(chi.Router){streamingRoutes},
		Ready:              ready,
	})
	logger.Info().Str("addr", cfg.ListenAddr).Msg("harbormaster started")
	return s.Run(ctx)
}

// dbReadiness returns a server.Deps.Ready snapshot that reports success iff
// the local database answers a ping. It intentionally takes no MinIO pool:
// readiness gates Service membership, and coupling it to MinIO reachability
// would let a downstream outage withdraw the pod and 503 every route —
// including the login page used to fix a bad connection.
func dbReadiness(sdb *sql.DB) func(context.Context) (bool, string) {
	return func(ctx context.Context) (bool, string) {
		if err := sdb.PingContext(ctx); err != nil {
			return false, "database unreachable"
		}
		return true, ""
	}
}
