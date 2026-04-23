// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package testcomponent

import (
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
)

var _ component.Config = (*Config)(nil)

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	// Host is the database host address.
	Host string `mapstructure:"host"`

	// Port is the database port number.
	Port int `mapstructure:"port"`

	// Username is the database username.
	Username string `mapstructure:"username"`

	// Password is the database password.
	Password string `mapstructure:"password"`

	// Timeout is the connection timeout.
	Timeout time.Duration `mapstructure:"timeout"`
}

// ServerConfig holds HTTP server configuration for testing.
type ServerConfig struct {
	// Endpoint is the server address.
	Endpoint string `mapstructure:"endpoint"`

	// ReadTimeout is the read timeout.
	ReadTimeout time.Duration `mapstructure:"read_timeout"`
}

// Config defines the configuration for the test component.
type Config struct {
	// Database contains database connection settings.
	Database DatabaseConfig `mapstructure:"database"`

	// HTTPServer is an optional HTTP server configuration.
	HTTPServer configoptional.Optional[ServerConfig] `mapstructure:"http_server"`

	// CollectionInterval is how often to collect data.
	CollectionInterval time.Duration `mapstructure:"collection_interval"`

	// BatchSize is the number of items to process in each batch.
	BatchSize int `mapstructure:"batch_size"`

	// EnableTracing enables trace collection.
	EnableTracing bool `mapstructure:"enable_tracing"`

	// LogLevel is the logging level.
	LogLevel string `mapstructure:"log_level,omitempty"`

	// IncludeTables is the list of tables to include.
	IncludeTables []string `mapstructure:"include_tables,omitempty"`

	// TableAliases maps table aliases to full table names.
	TableAliases map[string]string `mapstructure:"table_aliases,omitempty"`

	// OldEndpoint is deprecated, use Database.Host instead.
	OldEndpoint string `mapstructure:"old_endpoint,omitempty"`
}
