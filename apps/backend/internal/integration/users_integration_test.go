//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/policies"
	"github.com/jtumidanski/Harbormaster/internal/users"
)

// TestUsersIntegration_CreateListDelete walks the full IAM-user lifecycle
// through the live processor against the testcontainer MinIO:
//
//   - Create with the bundled "read-only" template → List (user is
//     present with the managed template attached) → Delete with
//     confirm_access_key → List (user is gone).
//
// Asserts the canonical Harbormaster policy is materialised on MinIO
// as part of Create (InfoCannedPolicy returns the JSON body).
func TestUsersIntegration_CreateListDelete(t *testing.T) {
	env, ctx := setup(t)

	const (
		accessKey = "harbormaster-it-user-readonly"
		actor     = "integration-test"
		sourceIP  = "127.0.0.1"
	)

	templates := []users.TemplateRef{{Name: "read-only"}}

	u, secret, err := env.Users.Create(ctx, accessKey, templates, actor, sourceIP)
	if err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	if u.AccessKey != accessKey {
		t.Fatalf("Create returned access_key=%q, want %q", u.AccessKey, accessKey)
	}
	if secret == "" {
		t.Fatalf("Create returned empty secret key")
	}

	// List must surface the new user with the managed template attached.
	list, err := env.Users.List(ctx)
	if err != nil {
		t.Fatalf("Users.List: %v", err)
	}
	var found *users.User
	for i := range list {
		if list[i].AccessKey == accessKey {
			found = &list[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("created user %q not present in List output (%d users)", accessKey, len(list))
	}
	if !hasTemplate(found.AttachedTemplates, "read-only", nil) {
		t.Errorf("user %q missing read-only template in AttachedTemplates=%+v",
			accessKey, found.AttachedTemplates)
	}

	// The canonical policy must exist on MinIO.
	if _, err := env.Adm.InfoCannedPolicy(ctx, policies.MaterializedName("read-only", nil)); err != nil {
		t.Errorf("InfoCannedPolicy(harbormaster-read-only) after Create: %v", err)
	}

	// Delete requires the confirm access key.
	if err := env.Users.Delete(ctx, accessKey, accessKey, actor, sourceIP); err != nil {
		t.Fatalf("Users.Delete: %v", err)
	}
	list, err = env.Users.List(ctx)
	if err != nil {
		t.Fatalf("Users.List after Delete: %v", err)
	}
	for _, u := range list {
		if u.AccessKey == accessKey {
			t.Fatalf("user %q still present after Delete", accessKey)
		}
	}
}

// TestUsersIntegration_BackupTargetPolicy verifies that creating a user
// with the parameterised backup-target template materialises a
// bucket-scoped canonical policy on MinIO with the expected canonical
// name (harbormaster-backup-target-<bucket>).
func TestUsersIntegration_BackupTargetPolicy(t *testing.T) {
	env, ctx := setup(t)

	const (
		accessKey = "harbormaster-it-user-backup"
		bucket    = "harbormaster-it-backup-bucket"
		actor     = "integration-test"
		sourceIP  = "127.0.0.1"
	)

	params := map[string]string{"bucket": bucket}
	templates := []users.TemplateRef{{Name: "backup-target", Params: params}}

	if _, _, err := env.Users.Create(ctx, accessKey, templates, actor, sourceIP); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}

	wantName := policies.MaterializedName("backup-target", params)
	if wantName != "harbormaster-backup-target-"+bucket {
		t.Fatalf("MaterializedName drifted: got %q", wantName)
	}
	body, err := env.Adm.InfoCannedPolicy(ctx, wantName)
	if err != nil {
		t.Fatalf("InfoCannedPolicy(%s): %v", wantName, err)
	}
	if !strings.Contains(string(body), bucket) {
		t.Errorf("materialised backup-target policy missing bucket name %q in body: %s",
			bucket, string(body))
	}

	// Confirm the policy is actually attached to the user.
	got, err := env.Users.Get(ctx, accessKey)
	if err != nil {
		t.Fatalf("Users.Get: %v", err)
	}
	if !hasTemplate(got.AttachedTemplates, "backup-target", params) {
		t.Errorf("user %q missing backup-target{bucket:%s} attachment; got %+v",
			accessKey, bucket, got.AttachedTemplates)
	}
}

// TestUsersIntegration_UpdatePolicies exercises the diff path: create
// with read-only, swap to read-write, and confirm the old template is
// detached and the new one attached.
func TestUsersIntegration_UpdatePolicies(t *testing.T) {
	env, ctx := setup(t)

	const (
		accessKey = "harbormaster-it-user-update"
		actor     = "integration-test"
		sourceIP  = "127.0.0.1"
	)

	if _, _, err := env.Users.Create(ctx, accessKey,
		[]users.TemplateRef{{Name: "read-only"}}, actor, sourceIP); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}

	if err := env.Users.UpdatePolicies(ctx, accessKey,
		[]users.TemplateRef{{Name: "read-write"}}, actor, sourceIP); err != nil {
		t.Fatalf("Users.UpdatePolicies: %v", err)
	}

	got, err := env.Users.Get(ctx, accessKey)
	if err != nil {
		t.Fatalf("Users.Get: %v", err)
	}
	if hasTemplate(got.AttachedTemplates, "read-only", nil) {
		t.Errorf("read-only template still attached after UpdatePolicies; got %+v",
			got.AttachedTemplates)
	}
	if !hasTemplate(got.AttachedTemplates, "read-write", nil) {
		t.Errorf("read-write template not attached after UpdatePolicies; got %+v",
			got.AttachedTemplates)
	}
}

// TestUsersIntegration_SetStatus toggles a user between enabled and
// disabled and verifies the status flag round-trips through MinIO.
func TestUsersIntegration_SetStatus(t *testing.T) {
	env, ctx := setup(t)

	const (
		accessKey = "harbormaster-it-user-status"
		actor     = "integration-test"
		sourceIP  = "127.0.0.1"
	)

	if _, _, err := env.Users.Create(ctx, accessKey,
		[]users.TemplateRef{{Name: "read-only"}}, actor, sourceIP); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}

	if err := env.Users.SetStatus(ctx, accessKey, false, actor, sourceIP); err != nil {
		t.Fatalf("SetStatus(false): %v", err)
	}
	got, err := env.Users.Get(ctx, accessKey)
	if err != nil {
		t.Fatalf("Get after disable: %v", err)
	}
	if !strings.EqualFold(got.Status, "disabled") {
		t.Errorf("status after disable = %q, want disabled", got.Status)
	}

	if err := env.Users.SetStatus(ctx, accessKey, true, actor, sourceIP); err != nil {
		t.Fatalf("SetStatus(true): %v", err)
	}
	got, err = env.Users.Get(ctx, accessKey)
	if err != nil {
		t.Fatalf("Get after enable: %v", err)
	}
	if !strings.EqualFold(got.Status, "enabled") {
		t.Errorf("status after enable = %q, want enabled", got.Status)
	}
}

// hasTemplate reports whether refs contains an entry whose Name matches
// want and whose Params match wantParams (nil/empty maps compare equal).
func hasTemplate(refs []users.TemplateRef, want string, wantParams map[string]string) bool {
	for _, r := range refs {
		if r.Name != want {
			continue
		}
		if len(r.Params) == 0 && len(wantParams) == 0 {
			return true
		}
		if len(r.Params) != len(wantParams) {
			continue
		}
		match := true
		for k, v := range wantParams {
			if r.Params[k] != v {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
