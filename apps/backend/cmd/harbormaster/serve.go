package main

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/config"
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
	s := server.New(cfg, server.Deps{Logger: logger})
	logger.Info().Str("addr", cfg.ListenAddr).Msg("harbormaster started")
	return s.Run(ctx)
}
