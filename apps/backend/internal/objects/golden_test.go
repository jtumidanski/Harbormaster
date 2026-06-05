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

// TestVersionResourceWireContract asserts the exact wire-level key set for
// versionResource in two shapes: a regular version (size non-null) and a
// delete marker (size null, is_delete_marker true, etag/content_type absent).
func TestVersionResourceWireContract(t *testing.T) {
	sz := int64(1024)

	// Regular version: all fields present.
	regular := versionResource{ObjectVersion{
		Key:          "cat.jpg",
		VersionID:    "v1",
		Size:         &sz,
		LastModified: time.Unix(1700000000, 0).UTC(),
		ETag:         "abc123",
		ContentType:  "image/jpeg",
		IsLatest:     true,
	}}
	assertGoldenKeys(t, goldenKeySet(t, regular),
		[]string{"content_type", "etag", "is_delete_marker", "is_latest", "key", "last_modified", "size", "version_id"})

	// Verify ResourceType and ResourceID.
	if regular.ResourceType() != "object_versions" {
		t.Errorf("ResourceType: %q", regular.ResourceType())
	}
	if regular.ResourceID() != "cat.jpg@v1" {
		t.Errorf("ResourceID: %q", regular.ResourceID())
	}

	// Delete marker: size must marshal as null; etag and content_type are omitted.
	marker := versionResource{ObjectVersion{
		Key:            "cat.jpg",
		VersionID:      "dm1",
		Size:           nil, // nil → JSON null
		LastModified:   time.Unix(1700000001, 0).UTC(),
		IsLatest:       true,
		IsDeleteMarker: true,
	}}
	raw, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal delete marker: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal delete marker: %v", err)
	}

	// size must be present and equal to JSON null.
	sizeRaw, ok := m["size"]
	if !ok {
		t.Fatal("delete marker missing 'size' key")
	}
	if string(sizeRaw) != "null" {
		t.Errorf("delete marker size: got %s want null", sizeRaw)
	}

	// is_delete_marker must be true.
	var isDM bool
	if err := json.Unmarshal(m["is_delete_marker"], &isDM); err != nil || !isDM {
		t.Errorf("is_delete_marker: %s (want true)", m["is_delete_marker"])
	}

	// etag and content_type must be absent (omitempty on empty strings).
	if _, present := m["etag"]; present {
		t.Errorf("delete marker should omit 'etag'")
	}
	if _, present := m["content_type"]; present {
		t.Errorf("delete marker should omit 'content_type'")
	}
}
