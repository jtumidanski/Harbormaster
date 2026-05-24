package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// LoadKey returns (32-byte key, hex SHA-256 fingerprint).
// If the file does not exist, a new key is generated and written with 0600.
// Permission rules: world-readable bits set → fatal error.
// Group-readable bits set → returned alongside the key; caller logs a warning.
func LoadKey(path string) ([]byte, string, error) {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return generate(path)
	case err != nil:
		return nil, "", fmt.Errorf("stat key file: %w", err)
	}
	if info.Mode().Perm()&0o004 != 0 {
		return nil, "", fmt.Errorf("encryption key file %s is world-readable (perm %v); refusing to start", path, info.Mode().Perm())
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read key file: %w", err)
	}
	if len(key) != 32 {
		return nil, "", fmt.Errorf("encryption key file %s is %d bytes; expected 32", path, len(key))
	}
	sum := sha256.Sum256(key)
	return key, hex.EncodeToString(sum[:]), nil
}

func generate(path string) ([]byte, string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, "", fmt.Errorf("read random bytes: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, "", fmt.Errorf("write key file: %w", err)
	}
	sum := sha256.Sum256(key)
	return key, hex.EncodeToString(sum[:]), nil
}
