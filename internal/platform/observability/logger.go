// Package observability holds structured logging and, later, metrics and
// tracing.
package observability

import (
	"log/slog"
	"os"
	"strings"
)

// Redacted is the placeholder that replaces sensitive values in logs.
const Redacted = "[REDACTED]"

// NewLogger builds the process-wide structured logger.
//
// Text output is easier to read during development; JSON is used in production
// for log ingestion. We never log passwords, tokens or certificates — wrap
// sensitive values in observability.Secret instead.
func NewLogger(level string, jsonOutput bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var h slog.Handler
	if jsonOutput {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Secret wraps a sensitive value so it can never leak into a log line.
//
// Rationale (AppSec): the classic leak is not someone logging a password on
// purpose — it is someone logging the whole config struct with %+v. A type
// that refuses to print its own contents makes that accident impossible.
type Secret string

// String implements fmt.Stringer.
func (Secret) String() string { return Redacted }

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(Redacted) }

// Reveal exposes the real value. Call it only at the point of use (e.g. when
// connecting to the broker), never on a logging path.
func (s Secret) Reveal() string { return string(s) }
