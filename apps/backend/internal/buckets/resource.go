package buckets

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// actorFromRequest pulls the authenticated username and source IP off the
// session context populated by auth.RequireSession. Falls back to the raw
// remote address when no session is attached (defence in depth — these
// routes are mounted behind RequireSession, so the empty-actor path is
// only exercised in tests that drive the handler directly).
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

// Routes returns a chi sub-router function that mounts the bucket
// endpoints under whatever parent path the caller picks.
//
// Resource endpoints (collection + single) render JSON:API documents and
// the JSON:API errors[] envelope. Action endpoints render plain JSON and
// the {error:{code,message,details?}} envelope. DELETE on a bucket is
// JSON:API in path style but per api-contracts.md its 409 / 403 error
// bodies use the action envelope (the SPA reads `error.details` to drive
// the Empty-bucket affordance), so writeAction is used for those too.
//
// When empty is non-nil, POST /buckets/{name}/empty is bound to the live
// SSE handler (T3.5). Otherwise the route falls back to the stub-501
// envelope so the surface remains predictable in test setups that do not
// wire the bucketempty service.
func Routes(p *Processor, empty *EmptyHandler) func(chi.Router) {
	h := &handler{p: p, enc: jsonapi.NewEncoder(), dec: jsonapi.NewDecoder()}
	return func(r chi.Router) {
		r.Get("/buckets", h.list)
		r.Post("/buckets", h.create)
		r.Get("/buckets/{name}", h.get)
		r.Delete("/buckets/{name}", h.delete)
		r.Put("/buckets/{name}/versioning", h.setVersioning)
		r.Put("/buckets/{name}/public-access", h.setPublicAccess)
		r.Put("/buckets/{name}/quota", h.setQuota)
		if empty != nil {
			r.Post("/buckets/{name}/empty", empty.ServeHTTP)
		} else {
			r.Post("/buckets/{name}/empty", h.empty)
		}
	}
}

// handler bundles the processor with the JSON:API codec instances so each
// HTTP method doesn't have to reconstruct them per-request.
type handler struct {
	p   *Processor
	enc *jsonapi.Encoder
	dec *jsonapi.Decoder
}

// list returns every bucket as a JSON:API collection document. Pagination
// is deferred to a later task — for now the whole set is returned in
// data[] with meta.page.total_records populated so the SPA contract stays
// stable.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	bs, err := h.p.List(r.Context())
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	resources := make([]jsonapi.Resource, len(bs))
	for i, b := range bs {
		resources[i] = BucketResource{Bucket: b}
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{TotalRecords: len(bs), TotalPages: 1},
	}, nil)
}

// create accepts a JSON:API single-resource document with attributes
// matching CreateRequest and returns the created bucket as a JSON:API
// single document on 201. Invalid bucket names surface as a JSON:API
// errors[] envelope with source.pointer set to the offending attribute.
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var attrs CreateRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	if err := ValidateBucketName(attrs.Name); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusUnprocessableEntity,
			"invalid_bucket_name", err.Error()).WithPointer("/data/attributes/name"))
		return
	}
	actor, ip := actorFromRequest(r)
	bucket, err := h.p.Create(r.Context(), attrs.Name, attrs.ToOpts(), actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, mapInvalidNameCode(err))
		return
	}
	writeSingle(w, h.enc, http.StatusCreated, BucketResource{Bucket: bucket})
}

// get returns the named bucket as a JSON:API single document, or a typed
// 404 envelope if MinIO reports the bucket as absent.
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	b, err := h.p.Get(r.Context(), name)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, mapInvalidNameCode(err))
		return
	}
	writeSingle(w, h.enc, http.StatusOK, BucketResource{Bucket: b})
}

// delete reads a {confirm_name} body (NOT JSON:API — see api-contracts.md
// §DELETE /api/v1/buckets/{name}), invokes the processor's destructive-
// action guard, and surfaces the result via the action envelope so the
// 409 bucket_not_empty body carries `error.details.object_count` (and the
// 403 confirm_name_mismatch body matches the same shape).
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.Delete(r.Context(), name, body.ConfirmName, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, mapInvalidNameCode(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setVersioning is the action endpoint for PUT /buckets/{name}/versioning.
// Returns 204 on success, action-envelope errors on failure.
func (h *handler) setVersioning(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body VersioningRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.SetVersioning(r.Context(), name, body.Enabled, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, mapInvalidNameCode(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setPublicAccess is the action endpoint for PUT /buckets/{name}/public-access.
// The processor enforces the confirm_name guard for public-read-write and
// materialises the canned policy JSON via internal/policies.
func (h *handler) setPublicAccess(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body PublicAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := h.p.SetPublicAccess(r.Context(), name, body.Mode, body.ConfirmName, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, mapInvalidNameCode(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setQuota is the action endpoint for PUT /buckets/{name}/quota. A
// kind:"none" body clears the quota (bytes is ignored). Other kinds
// require bytes > 0 — processor.SetQuota rejects bytes<=0 unless kind=none.
func (h *handler) setQuota(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body QuotaRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	// "none" is a wire-only sentinel: it maps to bytes=0 with the QuotaKind
	// inferred from MinIO's current state, but the simplest contract-correct
	// implementation is "clear the quota", which applyQuota does by passing
	// HardQuota with size=0. The processor doesn't know about "none" so we
	// translate here.
	actor, ip := actorFromRequest(r)
	if body.Kind == "none" {
		if err := h.p.SetQuota(r.Context(), name, QuotaKindHard, 0, actor, ip); err != nil {
			apierror.Write(w, apierror.StyleAction, mapInvalidNameCode(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.p.SetQuota(r.Context(), name, QuotaKind(body.Kind), body.Bytes, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, mapInvalidNameCode(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// empty is the POST /buckets/{name}/empty stub. The full SSE-streamed
// bulk-delete loop lands in T3.5; T3.3 only validates the body shape and
// returns a typed 501 so the route is reachable and request schemas are
// exercised end-to-end.
func (h *handler) empty(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConfirmName   string `json:"confirm_name"`
		PurgeVersions bool   `json:"purge_versions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	_ = body // accepted shape; full handler in T3.5
	apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusNotImplemented,
		"not_implemented", "empty-bucket SSE handler lands in T3.5"))
}

// writeSingle encodes a single JSON:API resource document at the chosen
// status code. Centralises the Content-Type and WriteHeader ordering so
// every resource handler emits the same envelope shape.
func writeSingle(w http.ResponseWriter, enc *jsonapi.Encoder, status int, res jsonapi.Resource) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = enc.Single(w, res, nil)
}

// mapInvalidNameCode normalises the processor's legacy `bucket_invalid_name`
// code (used internally by ValidateBucketName paths) to the public
// `invalid_bucket_name` documented in the global error table. Other errors
// pass through unmodified.
func mapInvalidNameCode(err error) error {
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		return err
	}
	if ae.Code == "bucket_invalid_name" {
		return apierror.New(http.StatusUnprocessableEntity, "invalid_bucket_name", ae.Message)
	}
	return ae
}
