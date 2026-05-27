package lifecycle

import (
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// actorFromRequest pulls the authenticated username and source IP off the
// session context populated by auth.RequireSession. Falls back to the raw
// remote address when no session is attached.
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

// Routes returns a chi sub-router function that mounts the
// lifecycle-rule endpoints under whatever parent path the caller
// picks. The intended mount point is the protected /api/v1 surface so
// the full paths are:
//
//   - GET    /buckets/{name}/lifecycle-rules          → list
//   - POST   /buckets/{name}/lifecycle-rules          → create (managed expiration)
//   - DELETE /buckets/{name}/lifecycle-rules/{rule_id} → delete (any rule, managed or not)
//
// All endpoints render application/vnd.api+json via the jsonapi
// codec; error envelopes follow the JSON:API errors[] shape via
// apierror.StyleJSONAPI to stay consistent with the rest of the
// resource-style surface (buckets, objects' list/upload/share-link).
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p, enc: jsonapi.NewEncoder(), dec: jsonapi.NewDecoder()}
	return func(r chi.Router) {
		r.Get("/buckets/{name}/lifecycle-rules", h.list)
		r.Post("/buckets/{name}/lifecycle-rules", h.create)
		r.Delete("/buckets/{name}/lifecycle-rules/{rule_id}", h.delete)
	}
}

// handler bundles the processor with the JSON:API codec instances so
// each HTTP method doesn't have to reconstruct them per-request.
type handler struct {
	p   *Processor
	enc *jsonapi.Encoder
	dec *jsonapi.Decoder
}

// list returns every rule attached to the bucket as a JSON:API
// collection document. An empty rule set serialises to data:[] so the
// SPA can render the "no rules" empty state without distinguishing it
// from a missing-config error.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "name")
	rules, err := h.p.List(r.Context(), bucket)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	resources := make([]jsonapi.Resource, len(rules))
	for i, rule := range rules {
		resources[i] = RuleResource{Rule: rule}
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{TotalRecords: len(rules), TotalPages: 1},
	}, nil)
}

// create accepts a JSON:API single-resource document with attributes
// matching CreateRequest and returns the newly created rule as a
// JSON:API single document on 201. Only kind="expiration" is accepted
// in v1; any other kind surfaces as a typed 422 envelope with
// source.pointer set to /data/attributes/kind so the SPA can attach
// the message to the right form field.
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "name")
	var attrs CreateRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	if attrs.Kind != "expiration" {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusUnprocessableEntity,
			"invalid_lifecycle_rule",
			"only kind=\"expiration\" is supported").WithPointer("/data/attributes/kind"))
		return
	}
	actor, ip := actorFromRequest(r)
	rule, err := h.p.Create(r.Context(), bucket, attrs.Days, attrs.Prefix, actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusCreated, RuleResource{Rule: rule})
}

// delete removes the rule identified by the {rule_id} path param.
// Success is 204 No Content with no body, whether or not the rule
// actually existed — processor.Delete treats a missing rule as a
// no-op so a double-DELETE from the UI does not raise a spurious 404.
// Errors render via the JSON:API errors[] envelope.
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "name")
	ruleID := chi.URLParam(r, "rule_id")
	actor, ip := actorFromRequest(r)
	if err := h.p.Delete(r.Context(), bucket, ruleID, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeSingle encodes a single JSON:API resource document at the
// chosen status code. Centralises the Content-Type and WriteHeader
// ordering so every resource handler emits the same envelope shape.
func writeSingle(w http.ResponseWriter, enc *jsonapi.Encoder, status int, res jsonapi.Resource) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = enc.Single(w, res, nil)
}

