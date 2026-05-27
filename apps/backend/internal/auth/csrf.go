package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// NewCSRFToken returns a 256-bit base64url-encoded random token.
func NewCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
