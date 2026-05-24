package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/db"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
	"github.com/jtumidanski/Harbormaster/internal/observability/log"
	"github.com/jtumidanski/Harbormaster/internal/server"
	"github.com/jtumidanski/Harbormaster/internal/setup"
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
	_ = auditProc // TODO(T2.16): wire audit into handler middleware

	// --- M2 wiring: auth, connection pool, setup --------------------------
	authProc := auth.NewProcessor(gdb)
	limiter := auth.NewLoginRateLimiter(5*time.Minute, 5)
	pool := hmminio.NewEmpty()
	connProc := connection.NewProcessor(gdb, cipher, pool)
	setupProc := &setup.Processor{
		DB:       gdb,
		Cipher:   cipher,
		AuthProc: authProc,
		ConnProc: connProc,
		McPath:   cfg.McConfigPath,
	}

	style := apierror.StyleAction
	csrfCookieName := "harbormaster_csrf"

	authDeps := auth.HandlerDeps{
		Processor:         authProc,
		RateLimiter:       limiter,
		SessionCookieName: cfg.SessionCookieName,
		CSRFCookieName:    csrfCookieName,
		BasePath:          cfg.BasePath,
		SessionTimeout:    cfg.SessionTimeout,
		Secure:            true,
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
		})
	}

	// TODO(T2.17): add E2E test once setup.Probe is stubbable.
	s := server.New(cfg, server.Deps{
		Logger:    logger,
		APIRoutes: []func(chi.Router){publicRoutes, protectedRoutes},
	})
	logger.Info().Str("addr", cfg.ListenAddr).Msg("harbormaster started")
	return s.Run(ctx)
}
