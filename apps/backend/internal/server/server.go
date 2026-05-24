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
	APIRoutes []func(r chi.Router)
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
	root.Use(chimw.Timeout(30 * time.Second))

	root.Get("/healthz", healthz)
	root.Get("/readyz", readyz(s))

	api := chi.NewRouter()
	for _, register := range s.deps.APIRoutes {
		register(api)
	}
	// Trim trailing "/" from BasePath so the "/" root case yields "/api/v1"
	// (chi treats "//api/v1" as the literal pattern, which would not match).
	apiMount := strings.TrimSuffix(s.cfg.BasePath, "/") + "/api/v1"
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
