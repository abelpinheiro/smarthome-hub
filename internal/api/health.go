package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Status is the result of a health check.
type Status string

const (
	StatusOK   Status = "ok"
	StatusFail Status = "fail"
)

// Check is the result for an individual dependency.
type Check struct {
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// HealthResponse is the body returned by GET /health.
type HealthResponse struct {
	Status  Status           `json:"status"`
	Version string           `json:"version"`
	Checks  map[string]Check `json:"checks"`
}

// DBChecker is the database dependency as seen by the HTTP layer.
// The interface is declared at the consumer (idiomatic Go): the API does not
// import the whole storage package just to check health.
type DBChecker interface {
	Ping(ctx context.Context) error
	TimescaleVersion(ctx context.Context) (string, error)
}

// BrokerChecker is the broker dependency as seen by the HTTP layer.
type BrokerChecker interface {
	IsConnected() bool
}

// HealthHandler serves the health endpoint.
//
// AppSec note: this endpoint exposes internal detail (extension version,
// dependency state). Today the hub only listens on the LAN, which makes that
// acceptable. Once the cloud module exists, the detailed /health must sit
// behind authentication and only a bodyless 200/503 stays public.
type HealthHandler struct {
	db      DBChecker
	broker  BrokerChecker
	version string
	log     *slog.Logger
}

// NewHealthHandler builds the health handler.
func NewHealthHandler(db DBChecker, broker BrokerChecker, version string, log *slog.Logger) *HealthHandler {
	return &HealthHandler{db: db, broker: broker, version: version, log: log}
}

// ServeHTTP implements http.Handler.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp := HealthResponse{
		Status:  StatusOK,
		Version: h.version,
		Checks:  make(map[string]Check, 2),
	}

	resp.Checks["postgres"] = h.checkPostgres(ctx)
	resp.Checks["mqtt"] = h.checkBroker()

	for _, c := range resp.Checks {
		if c.Status == StatusFail {
			resp.Status = StatusFail
			break
		}
	}

	code := http.StatusOK
	if resp.Status == StatusFail {
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.ErrorContext(ctx, "failed to encode /health response",
			slog.String("error", err.Error()))
	}
}

func (h *HealthHandler) checkPostgres(ctx context.Context) Check {
	if err := h.db.Ping(ctx); err != nil {
		return Check{Status: StatusFail, Error: err.Error()}
	}
	// Confirming the extension avoids the classic silent failure: booting
	// against a plain Postgres and only finding out at the first hypertable
	// migration.
	v, err := h.db.TimescaleVersion(ctx)
	if err != nil {
		return Check{Status: StatusFail, Error: err.Error()}
	}
	return Check{Status: StatusOK, Detail: "timescaledb " + v}
}

func (h *HealthHandler) checkBroker() Check {
	if !h.broker.IsConnected() {
		return Check{Status: StatusFail, Error: "no connection to the MQTT broker"}
	}
	return Check{Status: StatusOK}
}
