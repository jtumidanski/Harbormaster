package users

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
	"github.com/jtumidanski/Harbormaster/internal/policies"
)

// actorFromRequest pulls the authenticated username and source IP off the
// session context populated by auth.RequireSession. Mirrors the bucket
// helper so cross-package navigation is cheap.
func actorFromRequest(r *http.Request) (string, string) {
	if si, ok := auth.FromContext(r.Context()); ok {
		return si.Username, si.SourceIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "", host
}

// Routes returns a chi sub-router function that mounts the users-domain
// endpoints under whatever parent path the caller picks.
//
// Resource endpoints (collection + single) render JSON:API documents.
// Action endpoints (status, delete, policies, revoke service account)
// render plain JSON with the {error:{code,message,details?}} envelope.
//
// sa is the (optional) ServiceAccountProcessor used by the nested
// /users/{access_key}/service-accounts and the standalone
// /service-accounts/{access_key} revoke route. Pass nil to disable the
// service-account surface (the routes then return 501).
func Routes(p *Processor, sa *ServiceAccountProcessor) func(chi.Router) {
	h := &handler{p: p, sa: sa, enc: jsonapi.NewEncoder(), dec: jsonapi.NewDecoder()}
	return func(r chi.Router) {
		r.Get("/users", h.list)
		r.Post("/users", h.create)
		r.Get("/users/{access_key}", h.get)
		r.Put("/users/{access_key}/status", h.setStatus)
		r.Delete("/users/{access_key}", h.delete)
		r.Put("/users/{access_key}/policies", h.updatePolicies)
		r.Get("/users/{access_key}/service-accounts", h.listServiceAccounts)
		r.Post("/users/{access_key}/service-accounts", h.createServiceAccount)
		r.Delete("/service-accounts/{access_key}", h.revokeServiceAccount)
		r.Get("/policy-templates", h.listPolicyTemplates)
	}
}

// handler bundles the processors with the JSON:API codec instances so each
// HTTP method doesn't have to reconstruct them per-request.
type handler struct {
	p   *Processor
	sa  *ServiceAccountProcessor
	enc *jsonapi.Encoder
	dec *jsonapi.Decoder
}

// list returns every IAM user as a JSON:API collection document.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	us, err := h.p.List(r.Context())
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	resources := make([]jsonapi.Resource, len(us))
	for i, u := range us {
		resources[i] = UserResource{User: u}
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{TotalRecords: len(us), TotalPages: 1},
	}, nil)
}

// create accepts a JSON:API single-resource document with attributes
// matching CreateUserRequest, registers the user, and returns the freshly
// created user — including the one-time secret_key — on 201.
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var attrs CreateUserRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	if err := ValidateAccessKey(attrs.AccessKey); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusUnprocessableEntity,
			"invalid_access_key", err.Error()).WithPointer("/data/attributes/access_key"))
		return
	}
	actor, ip := actorFromRequest(r)
	u, secret, err := h.p.Create(r.Context(), attrs.AccessKey, attrs.ToTemplateRefs(), actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusCreated, CreatedUserResource{User: u, SecretKey: secret})
}

// get returns the named user as a JSON:API single document.
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	ak := chi.URLParam(r, "access_key")
	u, err := h.p.Get(r.Context(), ak)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusOK, UserResource{User: u})
}

// setStatus is the action endpoint for PUT /users/{access_key}/status.
func (h *handler) setStatus(w http.ResponseWriter, r *http.Request) {
	ak := chi.URLParam(r, "access_key")
	var body StatusRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.SetStatus(r.Context(), ak, body.Enabled, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// delete reads a {confirm_access_key} body and invokes the processor's
// destructive-action guard.
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	ak := chi.URLParam(r, "access_key")
	var body DeleteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.Delete(r.Context(), ak, body.ConfirmAccessKey, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updatePolicies is the action endpoint for PUT /users/{access_key}/policies.
func (h *handler) updatePolicies(w http.ResponseWriter, r *http.Request) {
	ak := chi.URLParam(r, "access_key")
	var body UpdatePoliciesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.UpdatePolicies(r.Context(), ak, body.ToTemplateRefs(), actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listServiceAccounts returns every service account belonging to a parent
// user as a JSON:API collection document.
func (h *handler) listServiceAccounts(w http.ResponseWriter, r *http.Request) {
	if h.sa == nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusNotImplemented,
			"not_implemented", "service-account routes not wired"))
		return
	}
	parent := chi.URLParam(r, "access_key")
	sas, err := h.sa.List(r.Context(), parent)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	resources := make([]jsonapi.Resource, len(sas))
	for i, s := range sas {
		resources[i] = ServiceAccountResource{ServiceAccount: s}
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{TotalRecords: len(sas), TotalPages: 1},
	}, nil)
}

// createServiceAccount mints a new child credential for the named parent
// user. The response includes the one-time secret_key.
func (h *handler) createServiceAccount(w http.ResponseWriter, r *http.Request) {
	if h.sa == nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusNotImplemented,
			"not_implemented", "service-account routes not wired"))
		return
	}
	parent := chi.URLParam(r, "access_key")
	var attrs CreateServiceAccountRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	actor, ip := actorFromRequest(r)
	sa, secret, err := h.sa.Create(r.Context(), parent, attrs.Name, attrs.Description, attrs.Override(), actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusCreated, CreatedServiceAccountResource{
		ServiceAccount: sa,
		SecretKey:      secret,
	})
}

// revokeServiceAccount tears down a child credential. Returns 204 on
// success.
func (h *handler) revokeServiceAccount(w http.ResponseWriter, r *http.Request) {
	if h.sa == nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusNotImplemented,
			"not_implemented", "service-account routes not wired"))
		return
	}
	ak := chi.URLParam(r, "access_key")
	actor, ip := actorFromRequest(r)
	if err := h.sa.Revoke(r.Context(), ak, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listPolicyTemplates returns the three bundled templates as a JSON:API
// collection document. The response payload carries each template's
// description and (optional) params schema so the SPA can render an
// add-template subform without reaching for a second endpoint.
func (h *handler) listPolicyTemplates(w http.ResponseWriter, r *http.Request) {
	tmpls := policies.All()
	resources := make([]jsonapi.Resource, len(tmpls))
	for i, t := range tmpls {
		resources[i] = PolicyTemplateResource{Template: t}
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{TotalRecords: len(tmpls), TotalPages: 1},
	}, nil)
}

// PolicyTemplateResource is the JSON:API resource wrapper for a bundled
// policy template. Defined alongside the handler because the template
// type lives in internal/policies; we wrap rather than altering the
// upstream type so the policies package stays free of JSON:API knowledge.
type PolicyTemplateResource struct {
	Template policies.Template
}

// ResourceType returns the canonical JSON:API type string.
func (r PolicyTemplateResource) ResourceType() string { return "policy-templates" }

// ResourceID returns the template name (the natural primary key).
func (r PolicyTemplateResource) ResourceID() string { return r.Template.Name }

// MarshalJSON shapes the on-the-wire payload. ParamsSchema is omitted
// when nil so parameterless templates serialise cleanly.
func (r PolicyTemplateResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		ParamsSchema json.RawMessage `json:"params_schema,omitempty"`
	}
	return json.Marshal(alias{
		Name:         r.Template.Name,
		Description:  r.Template.Description,
		ParamsSchema: r.Template.ParamsSchema,
	})
}

// writeSingle encodes a single JSON:API resource document at the chosen
// status code. Centralises the Content-Type and WriteHeader ordering.
func writeSingle(w http.ResponseWriter, enc *jsonapi.Encoder, status int, res jsonapi.Resource) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = enc.Single(w, res, nil)
}

// Compile-time assertion: errors.As is used by the apierror.Write
// fallback path; keep the import live even when the action handlers don't
// reference it directly.
var _ = errors.As
