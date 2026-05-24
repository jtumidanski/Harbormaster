package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadGeneratesKeyOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encryption.key")
	key, fp, err := LoadKey(path)
	require.NoError(t, err)
	require.Len(t, key, 32)
	expected := sha256.Sum256(key)
	require.Equal(t, hex.EncodeToString(expected[:]), fp)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadRejectsWorldReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encryption.key")
	require.NoError(t, os.WriteFile(path, make([]byte, 32), 0o644))
	_, _, err := LoadKey(path)
	require.ErrorContains(t, err, "world-readable")
}

func TestLoadReturnsExistingKeyAndFingerprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encryption.key")
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	key, fp, err := LoadKey(path)
	require.NoError(t, err)
	require.Equal(t, raw, key)
	expected := sha256.Sum256(raw)
	require.Equal(t, hex.EncodeToString(expected[:]), fp)
}
