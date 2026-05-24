// Package objects owns the object-level view of MinIO: listing object
// keys and common prefixes, uploading object bodies (subject to a
// configurable byte cap), deleting individual keys, downloading object
// bytes either by streaming through Harbormaster (proxy mode) or by
// redirecting the browser to a presigned MinIO URL (direct mode), and
// minting short-lived share-link presigned URLs.
//
// Like the buckets package, the source of truth is MinIO itself —
// nothing in this package persists local state.
package objects

import "time"

// Entry is the immutable read view of a single object key returned by a
// ListObjectsV2 call. Sizes are the raw MinIO-reported byte counts; we
// don't reinterpret them.
type Entry struct {
	Key          string
	Size         int64
	LastModified time.Time
	ContentType  string
	ETag         string
}

// Prefix is a common-prefix entry returned when a list call supplies a
// delimiter. The Name is the full prefix string MinIO emits (including
// the trailing delimiter, e.g. "photos/2025/").
type Prefix struct {
	Name string
}

// ListResult is the paginated outcome of a single List call. NextToken
// is the opaque continuation token to feed back into the next call; the
// empty string indicates the listing is exhausted.
type ListResult struct {
	Entries   []Entry
	Prefixes  []Prefix
	NextToken string
}

// ShareLink describes a freshly-minted presigned download URL together
// with the server-side expiry timestamp the operator can show in the UI.
// The URL is sensitive — never persist it or include it in audit
// payloads.
type ShareLink struct {
	URL       string
	ExpiresAt time.Time
}
