package objects

import (
	"encoding/json"
	"time"
)

// entryResource is the JSON:API wrapper for an Entry. The wire type is
// "object_entries" — list responses return a heterogeneous data[] that
// mixes object_entries (real keys) and object_prefixes (common-prefix
// directories).
type entryResource struct {
	Entry
}

// ResourceType returns the canonical JSON:API type string.
func (e entryResource) ResourceType() string { return "object_entries" }

// ResourceID returns the object key (the natural primary key for an
// entry inside a bucket).
func (e entryResource) ResourceID() string { return e.Key }

// MarshalJSON renders the attributes block. We don't put struct JSON
// tags on Entry itself because the domain type is consumed by other
// (snake_case-agnostic) callers; the wire-shape adaptation lives here.
func (e entryResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key          string    `json:"key"`
		Size         int64     `json:"size"`
		LastModified time.Time `json:"last_modified"`
		ContentType  string    `json:"content_type"`
		ETag         string    `json:"etag"`
	}{
		Key:          e.Key,
		Size:         e.Size,
		LastModified: e.LastModified,
		ContentType:  e.ContentType,
		ETag:         e.ETag,
	})
}

// prefixResource is the JSON:API wrapper for a Prefix. Coexists with
// entryResource in the same data[] array.
type prefixResource struct {
	Prefix
}

// ResourceType returns the canonical JSON:API type string.
func (prefixResource) ResourceType() string { return "object_prefixes" }

// ResourceID returns the prefix string (including trailing delimiter).
func (p prefixResource) ResourceID() string { return p.Name }

// MarshalJSON renders the attributes block for a common-prefix entry.
func (p prefixResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Prefix string `json:"prefix"`
	}{Prefix: p.Name})
}

// shareLinkResource is the JSON:API wrapper for a ShareLink. The ID is
// the URL itself (it's the only unique identifier the resource has);
// callers MUST treat the URL as a secret and never log or audit it.
type shareLinkResource struct {
	ShareLink
}

// ResourceType returns the canonical JSON:API type string.
func (shareLinkResource) ResourceType() string { return "object_share_links" }

// ResourceID returns the URL. JSON:API needs a string ID and a share
// link has no other natural key; the URL doubles as the resource ID for
// the duration of the response.
func (s shareLinkResource) ResourceID() string { return s.URL }

// MarshalJSON renders the attributes block.
func (s shareLinkResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expires_at"`
	}{URL: s.URL, ExpiresAt: s.ExpiresAt})
}

// ShareLinkRequest is the body accepted by POST /share-links.
type ShareLinkRequest struct {
	Key            string `json:"key"`
	ExpiresSeconds int    `json:"expires_seconds"`
}

// RestoreVersionRequest is the body accepted by POST /restore-version.
type RestoreVersionRequest struct {
	Key       string `json:"key"`
	VersionID string `json:"version_id"`
}

// ConfirmRequest is the body accepted by DELETE /version. The Confirm
// field must be true to authorise a permanent version delete.
type ConfirmRequest struct {
	Confirm bool `json:"confirm"`
}

// UndeleteRequest is the body accepted by POST /undelete.
type UndeleteRequest struct {
	Key string `json:"key"`
}

// BulkDeleteRequest is the body accepted by POST /objects/bulk-delete.
// At least one of Keys or Prefixes must be non-empty; DryRun selects the
// count-only preview vs. the real delete.
type BulkDeleteRequest struct {
	Keys     []string `json:"keys"`
	Prefixes []string `json:"prefixes"`
	DryRun   bool     `json:"dry_run"`
}

// versionResource is the JSON:API wrapper for an ObjectVersion. The wire
// type is "object_versions"; the resource ID combines key and version_id
// so it is unique within a bucket's version history.
type versionResource struct {
	ObjectVersion
}

// ResourceType returns the canonical JSON:API type string.
func (versionResource) ResourceType() string { return "object_versions" }

// ResourceID returns a composite "<key>@<version_id>" string that is
// unique within a bucket's version history.
func (v versionResource) ResourceID() string { return v.Key + "@" + v.VersionID }

// MarshalJSON renders the attributes block. Size is a nullable *int64
// (nil for delete markers). Fields with zero/empty values that are
// semantically empty (etag, content_type) use omitempty to keep delete-
// marker payloads lean.
func (v versionResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key            string    `json:"key"`
		VersionID      string    `json:"version_id"`
		Size           *int64    `json:"size"`
		LastModified   time.Time `json:"last_modified"`
		ETag           string    `json:"etag,omitempty"`
		ContentType    string    `json:"content_type,omitempty"`
		IsLatest       bool      `json:"is_latest"`
		IsDeleteMarker bool      `json:"is_delete_marker"`
	}{
		Key:            v.Key,
		VersionID:      v.VersionID,
		Size:           v.Size,
		LastModified:   v.LastModified,
		ETag:           v.ETag,
		ContentType:    v.ContentType,
		IsLatest:       v.IsLatest,
		IsDeleteMarker: v.IsDeleteMarker,
	})
}
