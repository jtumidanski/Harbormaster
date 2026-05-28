package setup

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// Routes returns a chi sub-router function that mounts the three setup
// endpoints under whatever parent path the caller picks. All three are
// unauthenticated; the POST handler self-guards with a Status check and
// returns 409 once setup has completed.
func Routes(p *Processor) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/setup/status", p.handleStatus)
		r.Get("/setup/mc-aliases", p.handleMcAliases)
		r.Post("/setup", p.handleSubmit)
	}
}

func (p *Processor) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, StatusResponse{Initialized: p.Status(r.Context())})
}

func (p *Processor) handleMcAliases(w http.ResponseWriter, _ *http.Request) {
	aliases, version, _ := ReadMcAliases(p.McPath)
	if aliases == nil {
		aliases = []McAlias{}
	}
	resp := McAliasesResponse{Aliases: aliases}
	if version != "" && version != "10" {
		resp.UnsupportedVersion = version
	}
	writeJSON(w, http.StatusOK, resp)
}

func (p *Processor) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if p.Status(r.Context()) {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusConflict,
			"already_initialized", "Setup has already been completed."))
		return
	}
	var body Request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	sourceIP := r.RemoteAddr
	if err := p.Submit(r.Context(), body, sourceIP); err != nil {
		writeError(w, err)
		return
	}
	// Contract: 201 Created with {"initialized": true}.
	writeJSON(w, http.StatusCreated, StatusResponse{Initialized: true})
}

// writeError renders an error from Submit through the action-style envelope.
// ErrAlreadyInitialized -> 409 setup_already_completed.
// ErrMcAliasNotFound    -> 422 mc_alias_not_found.
// *apierror.Error       -> pass through (probe failures from ConnProc.Validate).
// anything else         -> 500 internal_error.
func writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrAlreadyInitialized) {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusConflict,
			"already_initialized", "Setup has already been completed."))
		return
	}
	if errors.Is(err, ErrMcAliasNotFound) {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusUnprocessableEntity,
			"mc_alias_not_found", "Referenced mc alias was not found in the mc config."))
		return
	}
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
