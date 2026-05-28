package users

import (
	"encoding/json"
	"testing"
)

// The SPA reads access_key from the JSON:API attributes block
// (res.data.map(d => d.attributes)), and the API contract documents it there.
// Omitting it (leaving the key only in the resource `id`) renders a blank
// access-key column and navigates row clicks to /users/undefined.

func attrs(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestUserResourceIncludesAccessKey(t *testing.T) {
	m := attrs(t, UserResource{User{AccessKey: "alice", Status: "enabled"}})
	if m["access_key"] != "alice" {
		t.Errorf("user attributes missing access_key: %v", m)
	}
}

func TestServiceAccountResourceIncludesAccessKey(t *testing.T) {
	m := attrs(t, ServiceAccountResource{ServiceAccount{AccessKey: "sa-1", ParentUser: "alice", Status: "on"}})
	if m["access_key"] != "sa-1" {
		t.Errorf("service-account attributes missing access_key: %v", m)
	}
}

func TestCreatedUserResourceIncludesAccessKey(t *testing.T) {
	m := attrs(t, CreatedUserResource{User: User{AccessKey: "bob", Status: "enabled"}, SecretKey: "s3cr3t"})
	if m["access_key"] != "bob" {
		t.Errorf("created-user attributes missing access_key: %v", m)
	}
	if m["secret_key"] != "s3cr3t" {
		t.Errorf("created-user attributes missing secret_key: %v", m)
	}
}

// TestCreateServiceAccountRequestDecodesTemplateOverride guards the wire key
// `template_override` (it was previously decoded as `template`, silently
// dropping the operator's chosen policy on every service-account create).
func TestCreateServiceAccountRequestDecodesTemplateOverride(t *testing.T) {
	var req CreateServiceAccountRequest
	body := `{"name":"x","description":"y","template_override":{"name":"backup-target","params":{"bucket":"b"}}}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ov := req.Override()
	if ov == nil || ov.Name != "backup-target" || ov.Params["bucket"] != "b" {
		t.Fatalf("template_override not decoded into Override(): %+v", req)
	}
}

// TestResourceTypesUseUnderscoreConvention pins the JSON:API `type` strings to
// the contract's underscore form (the SPA types and api-contracts.md use
// service_accounts / policy_templates, matching object_entries etc.).
func TestResourceTypesUseUnderscoreConvention(t *testing.T) {
	if got := (ServiceAccountResource{}).ResourceType(); got != "service_accounts" {
		t.Errorf("ServiceAccountResource type: got %q want service_accounts", got)
	}
	if got := (CreatedServiceAccountResource{}).ResourceType(); got != "service_accounts" {
		t.Errorf("CreatedServiceAccountResource type: got %q want service_accounts", got)
	}
	if got := (PolicyTemplateResource{}).ResourceType(); got != "policy_templates" {
		t.Errorf("PolicyTemplateResource type: got %q want policy_templates", got)
	}
}

func TestCreatedServiceAccountResourceIncludesAccessKey(t *testing.T) {
	m := attrs(t, CreatedServiceAccountResource{
		ServiceAccount: ServiceAccount{AccessKey: "sa-2", ParentUser: "alice"},
		SecretKey:      "s3cr3t",
	})
	if m["access_key"] != "sa-2" {
		t.Errorf("created-service-account attributes missing access_key: %v", m)
	}
	if m["secret_key"] != "s3cr3t" {
		t.Errorf("created-service-account attributes missing secret_key: %v", m)
	}
}
