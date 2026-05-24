package buckets

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// Routes returns a chi sub-router function that mounts the bucket
// endpoints under whatever parent path the caller picks.
//
// T3.1 only defines the route surface — every handler currently returns a
// typed 501 envelope. The real handler wireup (decode → processor →
// JSON:API encode / action envelope) lands in T3.3.
func Routes(p *Processor) func(chi.Router) {
	_ = p // kept on the signature so T3.3 can wire handlers without churn
	return func(r chi.Router) {
		r.Get("/buckets", notImplemented)
		r.Post("/buckets", notImplemented)
		r.Get("/buckets/{name}", notImplemented)
		r.Delete("/buckets/{name}", notImplemented)
		r.Post("/buckets/{name}/versioning", notImplemented)
		r.Post("/buckets/{name}/public-access", notImplemented)
		r.Post("/buckets/{name}/quota", notImplemented)
	}
}

// notImplemented renders the action-style 501 envelope used by every
// T3.1 placeholder handler.
func notImplemented(w http.ResponseWriter, _ *http.Request) {
	apierror.Write(w, apierror.StyleAction,
		apierror.New(http.StatusNotImplemented, "not_implemented",
			"bucket handlers are wired in task T3.3"))
}
