package metrics

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// handler wraps a Processor for HTTP handling.
type handler struct {
	p *Processor
}

// Routes mounts GET /metrics under the provided chi.Router.
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p}
	return func(r chi.Router) {
		r.Get("/metrics", h.view)
	}
}

// view handles GET /metrics?window=<1h|6h|24h|7d>
func (h *handler) view(w http.ResponseWriter, r *http.Request) {
	win, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(
			http.StatusUnprocessableEntity,
			"invalid_metrics_window",
			"window must be one of 1h, 6h, 24h, 7d",
		))
		return
	}

	v, err := h.p.View(r.Context(), win)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Internal(err.Error()))
		return
	}

	resp := toResponse(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
}
