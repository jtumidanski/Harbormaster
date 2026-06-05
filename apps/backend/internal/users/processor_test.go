package users

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// TestCreateGeneratesSecretAndAttaches — happy path: a fresh user gets a
// 40-character secret, AddUser is invoked once, and every requested
// template surfaces as an AttachPolicy call with the canonical name.
func TestCreateGeneratesSecretAndAttaches(t *testing.T) {
	p, adm := newTestProcessor(t)

	refs := []TemplateRef{
		{Name: "read-only"},
		{Name: "backup-target", Params: map[string]string{"bucket": "ledger"}},
	}
	u, secret, err := p.Create(context.Background(), "alice", refs, "operator", "10.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "alice", u.AccessKey)
	require.Len(t, secret, secretLength)

	require.Len(t, adm.addUserCalls, 1)
	require.Equal(t, "alice", adm.addUserCalls[0].AccessKey)
	require.Equal(t, secret, adm.addUserCalls[0].SecretKey)

	require.Len(t, adm.attachCalls, 2)
	wantNames := map[string]struct{}{
		"harbormaster-read-only":            {},
		"harbormaster-backup-target-ledger": {},
	}
	for _, c := range adm.attachCalls {
		require.Len(t, c.Policies, 1)
		_, ok := wantNames[c.Policies[0]]
		require.Truef(t, ok, "unexpected attached policy: %s", c.Policies[0])
		require.Equal(t, "alice", c.User)
	}
}

// TestCreateMaterializesPolicies — each requested template causes an
// AddCannedPolicy call (the materializer's contract). Two templates →
// two AddCannedPolicy invocations.
func TestCreateMaterializesPolicies(t *testing.T) {
	p, adm := newTestProcessor(t)

	refs := []TemplateRef{
		{Name: "read-only"},
		{Name: "read-write"},
	}
	_, _, err := p.Create(context.Background(), "bob", refs, "", "")
	require.NoError(t, err)

	require.Len(t, adm.addCannedCalls, 2)
	names := map[string]struct{}{}
	for _, c := range adm.addCannedCalls {
		names[c.Name] = struct{}{}
	}
	require.Contains(t, names, "harbormaster-read-only")
	require.Contains(t, names, "harbormaster-read-write")
}

// TestCreateRejectsUnknownTemplate — admin-style or unknown template
// names short-circuit before AddUser runs.
func TestCreateRejectsUnknownTemplate(t *testing.T) {
	p, adm := newTestProcessor(t)
	_, _, err := p.Create(context.Background(), "alice", []TemplateRef{{Name: "administrator"}}, "", "")
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "unknown_template", ae.Code)
	require.Empty(t, adm.addUserCalls)
}

// TestDeleteRequiresExactAccessKeyMatch — the destructive-action guard:
// confirm_access_key must match the access key character-for-character.
func TestDeleteRequiresExactAccessKeyMatch(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("")

	err := p.Delete(context.Background(), "alice", "alice-typo", "operator", "10.0.0.2")
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "confirm_name_mismatch", ae.Code)
	require.Equal(t, http.StatusForbidden, ae.HTTPStatus)
	require.Empty(t, adm.removeCalls)
}

// TestDeleteHappyPath — matching confirmation removes the user.
func TestDeleteHappyPath(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("")

	require.NoError(t, p.Delete(context.Background(), "alice", "alice", "operator", "10.0.0.2"))
	require.Equal(t, []string{"alice"}, adm.removeCalls)
}

// TestSetStatusEnableAndDisable — covers both branches; the audit
// constant differs per branch.
func TestSetStatusEnableAndDisable(t *testing.T) {
	p, adm := newTestProcessor(t)
	require.NoError(t, p.SetStatus(context.Background(), "alice", true, "", ""))
	require.NoError(t, p.SetStatus(context.Background(), "alice", false, "", ""))
	require.Len(t, adm.setStatusCalls, 2)
	require.Equal(t, "enabled", string(adm.setStatusCalls[0].Status))
	require.Equal(t, "disabled", string(adm.setStatusCalls[1].Status))
}

// TestUpdatePoliciesDiff — current [A, B], requested [B, C] → Detach A,
// Attach C, leave B alone.
func TestUpdatePoliciesDiff(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("harbormaster-read-only,harbormaster-read-write")

	requested := []TemplateRef{
		{Name: "read-write"}, // unchanged
		{Name: "backup-target", Params: map[string]string{"bucket": "ledger"}}, // new
	}
	require.NoError(t, p.UpdatePolicies(context.Background(), "alice", requested, nil, "", ""))

	require.Len(t, adm.detachCalls, 1)
	require.Equal(t, []string{"harbormaster-read-only"}, adm.detachCalls[0].Policies)
	require.Equal(t, "alice", adm.detachCalls[0].User)

	require.Len(t, adm.attachCalls, 1)
	require.Equal(t, []string{"harbormaster-backup-target-ledger"}, adm.attachCalls[0].Policies)
}

// TestUpdatePoliciesIdempotent — requested == current → no detach, no
// attach.
func TestUpdatePoliciesIdempotent(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("harbormaster-read-only")

	require.NoError(t, p.UpdatePolicies(context.Background(), "alice",
		[]TemplateRef{{Name: "read-only"}}, nil, "", ""))
	require.Empty(t, adm.detachCalls)
	require.Empty(t, adm.attachCalls)
}

// TestListClassifiesPolicies — managed (harbormaster-*) policies surface
// as TemplateRef entries; operator-installed names (consoleAdmin) land in
// OtherPolicies.
func TestListClassifiesPolicies(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("harbormaster-read-only,consoleAdmin")
	adm.users["bob"] = makeUserInfo("harbormaster-backup-target-ledger")

	us, err := p.List(context.Background())
	require.NoError(t, err)
	require.Len(t, us, 2)

	// alice first (sorted).
	require.Equal(t, "alice", us[0].AccessKey)
	require.Len(t, us[0].AttachedTemplates, 1)
	require.Equal(t, "read-only", us[0].AttachedTemplates[0].Name)
	require.Equal(t, []string{"consoleAdmin"}, us[0].OtherPolicies)

	require.Equal(t, "bob", us[1].AccessKey)
	require.Len(t, us[1].AttachedTemplates, 1)
	require.Equal(t, "backup-target", us[1].AttachedTemplates[0].Name)
	require.Equal(t, "ledger", us[1].AttachedTemplates[0].Params["bucket"])
	require.Empty(t, us[1].OtherPolicies)
}

// --- custom-policy attach/detach tests ----------------------------------

// TestUpdatePolicies_AttachesCustom — deployment has "proj-a" (custom);
// request policies:["proj-a"] → stub records an AttachPolicy call for "proj-a".
func TestUpdatePolicies_AttachesCustom(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("")
	adm.canned["proj-a"] = json.RawMessage(`{}`) // custom origin (no "harbormaster-" prefix)

	err := p.UpdatePolicies(context.Background(), "alice", nil, []string{"proj-a"}, "", "")
	require.NoError(t, err)

	// Find an AttachPolicy call for "proj-a" specifically.
	found := false
	for _, c := range adm.attachCalls {
		for _, pol := range c.Policies {
			if pol == "proj-a" {
				found = true
			}
		}
	}
	require.True(t, found, "expected AttachPolicy call for proj-a, got calls: %v", adm.attachCalls)
}

// TestUpdatePolicies_DetachesRemovedCustom — user currently has "proj-a"
// (custom, in deployment); request empty policies → DetachPolicy("proj-a").
func TestUpdatePolicies_DetachesRemovedCustom(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("proj-a")
	adm.canned["proj-a"] = json.RawMessage(`{}`)

	err := p.UpdatePolicies(context.Background(), "alice", nil, nil, "", "")
	require.NoError(t, err)

	found := false
	for _, c := range adm.detachCalls {
		for _, pol := range c.Policies {
			if pol == "proj-a" {
				found = true
			}
		}
	}
	require.True(t, found, "expected DetachPolicy call for proj-a, got calls: %v", adm.detachCalls)
}

// TestUpdatePolicies_NeverDetachesBuiltin — GetUserInfo returns PolicyName
// "consoleAdmin"; update with empty templates+policies → ZERO DetachPolicy calls.
// consoleAdmin is a MinIO built-in and must never be detached.
func TestUpdatePolicies_NeverDetachesBuiltin(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("consoleAdmin")
	// consoleAdmin is NOT in the custom deployment set — it's a built-in.

	err := p.UpdatePolicies(context.Background(), "alice", nil, nil, "", "")
	require.NoError(t, err)

	require.Empty(t, adm.detachCalls,
		"consoleAdmin must never be detached; got detach calls: %v", adm.detachCalls)
}

// TestUpdatePolicies_NeverDetachesForeign — user has "some-foreign" NOT in
// deployment custom set; empty request → never detached.
func TestUpdatePolicies_NeverDetachesForeign(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("some-foreign")
	// "some-foreign" is not in adm.canned → not in deploymentCustom.

	err := p.UpdatePolicies(context.Background(), "alice", nil, nil, "", "")
	require.NoError(t, err)

	require.Empty(t, adm.detachCalls,
		"some-foreign must never be detached (not in deployment); got: %v", adm.detachCalls)
}

// TestUpdatePolicies_RejectsUnknownPolicy — requesting a custom policy not
// present in the deployment returns 422 with code "unknown_policy".
func TestUpdatePolicies_RejectsUnknownPolicy(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("")
	// "nope" is not in adm.canned.

	err := p.UpdatePolicies(context.Background(), "alice", nil, []string{"nope"}, "", "")
	require.Error(t, err)

	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, "unknown_policy", ae.Code)
	require.Equal(t, http.StatusUnprocessableEntity, ae.HTTPStatus)
}

// TestUpdatePolicies_DropsCustomKeepsBuiltin — combined security invariant:
// a user whose GetUserInfo PolicyName includes BOTH a built-in ("consoleAdmin")
// AND an owned custom policy ("proj-a", present in the deployment); calling
// UpdatePolicies with empty templates + empty policies must:
//   - DetachPolicy("proj-a")   — owned custom IS diffed and removed
//   - ZERO detach of "consoleAdmin" — built-in preserved under all conditions
func TestUpdatePolicies_DropsCustomKeepsBuiltin(t *testing.T) {
	p, adm := newTestProcessor(t)
	// User has both consoleAdmin (builtin) and proj-a (owned custom) attached.
	adm.users["alice"] = makeUserInfo("consoleAdmin,proj-a")
	// proj-a is in the deployment custom set; consoleAdmin is NOT.
	adm.canned["proj-a"] = json.RawMessage(`{}`)

	err := p.UpdatePolicies(context.Background(), "alice", nil, nil, "", "")
	require.NoError(t, err)

	// consoleAdmin must never appear in any detach call.
	for _, c := range adm.detachCalls {
		for _, pol := range c.Policies {
			require.NotEqual(t, "consoleAdmin", pol,
				"consoleAdmin (builtin) was detached — invariant violated; detach calls: %v", adm.detachCalls)
		}
	}

	// proj-a (owned custom, not in requested set) must have been detached.
	detachedProjA := false
	for _, c := range adm.detachCalls {
		for _, pol := range c.Policies {
			if pol == "proj-a" {
				detachedProjA = true
			}
		}
	}
	require.True(t, detachedProjA,
		"proj-a (owned custom) should have been detached; detach calls: %v", adm.detachCalls)
}

// --- audit invariants ---------------------------------------------------

func TestAuditEvent_UserCreate_NoSecret(t *testing.T) {
	p, _, a, _ := newAuditedProcessor(t)
	_, secret, err := p.Create(context.Background(), "alice",
		[]TemplateRef{{Name: "read-only"}}, "operator", "10.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, secret)

	ev, payload := loadLatestPayload(t, a, audit.ActionUserCreate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "alice", ev.TargetID)
	require.Contains(t, payload, "alice")
	require.NotContains(t, payload, secret) // the generated secret must not appear
	requireNoSecrets(t, payload)
}

func TestAuditEvent_UserDelete(t *testing.T) {
	p, _, a, adm := newAuditedProcessor(t)
	adm.users["alice"] = makeUserInfo("")
	require.NoError(t, p.Delete(context.Background(), "alice", "alice", "operator", "10.0.0.2"))

	ev, payload := loadLatestPayload(t, a, audit.ActionUserDelete)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "alice", ev.TargetID)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_UserStatusEnable(t *testing.T) {
	p, _, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.SetStatus(context.Background(), "alice", true, "operator", "10.0.0.3"))

	ev, payload := loadLatestPayload(t, a, audit.ActionUserEnable)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "alice", ev.TargetID)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_UserStatusDisable(t *testing.T) {
	p, _, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.SetStatus(context.Background(), "alice", false, "operator", "10.0.0.3"))

	ev, payload := loadLatestPayload(t, a, audit.ActionUserDisable)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_UserPoliciesUpdate(t *testing.T) {
	p, _, a, adm := newAuditedProcessor(t)
	adm.users["alice"] = makeUserInfo("harbormaster-read-only")
	require.NoError(t, p.UpdatePolicies(context.Background(), "alice",
		[]TemplateRef{{Name: "read-write"}}, nil, "operator", "10.0.0.4"))

	ev, payload := loadLatestPayload(t, a, audit.ActionUserPoliciesUpdate)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "alice", ev.TargetID)
	requireNoSecrets(t, payload)
}
