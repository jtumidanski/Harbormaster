package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

func newAdminResetEncryptionCmd(out io.Writer) *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "reset-encryption",
		Short: "Destructive: back up DB, regenerate encryption key, clear minio_connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				fmt.Fprintln(out, `WARNING: this is a destructive recovery operation.
It will:
  1. Back up the SQLite database to <path>.pre-reset-<unix-ts>.bak
  2. Generate a new encryption key at HARBORMASTER_ENCRYPTION_KEY_FILE
     (or <data dir>/encryption.key by default)
  3. Truncate the minio_connections table
  4. Clear the setup_completed flag so the first-run wizard reappears

Re-run with --confirm to proceed.`)
				return errors.New("--confirm is mandatory; this is a destructive recovery operation")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ts := time.Now().Unix()
			backup := fmt.Sprintf("%s.pre-reset-%d.bak", cfg.DatabasePath, ts)
			if err := copyFile(cfg.DatabasePath, backup); err != nil {
				return fmt.Errorf("backup db: %w", err)
			}
			fmt.Fprintf(out, "Backup written to %s\n", backup)
			// Move the old key out of the way before generating a new one.
			if _, err := os.Stat(cfg.EncryptionKeyFile); err == nil {
				if err := os.Rename(cfg.EncryptionKeyFile,
					fmt.Sprintf("%s.pre-reset-%d.bak", cfg.EncryptionKeyFile, ts)); err != nil {
					return fmt.Errorf("rotate old key: %w", err)
				}
			}
			if _, _, err := crypto.LoadKey(cfg.EncryptionKeyFile); err != nil {
				return fmt.Errorf("generate new key: %w", err)
			}
			gdb, sdb, err := db.Open(cfg)
			if err != nil {
				return err
			}
			defer sdb.Close()
			if err := db.Migrate(gdb); err != nil {
				return err
			}
			if err := gdb.Exec(`DELETE FROM minio_connections`).Error; err != nil {
				return err
			}
			if err := gdb.Exec(`DELETE FROM app_settings WHERE key IN ('setup_completed','encryption_key_fingerprint')`).Error; err != nil {
				return err
			}
			// Now record a fresh fingerprint for the new key.
			_, fp, err := crypto.LoadKey(cfg.EncryptionKeyFile)
			if err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if err := gdb.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)`,
				"encryption_key_fingerprint", fp, now).Error; err != nil {
				return err
			}
			fmt.Fprintln(out, "Reset complete. Restart Harbormaster to enter the first-run wizard.")
			return nil
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "acknowledge destructive recovery")
	return c
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
