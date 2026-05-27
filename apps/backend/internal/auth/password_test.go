package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
)

func TestHashVerify(t *testing.T) {
	h, err := auth.HashPassword("correct horse battery staple!")
	require.NoError(t, err)
	require.NoError(t, auth.VerifyPassword(h, "correct horse battery staple!"))
	require.Error(t, auth.VerifyPassword(h, "wrong"))
}

func TestHashIsRandomized(t *testing.T) {
	a, _ := auth.HashPassword("x")
	b, _ := auth.HashPassword("x")
	require.NotEqual(t, a, b)
}
