package users

import "encoding/json"

// UserResource is the JSON:API resource wrapper for a User. The transport
// layer encodes it via internal/jsonapi.Encoder. Note that this resource
// is used for the List and Get responses and intentionally does NOT
// include a secret_key field — the secret is only ever exposed via the
// dedicated CreateUserResponse.
type UserResource struct {
	User
}

// ResourceType returns the canonical JSON:API type string.
func (r UserResource) ResourceType() string { return "users" }

// ResourceID returns the access key (the natural primary key on MinIO).
func (r UserResource) ResourceID() string { return r.AccessKey }

// MarshalJSON shapes the on-the-wire payload for a User. We expose the
// status, attached templates, attached policies, and other policies but
// never the secret — MinIO does not return the secret on GetUserInfo and
// Harbormaster never caches it.
func (r UserResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		AccessKey         string         `json:"access_key"`
		Status            string         `json:"status"`
		AttachedTemplates []TemplateWire `json:"attached_templates"`
		AttachedPolicies  []string       `json:"attached_policies"`
		OtherPolicies     []string       `json:"other_policies"`
	}
	out := alias{
		AccessKey:    r.AccessKey,
		Status:       r.Status,
		OtherPolicies: r.OtherPolicies,
	}
	if out.OtherPolicies == nil {
		out.OtherPolicies = []string{}
	}
	out.AttachedTemplates = make([]TemplateWire, 0, len(r.AttachedTemplates))
	for _, t := range r.AttachedTemplates {
		out.AttachedTemplates = append(out.AttachedTemplates, TemplateWire(t))
	}
	if r.AttachedPolicies == nil {
		out.AttachedPolicies = []string{}
	} else {
		out.AttachedPolicies = r.AttachedPolicies
	}
	return json.Marshal(out)
}

// TemplateWire is the wire shape of a TemplateRef. Params is omitted from
// the marshaled JSON when nil so parameterless templates serialise as
// {"name":"read-only"} rather than {"name":"read-only","params":null}.
type TemplateWire struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params,omitempty"`
}

// CreateUserRequest is the attributes payload for POST /users.
type CreateUserRequest struct {
	AccessKey string         `json:"access_key"`
	Templates []TemplateWire `json:"templates,omitempty"`
}

// ToTemplateRefs converts the wire DTO into the domain slice.
func (r CreateUserRequest) ToTemplateRefs() []TemplateRef {
	if len(r.Templates) == 0 {
		return nil
	}
	out := make([]TemplateRef, 0, len(r.Templates))
	for _, t := range r.Templates {
		out = append(out, TemplateRef(t))
	}
	return out
}

// CreateUserResponseAttrs is the JSON:API attributes payload for the
// one-time secret reveal on POST /users. The secret_key field is the
// whole reason this struct exists alongside UserResource — it is rendered
// exactly once on the 201 response and never persisted server-side.
type CreateUserResponseAttrs struct {
	User
	SecretKey string `json:"secret_key"`
}

// CreatedUserResource wraps CreateUserResponseAttrs into a JSON:API
// resource. The resource type is the same "users" so the SPA's
// type-discriminator switch does not need a special branch.
type CreatedUserResource struct {
	User      User
	SecretKey string
}

// ResourceType returns the canonical JSON:API type string.
func (r CreatedUserResource) ResourceType() string { return "users" }

// ResourceID returns the access key.
func (r CreatedUserResource) ResourceID() string { return r.User.AccessKey }

// MarshalJSON shapes the on-the-wire payload for a freshly created user,
// including the one-time secret_key.
func (r CreatedUserResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		AccessKey         string         `json:"access_key"`
		Status            string         `json:"status"`
		AttachedTemplates []TemplateWire `json:"attached_templates"`
		AttachedPolicies  []string       `json:"attached_policies"`
		OtherPolicies     []string       `json:"other_policies"`
		SecretKey         string         `json:"secret_key"`
	}
	out := alias{
		AccessKey:    r.User.AccessKey,
		Status:       r.User.Status,
		OtherPolicies: r.User.OtherPolicies,
		SecretKey:    r.SecretKey,
	}
	if out.OtherPolicies == nil {
		out.OtherPolicies = []string{}
	}
	out.AttachedTemplates = make([]TemplateWire, 0, len(r.User.AttachedTemplates))
	for _, t := range r.User.AttachedTemplates {
		out.AttachedTemplates = append(out.AttachedTemplates, TemplateWire(t))
	}
	if r.User.AttachedPolicies == nil {
		out.AttachedPolicies = []string{}
	} else {
		out.AttachedPolicies = r.User.AttachedPolicies
	}
	return json.Marshal(out)
}

// StatusRequest is the body accepted by PUT /users/{access_key}/status.
type StatusRequest struct {
	Enabled bool `json:"enabled"`
}

// DeleteUserRequest is the body accepted by DELETE /users/{access_key}.
// (DELETE permits a body per RFC 7231 §4.3.5; Harbormaster uses it to
// carry the destructive-action confirmation token.)
type DeleteUserRequest struct {
	ConfirmAccessKey string `json:"confirm_access_key"`
}

// UpdatePoliciesRequest is the body accepted by
// PUT /users/{access_key}/policies.
type UpdatePoliciesRequest struct {
	Templates []TemplateWire `json:"templates"`
	Policies  []string       `json:"policies"`
}

// ToTemplateRefs converts the wire DTO into the domain slice.
func (r UpdatePoliciesRequest) ToTemplateRefs() []TemplateRef {
	out := make([]TemplateRef, 0, len(r.Templates))
	for _, t := range r.Templates {
		out = append(out, TemplateRef(t))
	}
	return out
}
