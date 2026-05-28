package objects

import (
	"encoding/json"
	"sort"
	"testing"
	"time"
)

// goldenKeySet marshals v and returns its top-level JSON keys, sorted. Golden
// tests assert the exact wire key set so any field rename / casing drift / and
// the PascalCase-default-marshal trap is caught at the seam the SPA reads.
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

func TestObjectsWireContract(t *testing.T) {
	assertGoldenKeys(t, goldenKeySet(t, entryResource{Entry{
		Key: "k", Size: 1, LastModified: time.Unix(1, 0).UTC(), ContentType: "text/plain", ETag: "e",
	}}), []string{"content_type", "etag", "key", "last_modified", "size"})

	assertGoldenKeys(t, goldenKeySet(t, prefixResource{Prefix{Name: "photos/"}}),
		[]string{"prefix"})

	assertGoldenKeys(t, goldenKeySet(t, shareLinkResource{ShareLink{
		URL: "https://x", ExpiresAt: time.Unix(1, 0).UTC(),
	}}), []string{"expires_at", "url"})
}
