// Command hub is the Smart Home Hub process.
//
// This file is the COMPOSITION ROOT: the only place in the system that knows
// every concrete part and wires them together. Everything else depends on
// interfaces only. If you find yourself importing a module anywhere outside
// this file, the boundary is wrong.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abelpinheiro/smarthome-hub/internal/api"
	"github.com/abelpinheiro/smarthome-hub/internal/modules/irrigation"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/bus"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/config"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/modular"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/mqtt"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/observability"
	"github.com/abelpinheiro/smarthome-hub/internal/platform/storage"

	"github.com/go-chi/chi/v5"
)

// version is injected at build time via -ldflags.
var version = "dev"

func main() {
	if err := run(); err != nil {
		// Boot errors go to stderr without relying on the logger, which may
		// not have been constructed yet.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// A single context governs the process lifecycle. Ctrl+C or SIGTERM
	// cancels it and everything else unwinds in cascade.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := observability.NewLogger(cfg.LogLevel, cfg.LogJSON)
	log.Info("starting smarthome-hub",
		slog.String("version", version),
		slog.String("home_id", cfg.HomeID),
	)

	// --- Platform layer ------------------------------------------------------

	db, err := storage.NewPostgres(ctx, cfg.DB.DSN, cfg.DB.MaxConns, cfg.DB.ConnTimeout)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("postgres connected")

	eventBus := bus.NewInProcess(log)

	// SH-001: this subscription only proves the end-to-end path. Parsing and
	// persistence arrive in SH-003.
	onMessage := func(_ context.Context, topic string, payload []byte) {
		log.Info("MQTT message received",
			slog.String("topic", topic),
			slog.Int("bytes", len(payload)),
		)
	}

	broker, err := mqtt.Connect(ctx, mqtt.Options{
		URL:       cfg.MQTT.URL,
		ClientID:  cfg.MQTT.ClientID,
		Username:  cfg.MQTT.Username,
		Password:  cfg.MQTT.Password,
		KeepAlive: cfg.MQTT.KeepAlive,
		Logger:    log,
	}, onMessage)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := broker.Close(closeCtx); err != nil {
			log.Warn("error closing MQTT connection", slog.String("error", err.Error()))
		}
	}()

	connCtx, cancelConn := context.WithTimeout(ctx, 15*time.Second)
	defer cancelConn()
	if err := broker.AwaitConnection(connCtx); err != nil {
		return err
	}

	// Wide telemetry subscription for this home (SH-001).
	telemetryTopic := fmt.Sprintf("home/%s/dev/+/telemetry", cfg.HomeID)
	if err := broker.Subscribe(ctx, telemetryTopic, 1); err != nil {
		return err
	}

	// --- Transport layer -----------------------------------------------------

	health := api.NewHealthHandler(db, broker, version, log)
	router := api.NewRouter(api.RouterDeps{Health: health, Logger: log})

	// --- Business modules ----------------------------------------------------
	//
	// Adding a new module means adding one line to this slice. No core file
	// needs to change — that is the real test of modularity.
	modules := []modular.Module{
		irrigation.New(),
	}

	deps := modular.Deps{
		Bus:    eventBus,
		DB:     db,
		Logger: log,
		HomeID: cfg.HomeID,
	}

	if err := registerModules(ctx, modules, deps, router); err != nil {
		return err
	}
	defer shutdownModules(modules, log)

	// --- HTTP server ---------------------------------------------------------

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", slog.String("addr", cfg.HTTP.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("HTTP server: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received, stopping gracefully")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown: %w", err)
	}

	log.Info("stopped cleanly")
	return nil
}

func registerModules(ctx context.Context, modules []modular.Module, deps modular.Deps, router chi.Router) error {
	// The subrouter is created ONCE: chi panics if the same pattern is mounted
	// twice. Each module mounts its routes under /api/v1/modules/{module}.
	var regErr error
	router.Route("/api/v1/modules", func(r chi.Router) {
		for _, m := range modules {
			if err := m.Register(ctx, deps, r); err != nil {
				regErr = fmt.Errorf("register module %q: %w", m.Name(), err)
				return
			}
		}
	})
	return regErr
}

func shutdownModules(modules []modular.Module, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, m := range modules {
		if err := m.Shutdown(ctx); err != nil {
			log.Warn("error shutting down module",
				slog.String("module", m.Name()),
				slog.String("error", err.Error()))
		}
	}
}
