package buckets

import (
	"encoding/json"
	"time"
)

// BucketResource is the JSON:API resource wrapper for a Bucket. The
// transport layer encodes it via internal/jsonapi.Encoder.
type BucketResource struct {
	Bucket
}

// ResourceType returns the canonical JSON:API type string.
func (r BucketResource) ResourceType() string { return "buckets" }

// ResourceID returns the bucket name (the natural primary key).
func (r BucketResource) ResourceID() string { return r.Name }

// MarshalJSON renders the attributes block in the snake_case shape the API
// contract and the SPA consume. The embedded Bucket model carries no JSON
// tags (its Go field names are PascalCase), so the encoder must NOT fall back
// to default struct marshalling — doing so emits "ObjectCount"/"CreatedAt"
// etc., which the frontend reads as undefined and crashes the bucket list.
// `quota` is emitted as null (not omitted) when unset to match `quota: Quota|null`.
func (r BucketResource) MarshalJSON() ([]byte, error) {
	type quotaAttrs struct {
		Kind      QuotaKind `json:"kind"`
		Bytes     int64     `json:"bytes"`
		UsedBytes int64     `json:"used_bytes"`
	}
	var q *quotaAttrs
	if r.Quota != nil {
		q = &quotaAttrs{Kind: r.Quota.Kind, Bytes: r.Quota.Bytes, UsedBytes: r.Quota.UsedBytes}
	}
	return json.Marshal(struct {
		Name              string       `json:"name"`
		CreatedAt         time.Time    `json:"created_at"`
		EstimatedBytes    int64        `json:"estimated_bytes"`
		ObjectCount       int64        `json:"object_count"`
		VersioningEnabled bool         `json:"versioning_enabled"`
		HasLifecycleRules bool         `json:"has_lifecycle_rules"`
		PublicAccess      PublicAccess `json:"public_access"`
		Quota             *quotaAttrs  `json:"quota"`
	}{
		Name:              r.Name,
		CreatedAt:         r.CreatedAt,
		EstimatedBytes:    r.EstimatedBytes,
		ObjectCount:       r.ObjectCount,
		VersioningEnabled: r.VersioningEnabled,
		HasLifecycleRules: r.HasLifecycleRules,
		PublicAccess:      r.PublicAccess,
		Quota:             q,
	})
}

// CreateRequest is the body accepted by POST /api/v1/buckets.
type CreateRequest struct {
	Name              string       `json:"name"`
	VersioningEnabled bool         `json:"versioning_enabled"`
	PublicAccess      string       `json:"public_access,omitempty"`
	Quota             *CreateQuota `json:"quota,omitempty"`
	LifecycleTemplate string       `json:"lifecycle_template,omitempty"`
}

// CreateQuota is the nested quota block carried by CreateRequest. Pulled
// out of an anonymous struct so handler tests can construct it directly.
type CreateQuota struct {
	Kind  string `json:"kind"`
	Bytes int64  `json:"bytes"`
}

// ToOpts converts the wire DTO into the processor's CreateOpts value,
// validating the enumerated string fields along the way.
func (r CreateRequest) ToOpts() CreateOpts {
	opts := CreateOpts{
		VersioningEnabled: r.VersioningEnabled,
		PublicAccess:      PublicAccess(r.PublicAccess),
		LifecycleTemplate: r.LifecycleTemplate,
	}
	if r.Quota != nil {
		opts.Quota = &Quota{
			Kind:  QuotaKind(r.Quota.Kind),
			Bytes: r.Quota.Bytes,
		}
	}
	return opts
}

// PublicAccessRequest is the body accepted by
// POST /api/v1/buckets/{name}/public-access.
type PublicAccessRequest struct {
	Mode        string `json:"mode"`
	ConfirmName string `json:"confirm_name"`
}

// QuotaRequest is the body accepted by POST /api/v1/buckets/{name}/quota.
type QuotaRequest struct {
	Kind  string `json:"kind"`
	Bytes int64  `json:"bytes"`
}

// DeleteRequest is the body accepted by DELETE /api/v1/buckets/{name}.
// (DELETE permits a body per RFC 7231 §4.3.5; Harbormaster uses it to
// carry the destructive-action confirmation token.)
type DeleteRequest struct {
	ConfirmName string `json:"confirm_name"`
}

// VersioningRequest is the body accepted by
// POST /api/v1/buckets/{name}/versioning.
type VersioningRequest struct {
	Enabled bool `json:"enabled"`
}
