package policies

import (
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// actorFromRequest pulls the authenticated username and source IP off the
// session context populated by auth.RequireSession. Mirrors the users helper
// so cross-package navigation is cheap.
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

// Routes returns a chi sub-router function that mounts the policies-domain
// endpoints under whatever parent path the caller picks.
// All resource endpoints render JSON:API documents.
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p, enc: jsonapi.NewEncoder(), dec: jsonapi.NewDecoder()}
	return func(r chi.Router) {
		r.Get("/policies", h.list)
		r.Post("/policies", h.create)
		r.Get("/policies/{name}", h.get)
		r.Put("/policies/{name}", h.update)
		r.Delete("/policies/{name}", h.delete)
	}
}

// handler bundles the processor with the JSON:API codec instances so each
// HTTP method doesn't have to reconstruct them per-request.
type handler struct {
	p   *Processor
	enc *jsonapi.Encoder
	dec *jsonapi.Decoder
}

// list returns every canned policy as a JSON:API collection document.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	ps, err := h.p.List(r.Context())
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	resources := make([]jsonapi.Resource, len(ps))
	for i, pol := range ps {
		resources[i] = policyResource{Policy: pol}
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{TotalRecords: len(ps), TotalPages: 1},
	}, nil)
}

// get returns the named policy (including full document) as a JSON:API single
// document.
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	detail, err := h.p.Get(r.Context(), name)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusOK, policyDetailResource{PolicyDetail: detail})
}

// create accepts a JSON:API single-resource document with attributes matching
// CreateRequest, validates, and returns the new policy on 201.
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var attrs CreateRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	actor, ip := actorFromRequest(r)
	pol, err := h.p.Create(r.Context(), attrs.Name, []byte(attrs.Document), actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusCreated, policyResource{Policy: pol})
}

// update accepts a JSON:API single-resource document with attributes matching
// UpdateRequest and replaces the named policy document.
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var attrs UpdateRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.Update(r.Context(), name, []byte(attrs.Document), actor, ip); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	// Re-fetch to return the updated policy as a resource document.
	detail, err := h.p.Get(r.Context(), name)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusOK, policyResource{Policy: detail.Policy})
}

// delete removes the named policy and returns 204 on success.
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	actor, ip := actorFromRequest(r)
	if err := h.p.Delete(r.Context(), name, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
