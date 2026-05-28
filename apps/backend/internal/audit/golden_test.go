package audit

import (
	"encoding/json"
	"sort"
	"testing"
	"time"
)

func goldenKeySet(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, raw)
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// TestAuditEventWireContract pins the audit-event attributes block. The event
// id must NOT appear in attributes (it lives at the JSON:API resource id; the
// SPA merges it from there).
func TestAuditEventWireContract(t *testing.T) {
	ks := goldenKeySet(t, ToAttrs(Event{
		ID:             "01HABCDEF",
		OccurredAt:     time.Unix(1, 0).UTC(),
		Actor:          "alice",
		SourceIP:       "10.0.0.1",
		Action:         "bucket.create",
		TargetType:     "bucket",
		TargetID:       "photos",
		Outcome:        "success",
		ErrorMessage:   "boom",
		PayloadSummary: map[string]any{"k": "v"},
	}))
	want := []string{
		"action", "actor", "error_message", "occurred_at", "outcome",
		"payload_summary", "source_ip", "target_id", "target_type",
	}
	if len(ks) != len(want) {
		t.Fatalf("wire keys: got %v want %v", ks, want)
	}
	for i := range want {
		if ks[i] != want[i] {
			t.Fatalf("wire keys: got %v want %v", ks, want)
		}
	}
	for _, k := range ks {
		if k == "id" {
			t.Fatalf("audit attributes must not carry id (it is the resource id): %v", ks)
		}
	}
}
