// Package server boots the chi mux, mounts the API and SPA, and runs the
// main + optional metrics HTTP servers with graceful shutdown.
package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/observability"
)

// Deps wires per-domain handler registrars into Server.
type Deps struct {
	Logger zerolog.Logger
	// Subrouter handlers added in M2+ — each domain provides a func(chi.Router).
	// APIRoutes run under the 30s request-timeout middleware; use
	// StreamingAPIRoutes for long-lived endpoints (SSE) that must not be
	// cancelled by the global timeout.
	APIRoutes          []func(r chi.Router)
	StreamingAPIRoutes []func(r chi.Router)
	// Ready is consulted by /readyz. When nil, readyz always returns 200
	// (M1 behaviour). When non-nil, ok=false yields 503 with reason in the
	// apierror envelope.
	Ready func(ctx context.Context) (ok bool, reason string)
}

// New builds a Server but does not start it.
func New(cfg config.Config, deps Deps) *Server {
	return &Server{cfg: cfg, deps: deps}
}

// Server holds the HTTP server lifecycle.
type Server struct {
	cfg   config.Config
	deps  Deps
	httpS *http.Server
}

// Run blocks until ctx is canceled. Graceful shutdown drains for 10s.
func (s *Server) Run(ctx context.Context) error {
	root := chi.NewRouter()
	root.Use(chimw.RequestID)
	root.Use(chimw.Recoverer)
	root.Use(observability.Logger(s.deps.Logger))
	root.Use(chimw.RealIP)
	// NOTE: the per-request Timeout middleware is intentionally NOT applied
	// at the root level. It is scoped to the non-streaming API router below
	// so SSE handlers (POST /api/v1/buckets/{name}/empty) can outlive the
	// 30s window without us giving up the 504 protection on the regular
	// JSON/JSON:API surface.

	root.Get("/healthz", healthz)
	root.Get("/readyz", readyz(s))

	// Trim trailing "/" from BasePath so the "/" root case yields "/api/v1"
	// (chi treats "//api/v1" as the literal pattern, which would not match).
	apiMount := strings.TrimSuffix(s.cfg.BasePath, "/") + "/api/v1"

	api := chi.NewRouter()
	// Streaming routes register outside the Timeout group so SSE handlers
	// can outlive the 30s window without giving up the 504 protection on
	// the regular JSON/JSON:API surface.
	for _, register := range s.deps.StreamingAPIRoutes {
		register(api)
	}
	api.Group(func(g chi.Router) {
		g.Use(chimw.Timeout(30 * time.Second))
		for _, register := range s.deps.APIRoutes {
			register(g)
		}
	})
	root.Mount(apiMount, api)

	root.Handle("/*", spaHandler(s.cfg.BasePath))

	s.httpS = &http.Server{Addr: s.cfg.ListenAddr, Handler: root, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		err := s.httpS.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpS.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}
