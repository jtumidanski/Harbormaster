package auth

import (
	"encoding/json"
	"net/http"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// notImplemented renders the action-shape "not_implemented" envelope. The
// real HTTP wiring (cookies, CSRF header, audit tagger) lands in T2.3/T2.6;
// these handlers exist now so route mounting in T2.6 can reference stable
// names.
func notImplemented(component string) *apierror.Error {
	return apierror.New(http.StatusNotImplemented, "not_implemented",
		component+" is not yet wired up; see T2.3.")
}

// LoginHandler returns a handler that decodes a LoginRequest and invokes
// Processor.Login. It does not yet emit Set-Cookie headers — T2.3 owns that.
func LoginHandler(p *Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, apierror.StyleAction,
				apierror.New(http.StatusBadRequest, "invalid_request_body",
					"Request body must be valid JSON."))
			return
		}
		sessionID, csrf, err := p.Login(r.Context(), req.Username, req.Password,
			r.RemoteAddr, r.UserAgent())
		if err != nil {
			apierror.Write(w, apierror.StyleAction, err)
			return
		}
		writeJSON(w, http.StatusOK, LoginResponse{
			SessionID: sessionID,
			CSRFToken: csrf,
		})
	}
}

// LogoutHandler returns a handler that delegates to Processor.Logout. T2.3
// will read the session ID from the HttpOnly cookie instead of the body.
func LogoutHandler(p *Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Placeholder for T2.1: nothing to do until cookies arrive. We accept
		// a JSON body with an optional session_id to keep tests deterministic
		// in this transitional state.
		var body struct {
			SessionID string `json:"session_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := p.Logout(r.Context(), body.SessionID); err != nil {
			apierror.Write(w, apierror.StyleAction, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// MeHandler returns a handler that resolves the caller's session. T2.3 will
// read the cookie; until then the placeholder reports not_implemented when
// no session header is provided.
func MeHandler(p *Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			apierror.Write(w, apierror.StyleAction, notImplemented("cookie reader"))
			return
		}
		sess, user, err := p.Me(r.Context(), sessionID)
		if err != nil {
			apierror.Write(w, apierror.StyleAction, err)
			return
		}
		writeJSON(w, http.StatusOK, MeResponse{
			Username:     user.Username(),
			SessionID:    sess.ID(),
			ExpiresAt:    sess.ExpiresAt(),
			LastActiveAt: sess.LastActiveAt(),
		})
	}
}

// ChangePasswordHandler returns a handler that updates the caller's password.
// Like MeHandler, the session is read from a header for now; T2.3 swaps this
// for the cookie + CSRF middleware.
func ChangePasswordHandler(p *Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			apierror.Write(w, apierror.StyleAction, notImplemented("cookie reader"))
			return
		}
		var req ChangePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, apierror.StyleAction,
				apierror.New(http.StatusBadRequest, "invalid_request_body",
					"Request body must be valid JSON."))
			return
		}
		if err := p.ChangePassword(r.Context(), sessionID, req.CurrentPassword, req.NewPassword); err != nil {
			apierror.Write(w, apierror.StyleAction, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeJSON writes an action-style JSON success body.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}
