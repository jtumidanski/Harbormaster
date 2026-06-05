package policies

import (
	"encoding/json"
	"testing"
)

// attrs marshals v to JSON and returns the result as a map[string]any.
func goldenAttrs(t *testing.T, v any) map[string]any {
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

// TestPolicyResourceMarshal_Custom asserts policyResource serialises the
// correct fields for a custom (editable) policy.
func TestPolicyResourceMarshal_Custom(t *testing.T) {
	res := policyResource{Policy{
		Name:             "my-policy",
		Origin:           OriginCustom,
		Editable:         true,
		StatementSummary: "Allow s3:GetObject on *",
	}}
	m := goldenAttrs(t, res)
	if m["name"] != "my-policy" {
		t.Errorf("name: got %v", m["name"])
	}
	if m["origin"] != OriginCustom {
		t.Errorf("origin: got %v", m["origin"])
	}
	if m["editable"] != true {
		t.Errorf("editable: got %v", m["editable"])
	}
	if m["statement_summary"] != "Allow s3:GetObject on *" {
		t.Errorf("statement_summary: got %v", m["statement_summary"])
	}
}

// TestPolicyResourceMarshal_Builtin asserts policyResource serialises a
// built-in (non-editable) policy correctly.
func TestPolicyResourceMarshal_Builtin(t *testing.T) {
	res := policyResource{Policy{
		Name:             "readonly",
		Origin:           OriginBuiltin,
		Editable:         false,
		StatementSummary: "Allow s3:GetObject on *",
	}}
	m := goldenAttrs(t, res)
	if m["name"] != "readonly" {
		t.Errorf("name: got %v", m["name"])
	}
	if m["origin"] != OriginBuiltin {
		t.Errorf("origin: got %v", m["origin"])
	}
	if m["editable"] != false {
		t.Errorf("editable: got %v", m["editable"])
	}
}

// TestPolicyDetailResourceDocumentPassthrough asserts that policyDetailResource
// round-trips the raw document bytes without alteration.
func TestPolicyDetailResourceDocumentPassthrough(t *testing.T) {
	rawDoc := json.RawMessage(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)

	res := policyDetailResource{PolicyDetail{
		Policy: Policy{
			Name:             "my-policy",
			Origin:           OriginCustom,
			Editable:         true,
			StatementSummary: "Allow s3:GetObject on *",
		},
		Document: rawDoc,
	}}

	out, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The document field must be present.
	if _, ok := m["document"]; !ok {
		t.Fatal("document key missing from policyDetailResource output")
	}

	// Re-marshal the document field and confirm it is byte-equal to the input.
	remarshaled, err := json.Marshal(m["document"])
	if err != nil {
		t.Fatalf("re-marshal document: %v", err)
	}
	if !json.Valid(remarshaled) {
		t.Fatal("re-marshaled document is not valid JSON")
	}
	// Normalise both sides through json.Unmarshal to compare values, not bytes.
	var want, got any
	_ = json.Unmarshal(rawDoc, &want)
	_ = json.Unmarshal(remarshaled, &got)
	wantBytes, _ := json.Marshal(want)
	gotBytes, _ := json.Marshal(got)
	if string(wantBytes) != string(gotBytes) {
		t.Errorf("document mismatch:\n  want: %s\n   got: %s", wantBytes, gotBytes)
	}
}

// TestResourceTypeIsPolicies pins the JSON:API type strings for both
// resource wrappers.
func TestResourceTypeIsPolicies(t *testing.T) {
	if got := (policyResource{}).ResourceType(); got != "policies" {
		t.Errorf("policyResource type: got %q want policies", got)
	}
	if got := (policyDetailResource{}).ResourceType(); got != "policies" {
		t.Errorf("policyDetailResource type: got %q want policies", got)
	}
}

// TestResourceIDIsName pins the ResourceID to the policy Name field.
func TestResourceIDIsName(t *testing.T) {
	pr := policyResource{Policy{Name: "my-policy"}}
	if pr.ResourceID() != "my-policy" {
		t.Errorf("policyResource ID: got %q", pr.ResourceID())
	}
	pdr := policyDetailResource{PolicyDetail{Policy: Policy{Name: "other"}}}
	if pdr.ResourceID() != "other" {
		t.Errorf("policyDetailResource ID: got %q", pdr.ResourceID())
	}
}
