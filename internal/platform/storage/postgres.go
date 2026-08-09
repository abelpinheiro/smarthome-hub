// Package storage encapsulates access to PostgreSQL/TimescaleDB.
//
// Decision (ADR-0003): a single database for both domain data and time series.
// TimescaleDB is a Postgres extension, so we get hypertables, compression and
// continuous aggregates without operating a second system.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the connection pool shared by the process.
type Postgres struct {
	Pool *pgxpool.Pool
}

// NewPostgres opens and validates the pool. It fails if the database does not
// answer within the timeout — we would rather not start at all than start
// half-working.
func NewPostgres(ctx context.Context, dsn string, maxConns int32, connTimeout time.Duration) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Never include the DSN in the error: it contains the password.
		return nil, fmt.Errorf("invalid postgres DSN: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.ConnConfig.ConnectTimeout = connTimeout

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("initial postgres ping: %w", err)
	}

	return &Postgres{Pool: pool}, nil
}

// Ping checks connection health. Used by the /health endpoint.
func (p *Postgres) Ping(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}

// TimescaleVersion returns the installed TimescaleDB extension version.
//
// Exposing it in /health guards against the classic silent failure: booting
// against a plain Postgres and only finding out at the first hypertable
// migration.
func (p *Postgres) TimescaleVersion(ctx context.Context) (string, error) {
	var version string
	err := p.Pool.QueryRow(ctx,
		`SELECT extversion FROM pg_extension WHERE extname = 'timescaledb'`,
	).Scan(&version)
	if err != nil {
		return "", fmt.Errorf("timescaledb extension not found: %w", err)
	}
	return version, nil
}

// Close shuts the pool down.
func (p *Postgres) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
}
