package lifecycle

import (
	"encoding/json"
)

// LifecycleRuleResource is the JSON:API resource wrapper for a Rule.
// The transport layer encodes it via internal/jsonapi.Encoder. Per
// api-contracts.md §lifecycle-rules the attributes object differs
// between managed and unmanaged rules: managed rules expose the
// structured (kind, days, prefix) trio, unmanaged rules expose only a
// human-readable summary string.
type LifecycleRuleResource struct {
	Rule
}

// ResourceType returns the canonical JSON:API type string.
func (LifecycleRuleResource) ResourceType() string { return "lifecycle_rules" }

// ResourceID returns the rule's MinIO-side ID (the natural primary key
// inside a bucket's lifecycle configuration).
func (r LifecycleRuleResource) ResourceID() string { return r.Rule.ID }

// MarshalJSON renders the attributes block. The shape is conditional
// on the Managed flag because the two flavours expose disjoint
// information: a managed rule's structured fields would be misleading
// on an unmanaged rule (the values are zero/empty by construction),
// and an unmanaged rule's Summary doesn't apply to a managed rule.
//
// We don't put struct JSON tags on Rule itself because the domain
// type is consumed by other (snake_case-agnostic) callers; the
// wire-shape adaptation lives here.
func (r LifecycleRuleResource) MarshalJSON() ([]byte, error) {
	if r.Rule.Managed {
		return json.Marshal(struct {
			Managed bool   `json:"managed"`
			Kind    string `json:"kind"`
			Days    int    `json:"days"`
			Prefix  string `json:"prefix"`
		}{
			Managed: true,
			Kind:    r.Rule.Kind,
			Days:    r.Rule.Days,
			Prefix:  r.Rule.Prefix,
		})
	}
	return json.Marshal(struct {
		Managed bool   `json:"managed"`
		Summary string `json:"summary"`
	}{
		Managed: false,
		Summary: r.Rule.Summary,
	})
}

// CreateRequest is the attributes block accepted by
// POST /api/v1/buckets/{name}/lifecycle-rules. The wire contract is
// JSON:API single-resource style (`{data:{type,attributes:{…}}}`),
// decoded via jsonapi.Decoder.Single. Only kind="expiration" is
// accepted in v1; the handler validates that explicitly so a future
// "transition" kind can land without changing the wire shape.
type CreateRequest struct {
	Kind   string `json:"kind"`
	Days   int    `json:"days"`
	Prefix string `json:"prefix"`
}
