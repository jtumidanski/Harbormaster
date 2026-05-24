package server

import (
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

func readyz(_ *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// In M1 we always return 200 once the server is running. M2 wires in
		// migrations + cached MinIO admin probe and gates here.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
