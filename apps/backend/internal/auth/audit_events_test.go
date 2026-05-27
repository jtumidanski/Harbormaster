package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/auth"
)

// newAuditedProcessor returns an (auth.Processor, audit.Processor) pair
// sharing the same on-disk SQLite test database.
func newAuditedProcessor(t *testing.T) (*auth.Processor, *audit.Processor) {
	t.Helper()
	gdb := newTestDB(t)
	a := audit.NewProcessor(gdb, 90*24*time.Hour)
	p := auth.NewProcessor(gdb).WithAudit(a)
	return p, a
}

// loadLatestAudit returns the raw payload_summary_json string for the
// most-recent audit row matching the given action, or "" when none exist.
func loadLatestAudit(t *testing.T, a *audit.Processor, action string) (audit.Event, string) {
	t.Helper()
	events, err := audit.List(a.DB(), audit.Filter{Action: action, PageSize: 1})
	require.NoError(t, err)
	if len(events) == 0 {
		return audit.Event{}, ""
	}
	type row struct {
		PayloadSummaryJSON string `gorm:"column:payload_summary_json"`
	}
	var r row
	require.NoError(t,
		a.DB().
			Table("audit_events").
			Select("payload_summary_json").
			Where("id = ?", events[0].ID).
			Scan(&r).Error,
	)
	return events[0], r.PayloadSummaryJSON
}

// requireNoSecrets asserts that the sanitised payload JSON does not contain
// the textual markers we treat as sensitive across the audit subsystem.
func requireNoSecrets(t *testing.T, payload string) {
	t.Helper()
	lower := strings.ToLower(payload)
	require.NotContains(t, lower, "password",
		"audit payload leaked password text: %s", payload)
	require.NotContains(t, lower, "secret_key",
		"audit payload leaked secret_key text: %s", payload)
	require.NotContains(t, lower, "current_password",
		"audit payload leaked current_password text: %s", payload)
}

func TestAuditEvent_LoginSuccess(t *testing.T) {
	p, a := newAuditedProcessor(t)
	seedAdmin(t, p.DB(), "operator", "correct horse battery staple")

	_, _, err := p.Login(context.Background(), "operator",
		"correct horse battery staple", "10.0.0.1", "ua")
	require.NoError(t, err)

	ev, payload := loadLatestAudit(t, a, audit.ActionSessionLogin)
	require.NotEmpty(t, payload, "expected an audit row for session.login")
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "10.0.0.1", ev.SourceIP)
	require.Contains(t, payload, "operator")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_LoginFailureBadPassword(t *testing.T) {
	p, a := newAuditedProcessor(t)
	seedAdmin(t, p.DB(), "operator", "right")

	_, _, err := p.Login(context.Background(), "operator", "wrong-pw-attempt", "10.0.0.2", "ua")
	require.Error(t, err)

	ev, payload := loadLatestAudit(t, a, audit.ActionSessionLoginFailed)
	require.NotEmpty(t, payload, "expected an audit row for session.login_failed")
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "10.0.0.2", ev.SourceIP)
	require.Contains(t, payload, "operator")
	require.NotContains(t, payload, "wrong-pw-attempt",
		"login failure payload must not include the attempted password")
	requireNoSecrets(t, payload)

	// Successful login should NOT have been recorded.
	_, success := loadLatestAudit(t, a, audit.ActionSessionLogin)
	require.Empty(t, success, "no session.login row should exist for a failed login")
}

func TestAuditEvent_LoginFailureUnknownUser(t *testing.T) {
	p, a := newAuditedProcessor(t)

	_, _, err := p.Login(context.Background(), "ghost", "irrelevant", "10.0.0.3", "ua")
	require.Error(t, err)

	ev, payload := loadLatestAudit(t, a, audit.ActionSessionLoginFailed)
	require.NotEmpty(t, payload, "expected an audit row for session.login_failed")
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
	require.Equal(t, "ghost", ev.Actor)
	require.Contains(t, payload, "ghost")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_LogoutRecordsUsername(t *testing.T) {
	p, a := newAuditedProcessor(t)
	seedAdmin(t, p.DB(), "operator", "pw")

	sessID, _, err := p.Login(context.Background(), "operator", "pw", "10.0.0.4", "ua")
	require.NoError(t, err)
	require.NoError(t, p.Logout(context.Background(), sessID))

	ev, payload := loadLatestAudit(t, a, audit.ActionSessionLogout)
	require.NotEmpty(t, payload, "expected an audit row for session.logout")
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Contains(t, payload, "operator")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ChangePasswordSuccess(t *testing.T) {
	p, a := newAuditedProcessor(t)
	seedAdmin(t, p.DB(), "operator", "old-pw-123456")

	sessID, _, err := p.Login(context.Background(), "operator", "old-pw-123456", "10.0.0.5", "ua")
	require.NoError(t, err)

	require.NoError(t, p.ChangePassword(context.Background(), sessID,
		"old-pw-123456", "new-pw-456789"))

	ev, payload := loadLatestAudit(t, a, audit.ActionAdminPasswordChange)
	require.NotEmpty(t, payload, "expected an audit row for admin.password.change")
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.NotContains(t, payload, "old-pw-123456")
	require.NotContains(t, payload, "new-pw-456789")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ChangePasswordFailureWrongCurrent(t *testing.T) {
	p, a := newAuditedProcessor(t)
	seedAdmin(t, p.DB(), "operator", "real-pw-123456")

	sessID, _, err := p.Login(context.Background(), "operator", "real-pw-123456", "10.0.0.6", "ua")
	require.NoError(t, err)

	err = p.ChangePassword(context.Background(), sessID, "wrong-pw-attempt", "new-pw-456789")
	require.Error(t, err)

	events, err := audit.List(a.DB(), audit.Filter{
		Action: audit.ActionAdminPasswordChange, PageSize: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, events)
	// The most recent should be the failure.
	require.Equal(t, audit.OutcomeFailure, events[0].Outcome)
	require.Equal(t, "operator", events[0].Actor)

	_, payload := loadLatestAudit(t, a, audit.ActionAdminPasswordChange)
	require.NotContains(t, payload, "wrong-pw-attempt")
	require.NotContains(t, payload, "new-pw-456789")
	requireNoSecrets(t, payload)
}

// TestAuditEvent_LogoutEmittedWhenActorLookupFails covers the defensive
// branch where the session row exists but the underlying admin user is
// missing (e.g. the row was hand-deleted out-of-band). The logout still
// succeeds and must still be auditable — operators need the trail even
// when actor resolution failed. The actor falls back to "unknown" and the
// payload carries a "reason" tag so the unusual case is greppable.
func TestAuditEvent_LogoutEmittedWhenActorLookupFails(t *testing.T) {
	p, a := newAuditedProcessor(t)
	seedAdmin(t, p.DB(), "operator", "pw")

	sessID, _, err := p.Login(context.Background(), "operator", "pw", "10.0.0.7", "ua")
	require.NoError(t, err)

	// Force the actor lookup to fail by deleting every admin user row
	// while leaving the session row in place.
	require.NoError(t,
		p.DB().Exec("DELETE FROM admin_users").Error,
	)

	require.NoError(t, p.Logout(context.Background(), sessID))

	ev, payload := loadLatestAudit(t, a, audit.ActionSessionLogout)
	require.NotEmpty(t, payload,
		"logout must still emit an audit row even when actor lookup fails")
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome,
		"the delete succeeded, so the row's outcome should be success")
	require.Equal(t, "unknown", ev.Actor)
	require.Contains(t, payload, "actor_lookup_failed",
		"expected the payload to carry a reason marker for the unusual case")
	requireNoSecrets(t, payload)
}

// TestProcessor_NilAuditIsSafe verifies the nil-audit path (used by tests
// that construct an auth.Processor without WithAudit) does not panic and
// does not interfere with normal operation.
func TestProcessor_NilAuditIsSafe(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "operator", "pw")
	p := auth.NewProcessor(gdb) // no .WithAudit

	sessID, _, err := p.Login(context.Background(), "operator", "pw", "1.2.3.4", "ua")
	require.NoError(t, err)
	require.NotEmpty(t, sessID)
	require.NoError(t, p.Logout(context.Background(), sessID))
}
