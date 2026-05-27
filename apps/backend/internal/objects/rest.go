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
