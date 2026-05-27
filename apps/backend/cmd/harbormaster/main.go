package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := newRootCmd(os.Stdout).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "harbormaster",
		Short: "Self-hosted MinIO admin UI",
	}
	root.SetOut(out)
	root.AddCommand(
		newServeCmd(out),
		newVersionCmd(out),
		newAdminCmd(out),
	)
	return root
}

func newAdminCmd(out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "admin", Short: "Administrative recovery commands"}
	c.AddCommand(newAdminResetPasswordCmd(out), newAdminResetEncryptionCmd(out))
	return c
}
