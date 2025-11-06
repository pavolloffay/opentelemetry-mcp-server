package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"gopkg.in/yaml.v3"
)

// TestSchemaGenerationWithCustomComponent tests the schema generator with our custom test component
func TestSchemaGenerationWithCustomComponent(t *testing.T) {
	// Create our custom test component factory
	factory := NewFactory()

	// Get the default config
	defaultConfig := factory.CreateDefaultConfig()
	if defaultConfig == nil {
		t.Fatalf("Factory returned nil config")
	}

	// Create schema generator
	generator := NewSchemaGenerator("test_output")

	// Generate schema for our test component
	generatedSchema, err := generator.generateYAMLSchema(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to generate YAML schema: %v", err)
	}

	// Write generated schema to file
	generatedBytes, err := yaml.Marshal(generatedSchema)
	if err != nil {
		t.Fatalf("Failed to marshal generated schema: %v", err)
	}

	generatedFile := filepath.Join("test_output", "actual_generated_schema.yaml")
	if err := os.MkdirAll("test_output", 0755); err != nil {
		t.Fatalf("Failed to create test_output directory: %v", err)
	}

	if err := os.WriteFile(generatedFile, generatedBytes, 0644); err != nil {
		t.Fatalf("Failed to write generated schema: %v", err)
	}

	// Read expected schema file
	expectedSchemaPath := filepath.Join("testdata", "expected_testcomponent_schema.yaml")
	expectedBytes, err := os.ReadFile(expectedSchemaPath)
	if err != nil {
		t.Fatalf("Failed to read expected schema file: %v", err)
	}

	// Read generated schema file
	actualBytes, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatalf("Failed to read generated schema file: %v", err)
	}

	// Compare as strings
	expectedStr := string(expectedBytes)
	actualStr := string(actualBytes)

	if expectedStr != actualStr {
		t.Errorf("Generated schema does not match expected schema.\nExpected file: %s\nActual file: %s", expectedSchemaPath, generatedFile)
	}
}

// TestComponentType is the type identifier for our test component
var TestComponentType = component.MustNewType("testcomponent")

// DatabaseConfig represents database connection configuration
type DatabaseConfig struct {
	// Host is the database server hostname or IP address
	Host string `mapstructure:"host"`
	// Port is the database server port number
	Port int `mapstructure:"port"`
	// Username for database authentication
	Username string `mapstructure:"username"`
	// Password for database authentication (will be encrypted)
	Password string `mapstructure:"password"`
	// Timeout for database connection attempts
	Timeout time.Duration `mapstructure:"timeout"`
}

// TestReceiverConfig defines the configuration for our test receiver
type TestReceiverConfig struct {
	// Database contains the database connection configuration
	Database DatabaseConfig `mapstructure:"database"`

	// HTTPServer configuration for optional HTTP endpoint (uses configoptional wrapper)
	HTTPServer configoptional.Optional[confighttp.ServerConfig] `mapstructure:"http_server"`

	// CollectionInterval defines how often to collect metrics from the database
	CollectionInterval time.Duration `mapstructure:"collection_interval"`
	// BatchSize controls how many records to process in each batch
	BatchSize int `mapstructure:"batch_size"`
	// EnableTracing enables distributed tracing for this receiver
	EnableTracing bool `mapstructure:"enable_tracing"`
	// LogLevel sets the logging verbosity (debug, info, warn, error)
	LogLevel string `mapstructure:"log_level,omitempty"`

	// IncludeTables lists specific database tables to monitor
	IncludeTables []string `mapstructure:"include_tables,omitempty"`

	// TableAliases maps short names to full table names for convenience
	TableAliases map[string]string `mapstructure:"table_aliases,omitempty"`

	// OldEndpoint is deprecated and will be removed in v2.0. Use HTTPServer instead.
	OldEndpoint string `mapstructure:"old_endpoint,omitempty"`

	// Embedded standard component configuration
	component.Config `mapstructure:",squash"`
}

// TestReceiver is our test receiver implementation
type TestReceiver struct {
	config   *TestReceiverConfig
	settings receiver.Settings
}

// CreateDefaultConfig creates the default configuration
func CreateDefaultConfig() component.Config {
	return &TestReceiverConfig{
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Username: "testuser",
			Password: "",
			Timeout:  30 * time.Second,
		},
		HTTPServer:         configoptional.Optional[confighttp.ServerConfig]{},
		CollectionInterval: 30 * time.Second,
		BatchSize:          100,
		EnableTracing:      true,
		LogLevel:           "info",
		IncludeTables:      []string{"users", "orders", "products"},
		TableAliases: map[string]string{
			"u": "users",
			"o": "orders",
		},
		OldEndpoint: "", // Default empty value for deprecated field
	}
}

// createTracesReceiver creates a trace receiver
func createTracesReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (receiver.Traces, error) {
	config := cfg.(*TestReceiverConfig)
	return &TestReceiver{
		config:   config,
		settings: settings,
	}, nil
}

// createMetricsReceiver creates a metrics receiver
func createMetricsReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (receiver.Metrics, error) {
	config := cfg.(*TestReceiverConfig)
	return &TestReceiver{
		config:   config,
		settings: settings,
	}, nil
}

// createLogsReceiver creates a logs receiver
func createLogsReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	config := cfg.(*TestReceiverConfig)
	return &TestReceiver{
		config:   config,
		settings: settings,
	}, nil
}

// Start starts the receiver
func (r *TestReceiver) Start(ctx context.Context, host component.Host) error {
	return nil
}

// Shutdown stops the receiver
func (r *TestReceiver) Shutdown(ctx context.Context) error {
	return nil
}

// NewFactory creates a new test receiver factory
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		TestComponentType,
		CreateDefaultConfig,
		receiver.WithTraces(createTracesReceiver, component.StabilityLevelDevelopment),
		receiver.WithMetrics(createMetricsReceiver, component.StabilityLevelDevelopment),
		receiver.WithLogs(createLogsReceiver, component.StabilityLevelDevelopment),
	)
}
