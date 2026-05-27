package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminResetEncryptionEnd2End(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HARBORMASTER_DATA_DIR", dir)
	t.Setenv("HARBORMASTER_DATABASE_PATH", filepath.Join(dir, "h.db"))
	require.NoError(t, os.MkdirAll(dir, 0o700))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "encryption.key"), make([]byte, 32), 0o600))
	// SQLite treats an empty file as a brand-new DB. "dummy" bytes would fail to open.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "h.db"), nil, 0o600))

	var out bytes.Buffer
	root := newRootCmd(&out)
	root.SetArgs([]string{"admin", "reset-encryption", "--confirm"})
	require.NoError(t, root.Execute())

	entries, _ := os.ReadDir(dir)
	hasBackup := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".bak" {
			hasBackup = true
		}
	}
	require.True(t, hasBackup, "expected at least one .bak file")
}
