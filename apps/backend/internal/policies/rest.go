package policies

import "encoding/json"

// policyResource is the JSON:API resource wrapper for a Policy. Used for
// List and Get (non-detail) responses.
type policyResource struct {
	Policy
}

// ResourceType returns the canonical JSON:API type string.
func (r policyResource) ResourceType() string { return "policies" }

// ResourceID returns the policy name (the natural primary key).
func (r policyResource) ResourceID() string { return r.Name }

// MarshalJSON shapes the on-the-wire payload.
func (r policyResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		Name             string `json:"name"`
		Origin           string `json:"origin"`
		Editable         bool   `json:"editable"`
		StatementSummary string `json:"statement_summary"`
	}
	return json.Marshal(alias{
		Name:             r.Name,
		Origin:           r.Origin,
		Editable:         r.Editable,
		StatementSummary: r.StatementSummary,
	})
}

// policyDetailResource is the JSON:API resource wrapper for a PolicyDetail.
// Used for Get responses that include the full IAM document.
type policyDetailResource struct {
	PolicyDetail
}

// ResourceType returns the canonical JSON:API type string.
func (r policyDetailResource) ResourceType() string { return "policies" }

// ResourceID returns the policy name.
func (r policyDetailResource) ResourceID() string { return r.Name }

// MarshalJSON shapes the on-the-wire payload including the full document.
func (r policyDetailResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		Name             string          `json:"name"`
		Origin           string          `json:"origin"`
		Editable         bool            `json:"editable"`
		StatementSummary string          `json:"statement_summary"`
		Document         json.RawMessage `json:"document"`
	}
	return json.Marshal(alias{
		Name:             r.Name,
		Origin:           r.Origin,
		Editable:         r.Editable,
		StatementSummary: r.StatementSummary,
		Document:         r.Document,
	})
}

// CreateRequest is the attributes payload for POST /policies.
type CreateRequest struct {
	Name     string          `json:"name"`
	Document json.RawMessage `json:"document"`
}

// UpdateRequest is the attributes payload for PUT /policies/{name}.
type UpdateRequest struct {
	Document json.RawMessage `json:"document"`
}
