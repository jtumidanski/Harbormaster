package audit

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// EventResource wraps an Event for JSON:API transport.
// The JSON marshaller emits the documented EventAttrs shape so the
// generic encoder produces the contract-correct `attributes` block.
type EventResource struct {
	Event Event
}

// ResourceType returns the JSON:API type name.
// Per api-contracts.md §GET /api/v1/audit-events the type is the
// underscored form "audit_events".
func (r EventResource) ResourceType() string { return "audit_events" }

// ResourceID returns the event's ULID.
func (r EventResource) ResourceID() string { return r.Event.ID }

// MarshalJSON renders the Event as the documented EventAttrs shape.
// The encoder uses the result as the JSON:API resource's `attributes`.
func (r EventResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(ToAttrs(r.Event))
}

// Routes mounts GET /audit-events under the parent router. Returns a
// JSON:API collection document. Query parameters:
//
//	filter[action]      string
//	filter[target_type] string
//	filter[target_id]   string
//	filter[outcome]     string  ("success" | "failure")
//	filter[from]        RFC 3339 timestamp (inclusive)
//	filter[to]          RFC 3339 timestamp (inclusive)
//	page[number]        1-indexed page (default 1)
//	page[size]          rows per page (default 50, capped at 200)
//
// Invalid timestamp values surface as a JSON:API errors[] envelope with
// source.pointer pointing at the offending query param.
func Routes(p *Processor) func(chi.Router) {
	h := &auditHandler{p: p, enc: jsonapi.NewEncoder()}
	return func(r chi.Router) {
		r.Get("/audit-events", h.list)
	}
}

type auditHandler struct {
	p   *Processor
	enc *jsonapi.Encoder
}

func (h *auditHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := Filter{
		Action:     q.Get("filter[action]"),
		TargetType: q.Get("filter[target_type]"),
		TargetID:   q.Get("filter[target_id]"),
		Outcome:    q.Get("filter[outcome]"),
	}
	if from := q.Get("filter[from]"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
				"bad_request", "filter[from] must be an RFC 3339 timestamp").
				WithPointer("filter[from]"))
			return
		}
		f.From = t
	}
	if to := q.Get("filter[to]"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
				"bad_request", "filter[to] must be an RFC 3339 timestamp").
				WithPointer("filter[to]"))
			return
		}
		f.To = t
	}

	page := Page{Number: parseIntDefault(q.Get("page[number]"), 1), Size: parseIntDefault(q.Get("page[size]"), 50)}

	events, total, err := h.p.List(r.Context(), f, page)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.Internal(err.Error()))
		return
	}

	resources := make([]jsonapi.Resource, len(events))
	for i, e := range events {
		resources[i] = EventResource{Event: e}
	}

	// Normalise page meta after the helper clamps Size — operators see
	// the value the server actually applied, not the raw query string.
	if page.Number < 1 {
		page.Number = 1
	}
	if page.Size < 1 {
		page.Size = 50
	}
	if page.Size > maxPageSize {
		page.Size = maxPageSize
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(page.Size) - 1) / int64(page.Size))
	}

	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, &jsonapi.Meta{
		Page: &jsonapi.Page{
			Number:       page.Number,
			Size:         page.Size,
			TotalRecords: int(total),
			TotalPages:   totalPages,
		},
	}, nil)
}

// parseIntDefault returns fallback when s is empty or not a positive int.
func parseIntDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
