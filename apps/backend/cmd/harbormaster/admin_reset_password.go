package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

func newAdminResetPasswordCmd(out io.Writer) *cobra.Command {
	var username string
	c := &cobra.Command{
		Use:   "reset-password",
		Short: "Reset the local admin password (interactive prompt)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Fprint(out, "New password: ")
			pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(out)
			if err != nil {
				return err
			}
			fmt.Fprint(out, "Confirm password: ")
			pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(out)
			if err != nil {
				return err
			}
			if string(pw1) != string(pw2) {
				return errors.New("passwords do not match")
			}
			if len(pw1) < 12 {
				return errors.New("password must be at least 12 characters")
			}
			hash, err := auth.HashPassword(string(pw1))
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
			now := time.Now().UTC().Format(time.RFC3339)
			res := gdb.Exec(`UPDATE admin_users SET password_hash=?, updated_at=? WHERE username=?`, hash, now, username)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("no admin user %q", username)
			}
			fmt.Fprintf(out, "Password updated for user %q.\n", username)
			return nil
		},
	}
	c.Flags().StringVar(&username, "username", "", "admin username (required)")
	_ = c.MarkFlagRequired("username")
	return c
}
