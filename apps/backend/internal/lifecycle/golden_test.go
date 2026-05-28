package lifecycle

import (
	"encoding/json"
	"sort"
	"testing"
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

func assertGoldenKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("wire keys: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("wire keys: got %v want %v", got, want)
		}
	}
}

// TestLifecycleRuleWireContract pins the two discriminated attribute shapes the
// SPA consumes; the rule id is intentionally absent from attributes (the client
// reads it from the JSON:API resource id).
func TestLifecycleRuleWireContract(t *testing.T) {
	assertGoldenKeys(t, goldenKeySet(t, RuleResource{Rule{
		ID: "harbormaster-expire-30d", Managed: true, Kind: "expiration", Days: 30, Prefix: "p/",
	}}), []string{"days", "kind", "managed", "prefix"})

	assertGoldenKeys(t, goldenKeySet(t, RuleResource{Rule{
		ID: "legacy", Managed: false, Summary: "expire after 90d",
	}}), []string{"managed", "summary"})
}
