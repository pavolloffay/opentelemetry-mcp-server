package collectorschema

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
	factory := newTestFactory()

	// Get the default config
	defaultConfig := factory.CreateDefaultConfig()
	if defaultConfig == nil {
		t.Fatalf("Factory returned nil config")
	}

	// Create schema generator
	generator := NewSchemaGenerator("testdata")

	// Generate schema for our test component
	generatedSchema, err := generator.GenerateYAMLSchema(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to generate YAML schema: %v", err)
	}

	// Write generated schema to file
	generatedBytes, err := yaml.Marshal(generatedSchema)
	if err != nil {
		t.Fatalf("Failed to marshal generated schema: %v", err)
	}

	generatedFile := filepath.Join("testdata", "actual_generated_schema.yaml")
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

// testComponentType is the type identifier for our test component
var testComponentType = component.MustNewType("testcomponent")

// testDatabaseConfig represents database connection configuration
type testDatabaseConfig struct {
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Username string        `mapstructure:"username"`
	Password string        `mapstructure:"password"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// testReceiverConfig defines the configuration for our test receiver
type testReceiverConfig struct {
	Database           testDatabaseConfig                              `mapstructure:"database"`
	HTTPServer         configoptional.Optional[confighttp.ServerConfig] `mapstructure:"http_server"`
	CollectionInterval time.Duration                                   `mapstructure:"collection_interval"`
	BatchSize          int                                             `mapstructure:"batch_size"`
	EnableTracing      bool                                            `mapstructure:"enable_tracing"`
	LogLevel           string                                          `mapstructure:"log_level,omitempty"`
	IncludeTables      []string                                        `mapstructure:"include_tables,omitempty"`
	TableAliases       map[string]string                               `mapstructure:"table_aliases,omitempty"`
	OldEndpoint        string                                          `mapstructure:"old_endpoint,omitempty"`
	component.Config   `mapstructure:",squash"`
}

// testReceiver is our test receiver implementation
type testReceiver struct {
	config   *testReceiverConfig
	settings receiver.Settings
}

// createTestDefaultConfig creates the default configuration
func createTestDefaultConfig() component.Config {
	return &testReceiverConfig{
		Database: testDatabaseConfig{
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
		OldEndpoint: "",
	}
}

func createTestTracesReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (receiver.Traces, error) {
	config := cfg.(*testReceiverConfig)
	return &testReceiver{
		config:   config,
		settings: settings,
	}, nil
}

func createTestMetricsReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (receiver.Metrics, error) {
	config := cfg.(*testReceiverConfig)
	return &testReceiver{
		config:   config,
		settings: settings,
	}, nil
}

func createTestLogsReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	config := cfg.(*testReceiverConfig)
	return &testReceiver{
		config:   config,
		settings: settings,
	}, nil
}

func (r *testReceiver) Start(ctx context.Context, host component.Host) error {
	return nil
}

func (r *testReceiver) Shutdown(ctx context.Context) error {
	return nil
}

// newTestFactory creates a new test receiver factory
func newTestFactory() receiver.Factory {
	return receiver.NewFactory(
		testComponentType,
		createTestDefaultConfig,
		receiver.WithTraces(createTestTracesReceiver, component.StabilityLevelDevelopment),
		receiver.WithMetrics(createTestMetricsReceiver, component.StabilityLevelDevelopment),
		receiver.WithLogs(createTestLogsReceiver, component.StabilityLevelDevelopment),
	)
}