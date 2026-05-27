package auth_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
)

// base64urlAlphabet matches the unpadded base64url alphabet ([A-Za-z0-9_-]).
var base64urlAlphabet = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func TestCSRFTokenLength(t *testing.T) {
	tok, err := auth.NewCSRFToken()
	require.NoError(t, err)
	require.Greater(t, len(tok), 32, "csrf token must be longer than 32 characters")
}

func TestCSRFTokensDifferAcrossCalls(t *testing.T) {
	a, err := auth.NewCSRFToken()
	require.NoError(t, err)
	b, err := auth.NewCSRFToken()
	require.NoError(t, err)
	require.NotEqual(t, a, b, "two consecutive tokens must not collide")
}

func TestCSRFTokenAlphabet(t *testing.T) {
	tok, err := auth.NewCSRFToken()
	require.NoError(t, err)
	require.Truef(t, base64urlAlphabet.MatchString(tok),
		"token %q contains chars outside the base64url alphabet", tok)
}
