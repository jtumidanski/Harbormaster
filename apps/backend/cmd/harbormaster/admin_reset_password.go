package main

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

func newAdminResetPasswordCmd(out io.Writer) *cobra.Command {
	var username string
	c := &cobra.Command{
		Use:   "reset-password",
		Short: "Reset the local admin password (interactive prompt)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = out
			_ = username
			return errors.New("reset-password not yet implemented; lands in M1 task T1.13")
		},
	}
	c.Flags().StringVar(&username, "username", "", "admin username (required)")
	_ = c.MarkFlagRequired("username")
	return c
}
