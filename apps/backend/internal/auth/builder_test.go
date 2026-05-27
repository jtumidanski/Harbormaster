package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
)

func TestAdminUserBuilderRejectsShortUsername(t *testing.T) {
	_, err := auth.NewAdminUserBuilder().
		Username("ab").
		PasswordHash("x").
		Build()
	require.Error(t, err)
}

func TestAdminUserBuilderRejectsBadCharacters(t *testing.T) {
	_, err := auth.NewAdminUserBuilder().
		Username("Foo Bar").
		PasswordHash("x").
		Build()
	require.Error(t, err)
}

func TestAdminUserBuilderHappyPath(t *testing.T) {
	u, err := auth.NewAdminUserBuilder().
		Username("admin").
		PasswordHash("$argon2id$...").
		Build()
	require.NoError(t, err)
	require.Equal(t, "admin", u.Username())
	require.Equal(t, "$argon2id$...", u.PasswordHash())
}

func TestSessionBuilderRequiresExpiry(t *testing.T) {
	_, err := auth.NewSessionBuilder().
		ID("01HKZ").
		AdminUserID(1).
		Build()
	require.Error(t, err)
}

func TestSessionBuilderRejectsExpiryBeforeCreation(t *testing.T) {
	now := time.Now().UTC()
	_, err := auth.NewSessionBuilder().
		ID("01HKZ").
		AdminUserID(1).
		CreatedAt(now).
		ExpiresAt(now.Add(-time.Second)).
		Build()
	require.Error(t, err)
}

func TestSessionBuilderHappyPath(t *testing.T) {
	now := time.Now().UTC()
	s, err := auth.NewSessionBuilder().
		ID("01HKZ").
		AdminUserID(1).
		CreatedAt(now).
		ExpiresAt(now.Add(time.Hour)).
		Build()
	require.NoError(t, err)
	require.Equal(t, "01HKZ", s.ID())
	require.Equal(t, uint(1), s.AdminUserID())
	require.WithinDuration(t, now.Add(time.Hour), s.ExpiresAt(), time.Second)
}
