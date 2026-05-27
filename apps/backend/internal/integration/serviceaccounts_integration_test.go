//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/policies"
	"github.com/jtumidanski/Harbormaster/internal/users"
)

// TestServiceAccountsIntegration_CreateListRevoke walks the full SA
// lifecycle: create a parent IAM user, mint a service account with no
// override, confirm it appears in List, then Revoke and confirm it
// disappears.
func TestServiceAccountsIntegration_CreateListRevoke(t *testing.T) {
	env, ctx := setup(t)

	const (
		parent   = "harbormaster-it-sa-parent"
		actor    = "integration-test"
		sourceIP = "127.0.0.1"
	)

	if _, _, err := env.Users.Create(ctx, parent,
		[]users.TemplateRef{{Name: "read-only"}}, actor, sourceIP); err != nil {
		t.Fatalf("Users.Create(parent): %v", err)
	}

	sa, secret, err := env.ServiceAccounts.Create(ctx, parent, "", "", nil, actor, sourceIP)
	if err != nil {
		t.Fatalf("ServiceAccounts.Create: %v", err)
	}
	if sa.AccessKey == "" {
		t.Fatalf("ServiceAccounts.Create returned empty access_key")
	}
	if secret == "" {
		t.Fatalf("ServiceAccounts.Create returned empty secret")
	}

	list, err := env.ServiceAccounts.List(ctx, parent)
	if err != nil {
		t.Fatalf("ServiceAccounts.List: %v", err)
	}
	found := false
	for _, item := range list {
		if item.AccessKey == sa.AccessKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created SA %q not present in List output (%d accounts)",
			sa.AccessKey, len(list))
	}

	if err := env.ServiceAccounts.Revoke(ctx, sa.AccessKey, actor, sourceIP); err != nil {
		t.Fatalf("ServiceAccounts.Revoke: %v", err)
	}
	list, err = env.ServiceAccounts.List(ctx, parent)
	if err != nil {
		t.Fatalf("ServiceAccounts.List after Revoke: %v", err)
	}
	for _, item := range list {
		if item.AccessKey == sa.AccessKey {
			t.Fatalf("SA %q still present after Revoke", sa.AccessKey)
		}
	}
}

// TestServiceAccountsIntegration_TemplateOverride verifies that creating
// a service account with a backup-target override materialises the
// bucket-scoped canonical policy on MinIO (the materializer is shared
// with the users processor).
func TestServiceAccountsIntegration_TemplateOverride(t *testing.T) {
	env, ctx := setup(t)

	const (
		parent   = "harbormaster-it-sa-override-parent"
		bucket   = "harbormaster-it-sa-override-bucket"
		actor    = "integration-test"
		sourceIP = "127.0.0.1"
	)

	if _, _, err := env.Users.Create(ctx, parent,
		[]users.TemplateRef{{Name: "read-write"}}, actor, sourceIP); err != nil {
		t.Fatalf("Users.Create(parent): %v", err)
	}

	override := &users.TemplateRef{Name: "backup-target", Params: map[string]string{"bucket": bucket}}
	sa, _, err := env.ServiceAccounts.Create(ctx, parent, "scoped", "scoped to one bucket", override, actor, sourceIP)
	if err != nil {
		t.Fatalf("ServiceAccounts.Create with override: %v", err)
	}
	if sa.AccessKey == "" {
		t.Fatalf("ServiceAccounts.Create returned empty access_key")
	}

	// The materializer ensures the canonical policy exists on MinIO
	// even when its body is also delivered inline on AddServiceAccount.
	wantName := policies.MaterializedName("backup-target", override.Params)
	body, err := env.Adm.InfoCannedPolicy(ctx, wantName)
	if err != nil {
		t.Fatalf("InfoCannedPolicy(%s): %v", wantName, err)
	}
	if !strings.Contains(string(body), bucket) {
		t.Errorf("materialised backup-target policy missing bucket %q in body: %s",
			bucket, string(body))
	}

	// MinIO's InfoServiceAccount surfaces the inline policy on the SA;
	// confirm it carries the bucket name so we know the override took.
	info, err := env.Adm.InfoServiceAccount(ctx, sa.AccessKey)
	if err != nil {
		t.Fatalf("InfoServiceAccount: %v", err)
	}
	if info.Policy != "" && !strings.Contains(info.Policy, bucket) {
		t.Errorf("SA inline policy missing bucket %q: %s", bucket, info.Policy)
	}
}
