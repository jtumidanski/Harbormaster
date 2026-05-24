package policies

import (
	"encoding/json"
	"testing"
)

// TestAllRendersValidJSON verifies that every bundled template's Render
// produces a JSON document MinIO will accept. backup-target needs a bucket
// param; everything else accepts an empty param map.
func TestAllRendersValidJSON(t *testing.T) {
	for _, tmpl := range All() {
		t.Run(tmpl.Name, func(t *testing.T) {
			params := map[string]string{}
			if tmpl.Name == "backup-target" {
				params["bucket"] = "test-bucket"
			}
			body, err := tmpl.Render(params)
			if err != nil {
				t.Fatalf("Render returned error: %v", err)
			}
			if body == "" {
				t.Fatal("Render returned empty body")
			}
			var parsed map[string]any
			if err := json.Unmarshal([]byte(body), &parsed); err != nil {
				t.Fatalf("Render produced invalid JSON: %v\nbody: %s", err, body)
			}
			if parsed["Version"] != "2012-10-17" {
				t.Errorf("missing/incorrect Version field: %v", parsed["Version"])
			}
			stmt, ok := parsed["Statement"].([]any)
			if !ok || len(stmt) == 0 {
				t.Fatalf("missing Statement[]: %v", parsed["Statement"])
			}
		})
	}
}

// TestBackupTargetRequiresBucket — the bucket param is mandatory; missing it
// must surface as an error rather than rendering a wildcard policy.
func TestBackupTargetRequiresBucket(t *testing.T) {
	tmpl, ok := Find("backup-target")
	if !ok {
		t.Fatal("backup-target template missing")
	}
	if _, err := tmpl.Render(map[string]string{}); err == nil {
		t.Fatal("expected error for missing bucket param, got nil")
	}
}

// TestMaterializedName confirms the deterministic naming contract the
// materializer depends on for idempotency.
func TestMaterializedName(t *testing.T) {
	cases := []struct {
		template string
		params   map[string]string
		want     string
	}{
		{"read-only", nil, "harbormaster-read-only"},
		{"read-write", nil, "harbormaster-read-write"},
		{"backup-target", map[string]string{"bucket": "ledger"}, "harbormaster-backup-target-ledger"},
		{"backup-target", map[string]string{"bucket": "photos"}, "harbormaster-backup-target-photos"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			got := MaterializedName(c.template, c.params)
			if got != c.want {
				t.Errorf("MaterializedName(%q, %v) = %q, want %q", c.template, c.params, got, c.want)
			}
		})
	}
	// Determinism: same input twice → same output.
	a := MaterializedName("backup-target", map[string]string{"bucket": "ledger"})
	b := MaterializedName("backup-target", map[string]string{"bucket": "ledger"})
	if a != b {
		t.Errorf("non-deterministic: %q vs %q", a, b)
	}
}

// TestNoAdminTemplate — Harbormaster intentionally does not bundle an
// administrator template. Find must miss so the REST layer can surface a
// typed 422 rather than silently materialising console-admin privileges.
func TestNoAdminTemplate(t *testing.T) {
	for _, name := range []string{"administrator", "admin", "console-admin", "consoleAdmin"} {
		if _, ok := Find(name); ok {
			t.Errorf("Find(%q) unexpectedly succeeded — admin templates must not be bundled", name)
		}
	}
}
