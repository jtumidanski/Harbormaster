package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// Routes returns a chi sub-router function that mounts the dashboard
// endpoint under whatever parent path the caller picks. The endpoint is
// action-style (per api-contracts.md §dashboard) — it returns a plain
// JSON object, not a JSON:API resource.
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p}
	return func(r chi.Router) {
		r.Get("/dashboard", h.get)
	}
}

type handler struct {
	p *Processor
}

// get handles GET /dashboard?failures_window=24h|7d|30d. The window
// query parameter is optional (defaults to 7d); an unrecognised value
// returns 422 invalid_failures_window per api-contracts.md.
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	window, err := Parse(r.URL.Query().Get("failures_window"))
	if err != nil {
		if errors.Is(err, ErrInvalidWindow) {
			apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusUnprocessableEntity,
				"invalid_failures_window",
				"failures_window must be one of: 24h, 7d, 30d"))
			return
		}
		apierror.Write(w, apierror.StyleAction, apierror.Internal(err.Error()))
		return
	}
	view, err := h.p.Build(r.Context(), window)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Internal(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(view)
}
