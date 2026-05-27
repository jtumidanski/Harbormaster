package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionPrints(t *testing.T) {
	var out bytes.Buffer
	root := newRootCmd(&out)
	root.SetArgs([]string{"version"})
	require.NoError(t, root.Execute())
	require.NotEqual(t, "", strings.TrimSpace(out.String()))
}

func TestAdminResetEncryptionRequiresConfirm(t *testing.T) {
	root := newRootCmd(&bytes.Buffer{})
	root.SetArgs([]string{"admin", "reset-encryption"})
	err := root.Execute()
	require.ErrorContains(t, err, "--confirm is mandatory")
}
