package crypto

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	c, err := New(key)
	require.NoError(t, err)
	ct, err := c.Encrypt([]byte("hello world"))
	require.NoError(t, err)
	pt, err := c.Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, []byte("hello world"), pt)
}

func TestNonceUniqueness(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := New(key)
	a, _ := c.Encrypt([]byte("x"))
	b, _ := c.Encrypt([]byte("x"))
	require.NotEqual(t, a, b, "same plaintext must produce different ciphertext (random nonce)")
}

func TestRejectShortKey(t *testing.T) {
	_, err := New(make([]byte, 16))
	require.ErrorContains(t, err, "32 bytes")
}
