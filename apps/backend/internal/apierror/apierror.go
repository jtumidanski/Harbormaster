// Package apierror defines the typed error envelope used by the HTTP layer.
// Resource routes render JSON:API errors[]; action routes render a plain
// { error: { code, message, details? } } envelope. The Style constant on
// the route record picks which one.
package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// Style selects the envelope shape used to render an Error.
type Style int

const (
	StyleAction Style = iota // {"error":{"code","message","details?"}}
	StyleJSONAPI             // {"errors":[{"status","code","title","detail","source"}]}
)

// Error is the typed error sentinel carried across handlers.
type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	HTTPStatus int            `json:"-"`
	Pointer    string         `json:"-"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// New constructs an Error.
func New(status int, code, msg string) *Error {
	return &Error{HTTPStatus: status, Code: code, Message: msg}
}

// WithDetails returns a copy with details attached.
func (e *Error) WithDetails(d map[string]any) *Error {
	cp := *e
	cp.Details = d
	return &cp
}

// WithPointer returns a copy with JSON:API source.pointer set.
func (e *Error) WithPointer(p string) *Error {
	cp := *e
	cp.Pointer = p
	return &cp
}

// Write renders err to w with the chosen Style. Falls back to 500 if err is
// not an *Error.
func Write(w http.ResponseWriter, style Style, err error) {
	var ae *Error
	if !errors.As(err, &ae) {
		ae = New(http.StatusInternalServerError, "internal_error", "An internal error occurred.")
	}
	w.Header().Set("Content-Type", contentType(style))
	w.WriteHeader(ae.HTTPStatus)
	switch style {
	case StyleJSONAPI:
		_ = jsonapi.WriteError(w, jsonapi.Error{
			Status: ae.HTTPStatus, Code: ae.Code, Title: ae.Code,
			Detail: ae.Message, Pointer: ae.Pointer, Meta: ae.Details,
		})
	default:
		writeAction(w, ae)
	}
}

func writeAction(w io.Writer, ae *Error) {
	type body struct {
		Error *Error `json:"error"`
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body{Error: ae})
}

func contentType(s Style) string {
	if s == StyleJSONAPI {
		return "application/vnd.api+json"
	}
	return "application/json"
}

// Common sentinel constructors. Add additional codes as features land.
func Unauthenticated() *Error { return New(http.StatusUnauthorized, "unauthenticated", "Authentication required.") }
func CSRFInvalid() *Error     { return New(http.StatusForbidden, "csrf_token_invalid", "Missing or invalid CSRF token.") }
func NotFound(what string) *Error {
	return New(http.StatusNotFound, "not_found", what+" not found")
}
func Internal(msg string) *Error { return New(http.StatusInternalServerError, "internal_error", msg) }
