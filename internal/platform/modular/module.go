// Package modular defines the contract every business module must satisfy to
// be plugged into the hub.
//
// This is the heart of the system's modularity (ADR-0001). The core does NOT
// know about irrigation, lighting or blinds. It only knows this interface.
// Adding a new module means writing a Module implementation and registering it
// in the composition root (cmd/hub/main.go) — without touching a single core
// file.
package modular

import (
	"context"
	"log/slog"

	"github.com/abelpinheiro/smarthome-hub/internal/platform/bus"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/storage"

	"github.com/go-chi/chi/v5"
)

// Deps are the dependencies the hub injects into every module.
//
// Note what is NOT here: no module receives the MQTT client directly.
// Publishing commands is the exclusive responsibility of core/command, which
// enforces arbitration, idempotency and auditing (ADR-0006). If a module could
// speak MQTT directly it would bypass the failsafe — exactly the kind of
// shortcut that causes physical accidents.
type Deps struct {
	Bus    bus.Bus
	DB     *storage.Postgres
	Logger *slog.Logger
	HomeID string
}

// Capability is an ability a module requires from the devices it manages.
// The irrigation module requires "valve" and "soil_moisture"; the lighting
// module requires "switch" and optionally "dimmer".
//
// Matching capability to device is what lets a module work with any hardware
// that declares the right ability, instead of knowing specific brands or
// models.
type Capability string

// Module is the contract of a pluggable business module.
type Module interface {
	// Name is the module's stable identifier (e.g. "irrigation").
	Name() string

	// RequiredCapabilities lists what the module expects from devices.
	RequiredCapabilities() []Capability

	// Register is called once at bootstrap. This is where the module
	// subscribes to bus events and mounts its HTTP routes.
	Register(ctx context.Context, deps Deps, r chi.Router) error

	// Shutdown releases module resources during graceful shutdown.
	Shutdown(ctx context.Context) error
}
