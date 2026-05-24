package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/setup"
)

func TestReadMcAliasesV10(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(p, []byte(`{
		"version": "10",
		"aliases": {
			"myminio": {"url": "https://minio.lan:9000", "accessKey": "AKIA", "secretKey": "SECRET", "insecure": false}
		}
	}`), 0o600))
	aliases, ver, err := setup.ReadMcAliases(p)
	require.NoError(t, err)
	require.Equal(t, "10", ver)
	require.Len(t, aliases, 1)
	require.Equal(t, "myminio", aliases[0].Name)
	require.Equal(t, "AKIA", aliases[0].AccessKey)
}

func TestReadMcAliasesWrongVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"version":"9","aliases":{"x":{}}}`), 0o600))
	aliases, ver, err := setup.ReadMcAliases(p)
	require.NoError(t, err)
	require.Equal(t, "9", ver)
	require.Empty(t, aliases)
}

func TestReadMcAliasesMissingFile(t *testing.T) {
	a, _, err := setup.ReadMcAliases("/nonexistent/path")
	require.NoError(t, err)
	require.Empty(t, a)
}
