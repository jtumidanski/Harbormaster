package server

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Health holds the live/ready signals.
type Health struct {
	ready atomic.Bool
}

// SetReady flips the readiness signal.
func (h *Health) SetReady(b bool) { h.ready.Store(b) }

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyz(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// When no Ready probe is wired (M1 / tests), behave as before.
		if s.deps.Ready == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		ok, reason := s.deps.Ready(r.Context())
		if ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		body := map[string]any{
			"error": map[string]any{
				"code":    "not_ready",
				"message": reason,
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}
}
