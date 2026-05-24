package connection

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
)

// Routes returns a chi sub-router function that mounts /connection,
// /connection (PUT), and /connection/test under whatever parent path the
// caller picks. Auth/CSRF middleware is mounted upstream by the composition
// root; this package assumes a valid session by the time a handler runs.
func Routes(p *Processor) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/connection", p.handleGet)
		r.Put("/connection", p.handleUpdate)
		r.Post("/connection/test", p.handleTest)
	}
}

func (p *Processor) handleGet(w http.ResponseWriter, r *http.Request) {
	view, err := p.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGetResponse(view))
}

func (p *Processor) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var body UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	if err := p.Update(r.Context(), body.toSubmitInput(), actor, ip); err != nil {
		writeError(w, err)
		return
	}
	view, err := p.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, UpdateResponse(toGetResponse(view)))
}

func (p *Processor) handleTest(w http.ResponseWriter, r *http.Request) {
	var body TestRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	result, _ := p.Test(r.Context(), body.toSubmitInput(), actor, ip)
	// Per api-contracts.md, /connection/test always returns 200 with the
	// per-step status object — the "failed: {…}" shape on a step is how
	// the wizard renders partial probe outcomes. The structured apierror
	// envelope is only emitted on PUT.
	writeJSON(w, http.StatusOK, result)
}

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

// writeError renders any error through the action-style apierror envelope.
// An *apierror.Error passes through with its HTTPStatus + Code intact;
// anything else collapses to 500 internal_error.
func writeError(w http.ResponseWriter, err error) {
	var ae *apierror.Error
	if errors.As(err, &ae) {
		apierror.Write(w, apierror.StyleAction, ae)
		return
	}
	apierror.Write(w, apierror.StyleAction, apierror.Internal(err.Error()))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
