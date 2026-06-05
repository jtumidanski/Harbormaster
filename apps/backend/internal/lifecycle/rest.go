package lifecycle

import (
	"encoding/json"
)

// RuleResource is the JSON:API resource wrapper for a Rule.
// The transport layer encodes it via internal/jsonapi.Encoder. Per
// api-contracts.md §lifecycle-rules the attributes object differs
// between managed and unmanaged rules: managed rules expose the
// structured (kind, days, prefix) trio, unmanaged rules expose only a
// human-readable summary string.
type RuleResource struct {
	Rule
}

// ResourceType returns the canonical JSON:API type string.
func (RuleResource) ResourceType() string { return "lifecycle_rules" }

// ResourceID returns the rule's MinIO-side ID (the natural primary key
// inside a bucket's lifecycle configuration).
func (r RuleResource) ResourceID() string { return r.ID }

// MarshalJSON renders the attributes block. The shape is conditional
// on the Managed flag and, for managed rules, on the Kind value,
// because each lifecycle kind exposes a disjoint set of configuration
// fields. Unmanaged rules expose only a human-readable summary string.
//
// Managed shapes by kind:
//   - KindExpiration           → {managed, kind, days, prefix}
//   - KindNoncurrentExpiration → {managed, kind, noncurrent_days, newer_noncurrent_versions, prefix}
//   - KindAbortIncompleteMPU   → {managed, kind, days_after_initiation, prefix}
//
// We don't put struct JSON tags on Rule itself because the domain
// type is consumed by other (snake_case-agnostic) callers; the
// wire-shape adaptation lives here.
func (r RuleResource) MarshalJSON() ([]byte, error) {
	if r.Managed {
		switch r.Kind {
		case KindNoncurrentExpiration:
			return json.Marshal(struct {
				Managed                 bool   `json:"managed"`
				Kind                    string `json:"kind"`
				NoncurrentDays          int    `json:"noncurrent_days"`
				NewerNoncurrentVersions int    `json:"newer_noncurrent_versions"`
				Prefix                  string `json:"prefix"`
			}{
				Managed:                 true,
				Kind:                    r.Kind,
				NoncurrentDays:          r.NoncurrentDays,
				NewerNoncurrentVersions: r.NewerNoncurrentVersions,
				Prefix:                  r.Prefix,
			})
		case KindAbortIncompleteMPU:
			return json.Marshal(struct {
				Managed             bool   `json:"managed"`
				Kind                string `json:"kind"`
				DaysAfterInitiation int    `json:"days_after_initiation"`
				Prefix              string `json:"prefix"`
			}{
				Managed:             true,
				Kind:                r.Kind,
				DaysAfterInitiation: r.DaysAfterInitiation,
				Prefix:              r.Prefix,
			})
		default: // KindExpiration and any future forward-compat kind
			return json.Marshal(struct {
				Managed bool   `json:"managed"`
				Kind    string `json:"kind"`
				Days    int    `json:"days"`
				Prefix  string `json:"prefix"`
			}{
				Managed: true,
				Kind:    r.Kind,
				Days:    r.Days,
				Prefix:  r.Prefix,
			})
		}
	}
	return json.Marshal(struct {
		Managed bool   `json:"managed"`
		Summary string `json:"summary"`
	}{
		Managed: false,
		Summary: r.Summary,
	})
}

// CreateRequest is the attributes block accepted by
// POST /api/v1/buckets/{name}/lifecycle-rules. The wire contract is
// JSON:API single-resource style (`{data:{type,attributes:{…}}}`),
// decoded via jsonapi.Decoder.Single.
//
// The superset struct carries every kind's fields. The handler
// switches on Kind to dispatch to the appropriate Processor method and
// validates that only the three supported kinds are accepted. Fields
// not relevant to the chosen kind are ignored.
type CreateRequest struct {
	Kind                    string `json:"kind"`
	Days                    int    `json:"days"`
	NoncurrentDays          int    `json:"noncurrent_days"`
	NewerNoncurrentVersions int    `json:"newer_noncurrent_versions"`
	DaysAfterInitiation     int    `json:"days_after_initiation"`
	Prefix                  string `json:"prefix"`
}
