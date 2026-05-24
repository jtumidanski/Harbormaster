package auth

import "time"

// LoginRequest is the action-shape payload accepted by POST /api/auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the action-shape success payload returned by login.
// In T2.3 the session ID will be transported via Set-Cookie rather than the
// JSON body; the CSRF token continues to be returned here so the SPA can
// echo it via the X-CSRF-Token header.
type LoginResponse struct {
	SessionID string `json:"session_id,omitempty"`
	CSRFToken string `json:"csrf_token"`
}

// MeResponse is returned by GET /api/auth/me.
type MeResponse struct {
	Username     string    `json:"username"`
	SessionID    string    `json:"session_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// ChangePasswordRequest is the body for POST /api/auth/password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// LogoutRequest currently carries no fields; declared for symmetry and to
// reserve room for an "all_sessions" flag in a later milestone.
type LogoutRequest struct{}
