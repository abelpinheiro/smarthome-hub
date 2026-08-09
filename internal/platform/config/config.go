// Package config loads the hub configuration exclusively from environment
// variables.
//
// Security decision (ADR-0007): no instance configuration lives in the
// repository. The public code ships capability; configuration is private and
// local to each home. That is why there is no versioned config file — only
// .env.example, filled with fictional values.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the complete configuration of the hub process.
type Config struct {
	// HomeID identifies the installation. It appears in the MQTT topic
	// hierarchy and becomes the multi-tenant isolation key once the cloud
	// module exists.
	HomeID   string `env:"HUB_HOME_ID,required"`
	LogLevel string `env:"HUB_LOG_LEVEL" envDefault:"info"`
	LogJSON  bool   `env:"HUB_LOG_JSON" envDefault:"false"`

	HTTP HTTP
	DB   DB
	MQTT MQTT
}

// HTTP configures the inbound server (REST today, WebSocket from SH-003 on).
type HTTP struct {
	Addr            string        `env:"HUB_HTTP_ADDR" envDefault:":8080"`
	ReadTimeout     time.Duration `env:"HUB_HTTP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout    time.Duration `env:"HUB_HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	ShutdownTimeout time.Duration `env:"HUB_HTTP_SHUTDOWN_TIMEOUT" envDefault:"15s"`
}

// DB configures access to PostgreSQL/TimescaleDB.
type DB struct {
	DSN         string        `env:"HUB_DB_DSN,required"`
	MaxConns    int32         `env:"HUB_DB_MAX_CONNS" envDefault:"10"`
	ConnTimeout time.Duration `env:"HUB_DB_CONN_TIMEOUT" envDefault:"5s"`
}

// MQTT configures the broker connection.
type MQTT struct {
	URL      string `env:"HUB_MQTT_URL" envDefault:"mqtt://localhost:1883"`
	ClientID string `env:"HUB_MQTT_CLIENT_ID" envDefault:"smarthome-hub"`
	Username string `env:"HUB_MQTT_USERNAME,required"`
	// Password must never be logged. See observability.Secret.
	Password  string        `env:"HUB_MQTT_PASSWORD,required"`
	KeepAlive time.Duration `env:"HUB_MQTT_KEEPALIVE" envDefault:"20s"`
}

// Load reads and validates the configuration. It fails fast on purpose: a
// misconfigured hub should die at boot rather than discover the problem while
// trying to open a valve.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}
