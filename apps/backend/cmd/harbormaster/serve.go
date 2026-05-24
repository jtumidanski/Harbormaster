package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/db"
	"github.com/jtumidanski/Harbormaster/internal/observability/log"
	"github.com/jtumidanski/Harbormaster/internal/server"
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
	_ = cipher // wired into M2 setup/connection processors

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
	_ = auditProc // referenced by M2 handlers via context
	s := server.New(cfg, server.Deps{Logger: logger})
	logger.Info().Str("addr", cfg.ListenAddr).Msg("harbormaster started")
	return s.Run(ctx)
}
