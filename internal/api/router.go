// Package api wires the inbound HTTP transport (primary adapter).
//
// Dependency rule: this package knows the core and the modules; nobody knows
// this package. Swapping HTTP for gRPC tomorrow must not touch domain code.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// RouterDeps are the dependencies needed to build the root router.
type RouterDeps struct {
	Health *HealthHandler
	Logger *slog.Logger
}

// NewRouter builds the root router with global middleware.
func NewRouter(deps RouterDeps) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health endpoint (SH-001).
	r.Method(http.MethodGet, "/health", deps.Health)

	// API surface under /api/v1. Versioning from day one is cheap; versioning
	// after firmware is deployed in the field is not.
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})

	return r
}
