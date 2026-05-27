package users

import (
	"context"
	"encoding/json"
	"testing"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// TestServiceAccountCreateReturnsSecretOnce — the secret is in the
// Credentials return value but never surfaces on the wire shape's
// MarshalJSON.
func TestServiceAccountCreateReturnsSecretOnce(t *testing.T) {
	sa, adm := newTestSAProcessor(t)
	adm.addServiceCreds = madmin.Credentials{AccessKey: "svc1", SecretKey: "supersecret-XYZ"}

	created, secret, err := sa.Create(context.Background(), "alice", "ci-key", "for nightly job", nil, "", "")
	require.NoError(t, err)
	require.Equal(t, "supersecret-XYZ", secret)
	require.Equal(t, "svc1", created.AccessKey)
	require.Len(t, adm.addServiceCalls, 1)
	require.Equal(t, "alice", adm.addServiceCalls[0].TargetUser)
	require.Equal(t, "ci-key", adm.addServiceCalls[0].Name)
	// No override → no inline policy.
	require.Empty(t, adm.addServiceCalls[0].Policy)
}

// TestServiceAccountListOmitsSecret — JSON for the read-side resource
// must never include a secret_key.
func TestServiceAccountListOmitsSecret(t *testing.T) {
	sa, adm := newTestSAProcessor(t)
	adm.listServiceResp = madmin.ListServiceAccountsResp{
		Accounts: []madmin.ServiceAccountInfo{
			{AccessKey: "svc1", ParentUser: "alice", Name: "k1", AccountStatus: "enabled"},
		},
	}
	list, err := sa.List(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, list, 1)

	body, err := json.Marshal(ServiceAccountResource{ServiceAccount: list[0]})
	require.NoError(t, err)
	require.NotContains(t, string(body), "secret")
}

// TestServiceAccountCreateBackupTargetOverride — the override is
// materialised via the policy materializer (AddCannedPolicy) and the
// rendered JSON is passed inline on AddServiceAccount so MinIO scopes the
// child credential to the bucket.
func TestServiceAccountCreateBackupTargetOverride(t *testing.T) {
	sa, adm := newTestSAProcessor(t)
	override := &TemplateRef{Name: "backup-target", Params: map[string]string{"bucket": "ledger"}}
	_, _, err := sa.Create(context.Background(), "alice", "backup", "nightly", override, "", "")
	require.NoError(t, err)

	// Materializer hit MinIO with the canonical name.
	require.Len(t, adm.addCannedCalls, 1)
	require.Equal(t, "harbormaster-backup-target-ledger", adm.addCannedCalls[0].Name)

	// AddServiceAccount carries the rendered policy inline.
	require.Len(t, adm.addServiceCalls, 1)
	require.NotEmpty(t, adm.addServiceCalls[0].Policy)
	var policy map[string]any
	require.NoError(t, json.Unmarshal(adm.addServiceCalls[0].Policy, &policy))
	require.Equal(t, "2012-10-17", policy["Version"])
}

// TestServiceAccountRevoke — happy path.
func TestServiceAccountRevoke(t *testing.T) {
	sa, adm := newTestSAProcessor(t)
	require.NoError(t, sa.Revoke(context.Background(), "svc1", "", ""))
	require.Equal(t, []string{"svc1"}, adm.deleteServiceCalls)
}

// --- audit invariants ---------------------------------------------------

func TestAuditEvent_ServiceAccountCreate_NoSecret(t *testing.T) {
	_, sa, a, adm := newAuditedProcessor(t)
	adm.addServiceCreds = madmin.Credentials{AccessKey: "svc1", SecretKey: "supersecret-XYZ"}

	_, secret, err := sa.Create(context.Background(), "alice", "ci-key", "desc", nil, "operator", "10.0.0.5")
	require.NoError(t, err)
	require.Equal(t, "supersecret-XYZ", secret)

	ev, payload := loadLatestPayload(t, a, audit.ActionServiceAccountCreate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "svc1", ev.TargetID)
	require.Contains(t, payload, "alice") // parent_user surfaces
	require.NotContains(t, payload, secret)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ServiceAccountRevoke(t *testing.T) {
	_, sa, a, _ := newAuditedProcessor(t)
	require.NoError(t, sa.Revoke(context.Background(), "svc1", "operator", "10.0.0.6"))

	ev, payload := loadLatestPayload(t, a, audit.ActionServiceAccountRevoke)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "svc1", ev.TargetID)
	requireNoSecrets(t, payload)
}
