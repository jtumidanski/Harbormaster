package policies

import "testing"

func TestValidatePolicyName(t *testing.T) {
	good := []string{"my-policy", "team_read.v2", "a/b", "abc123"}
	for _, n := range good {
		if err := ValidatePolicyName(n); err != nil {
			t.Errorf("ValidatePolicyName(%q) unexpected error: %v", n, err)
		}
	}
	bad := []string{"", "has space", "bad$char", string(make([]byte, 129))}
	for _, n := range bad {
		if err := ValidatePolicyName(n); err == nil {
			t.Errorf("ValidatePolicyName(%q) expected error", n)
		}
	}
}

func TestValidatePolicyDocument(t *testing.T) {
	valid := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::b/*"]}]}`)
	if err := ValidatePolicyDocument(valid); err != nil {
		t.Fatalf("valid doc rejected: %v", err)
	}
	if err := ValidatePolicyDocument([]byte(`{not json`)); err == nil {
		t.Error("expected invalid_policy_json")
	}
	noStmt := []byte(`{"Version":"2012-10-17","Statement":[]}`)
	if err := ValidatePolicyDocument(noStmt); err == nil {
		t.Error("expected invalid_policy_structure for empty Statement")
	}
	badEffect := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Maybe","Action":["s3:GetObject"]}]}`)
	if err := ValidatePolicyDocument(badEffect); err == nil {
		t.Error("expected invalid_policy_structure for bad Effect")
	}
	noAction := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`)
	if err := ValidatePolicyDocument(noAction); err == nil {
		t.Error("expected invalid_policy_structure for missing Action/NotAction")
	}
}
