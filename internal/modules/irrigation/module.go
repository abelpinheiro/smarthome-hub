// Package irrigation is the plant irrigation business module.
//
// Boundary (ADR-0001): every business rule lives under ./internal/, which the
// Go compiler forbids other modules from importing. The public surface of this
// package is just Module and the contract in contract.go.
//
// Operating model (ADR-0006): irrigation is AUTOMATIC by default. Manual
// control is a deliberate exception, not the primary mode — and even in manual
// the maximum-pump-runtime failsafe stays sovereign.
package irrigation

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/abelpinheiro/smarthome-hub/internal/platform/modular"

	"github.com/go-chi/chi/v5"
)

// Module implements modular.Module for irrigation.
//
// SH-001 ships only the skeleton and its registration. Domain logic
// (scheduling, watering decisions, failsafe) arrives in SH-006.
type Module struct {
	deps modular.Deps
	log  *slog.Logger
}

// New creates the irrigation module.
func New() *Module { return &Module{} }

// Name implements modular.Module.
func (m *Module) Name() string { return ModuleName }

// RequiredCapabilities implements modular.Module.
func (m *Module) RequiredCapabilities() []modular.Capability {
	return []modular.Capability{CapValve, CapSoilMoisture}
}

// Register implements modular.Module.
func (m *Module) Register(_ context.Context, deps modular.Deps, r chi.Router) error {
	m.deps = deps
	m.log = deps.Logger.With(slog.String("module", ModuleName))

	// SH-006: subscribe to telemetry events and mount the module routes.
	r.Route("/"+ModuleName, func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	})

	m.log.Info("module registered",
		slog.Any("capabilities", m.RequiredCapabilities()))
	return nil
}

// Shutdown implements modular.Module.
func (m *Module) Shutdown(_ context.Context) error { return nil }

// Compile-time contract check.
var _ modular.Module = (*Module)(nil)
