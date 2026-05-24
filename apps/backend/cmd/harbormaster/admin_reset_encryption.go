package main

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

func newAdminResetEncryptionCmd(out io.Writer) *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "reset-encryption",
		Short: "Destructive: back up DB, regenerate encryption key, clear minio_connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = out
			if !confirm {
				return errors.New("--confirm is mandatory; this is a destructive recovery operation")
			}
			return errors.New("reset-encryption not yet implemented; lands in M1 task T1.14")
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "acknowledge destructive recovery")
	return c
}
