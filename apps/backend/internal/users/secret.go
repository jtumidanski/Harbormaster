package users

import (
	"crypto/rand"
	"math/big"
)

// alphabet is the 62-character base62 set the secret generator draws from.
// MinIO accepts a wider alphabet but base62 sidesteps quoting headaches
// when the secret appears in shell snippets the operator might copy/paste.
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// secretLength is the fixed output length. 40 base62 characters carry
// log2(62^40) ≈ 238 bits of entropy — well beyond MinIO's 8-character
// minimum and beyond any sensible brute-force horizon.
const secretLength = 40

// GenerateSecret returns a fresh 40-character base62 secret using
// crypto/rand. The function is safe for concurrent use (crypto/rand.Reader
// is shared and thread-safe). The secret is intended to be revealed to the
// operator exactly once at user creation; Harbormaster never persists it.
func GenerateSecret() (string, error) {
	b := make([]byte, secretLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}
