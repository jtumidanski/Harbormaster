package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
)

func TestLoginUnknownUser(t *testing.T) {
	gdb := newTestDB(t)
	p := auth.NewProcessor(gdb)

	_, _, err := p.Login(context.Background(), "nobody", "irrelevant", "127.0.0.1", "ua")
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae), "expected *apierror.Error, got %T", err)
	require.Equal(t, "invalid_credentials", ae.Code)
	require.Equal(t, 401, ae.HTTPStatus)
}

func TestLoginBadPassword(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "correct horse battery staple")
	p := auth.NewProcessor(gdb)

	_, _, err := p.Login(context.Background(), "admin", "wrong", "127.0.0.1", "ua")
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "invalid_credentials", ae.Code)
	require.Equal(t, 401, ae.HTTPStatus)
}

func TestLoginSuccessIssuesSessionAndCSRF(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "correct horse battery staple")
	p := auth.NewProcessor(gdb)

	sessID, csrf, err := p.Login(context.Background(), "admin",
		"correct horse battery staple", "127.0.0.1", "ua")
	require.NoError(t, err)
	require.NotEmpty(t, sessID)
	require.NotEmpty(t, csrf)
	require.Greater(t, len(csrf), 32, "csrf token must be > 32 chars")

	got, ok := p.CSRFTokenFor(sessID)
	require.True(t, ok)
	require.Equal(t, csrf, got)
}

func TestLogoutInvalidatesSession(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "pw")
	p := auth.NewProcessor(gdb)

	sessID, _, err := p.Login(context.Background(), "admin", "pw", "1.2.3.4", "ua")
	require.NoError(t, err)

	require.NoError(t, p.Logout(context.Background(), sessID))

	_, _, err = p.Me(context.Background(), sessID)
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "unauthenticated", ae.Code)
}

func TestSessionExpiry(t *testing.T) {
	gdb := newTestDB(t)
	adminID := seedAdmin(t, gdb, "admin", "pw")
	p := auth.NewProcessor(gdb)

	// Insert a session row directly with an expiry in the past.
	pastCreated := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	pastExpiry := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	res := gdb.Exec(
		`INSERT INTO sessions (id, admin_user_id, created_at, expires_at, last_active_at, source_ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"01EXPIRED", adminID, pastCreated, pastExpiry, pastCreated, "1.2.3.4", "ua",
	)
	require.NoError(t, res.Error)

	_, _, err := p.Me(context.Background(), "01EXPIRED")
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "unauthenticated", ae.Code)
}

func TestRotateOnLogin(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "pw")
	p := auth.NewProcessor(gdb)

	first, _, err := p.Login(context.Background(), "admin", "pw", "1.2.3.4", "ua")
	require.NoError(t, err)
	// A tiny pause ensures the monotonic ULID timestamp advances.
	time.Sleep(2 * time.Millisecond)
	second, _, err := p.Login(context.Background(), "admin", "pw", "1.2.3.4", "ua")
	require.NoError(t, err)
	require.NotEqual(t, first, second, "consecutive logins must mint distinct session ids")
}

func TestChangePasswordRejectsWrongCurrent(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "right")
	p := auth.NewProcessor(gdb)

	sessID, _, err := p.Login(context.Background(), "admin", "right", "1.2.3.4", "ua")
	require.NoError(t, err)

	err = p.ChangePassword(context.Background(), sessID, "wrong", "newpw")
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "invalid_credentials", ae.Code)
}

func TestChangePasswordHappyPath(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "old")
	p := auth.NewProcessor(gdb)

	sessID, _, err := p.Login(context.Background(), "admin", "old", "1.2.3.4", "ua")
	require.NoError(t, err)

	require.NoError(t, p.ChangePassword(context.Background(), sessID, "old", "new"))

	// Old password rejected.
	_, _, err = p.Login(context.Background(), "admin", "old", "1.2.3.4", "ua")
	require.Error(t, err)
	// New password accepted.
	_, _, err = p.Login(context.Background(), "admin", "new", "1.2.3.4", "ua")
	require.NoError(t, err)
}

func TestSweepRemovesExpired(t *testing.T) {
	gdb := newTestDB(t)
	adminID := seedAdmin(t, gdb, "admin", "pw")
	p := auth.NewProcessor(gdb)

	// Active session via the normal happy path.
	activeID, _, err := p.Login(context.Background(), "admin", "pw", "1.2.3.4", "ua")
	require.NoError(t, err)

	// Insert an expired session directly.
	pastCreated := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339Nano)
	pastExpiry := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	res := gdb.Exec(
		`INSERT INTO sessions (id, admin_user_id, created_at, expires_at, last_active_at, source_ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"01EXPIRED2", adminID, pastCreated, pastExpiry, pastCreated, "1.2.3.4", "ua",
	)
	require.NoError(t, res.Error)

	require.NoError(t, p.SweepExpired(context.Background()))

	// Expired row gone.
	var count int64
	require.NoError(t, gdb.Raw(
		`SELECT COUNT(*) FROM sessions WHERE id = ?`, "01EXPIRED2",
	).Scan(&count).Error)
	require.Equal(t, int64(0), count)

	// Active session still resolvable.
	_, _, err = p.Me(context.Background(), activeID)
	require.NoError(t, err)
}
