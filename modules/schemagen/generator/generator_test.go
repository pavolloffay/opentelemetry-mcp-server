// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSchemaGenerator_GenerateSchema(t *testing.T) {
	testDir := filepath.Join(".", "testcomponent")
	outputDir := t.TempDir()

	analyzer := NewPackageAnalyzer(testDir)
	generator := NewSchemaGenerator(outputDir, analyzer)

	// Pass empty strings to test auto-detection of config type
	err := generator.GenerateSchema("receiver", "test", "", "")
	require.NoError(t, err)

	// Check that schema file was created
	schemaPath := filepath.Join(outputDir, "config_schema.yaml")
	_, err = os.Stat(schemaPath)
	require.NoError(t, err, "schema file was not created")

	// Read the generated schema
	generatedData, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	// Read the expected schema from testdata
	expectedPath := filepath.Join("testdata", "config_schema.yaml")
	expectedData, err := os.ReadFile(expectedPath)
	require.NoError(t, err)

	// Compare YAML content by parsing both and comparing as JSON
	var generatedSchema, expectedSchema any
	require.NoError(t, yaml.Unmarshal(generatedData, &generatedSchema))
	require.NoError(t, yaml.Unmarshal(expectedData, &expectedSchema))

	generatedJSON, err := json.Marshal(generatedSchema)
	require.NoError(t, err)
	expectedJSON, err := json.Marshal(expectedSchema)
	require.NoError(t, err)

	assert.JSONEq(t, string(expectedJSON), string(generatedJSON))
}

func TestDetectConfigFromFactory(t *testing.T) {
	testDir := filepath.Join(".", "testcomponent")
	analyzer := NewPackageAnalyzer(testDir)

	// Load the package
	structInfo, err := analyzer.AnalyzeConfig("", "")
	require.NoError(t, err)

	// Verify the config was detected
	assert.Equal(t, "Config", structInfo.Name)
	assert.Contains(t, structInfo.Package, "testcomponent")

	// Verify fields were extracted
	assert.NotEmpty(t, structInfo.Fields)

	// Check for known fields
	fieldNames := make(map[string]bool)
	for _, f := range structInfo.Fields {
		fieldNames[f.JSONName] = true
	}
	assert.True(t, fieldNames["database"], "expected 'database' field")
	assert.True(t, fieldNames["collection_interval"], "expected 'collection_interval' field")
	assert.True(t, fieldNames["batch_size"], "expected 'batch_size' field")
}

func TestParseTag(t *testing.T) {
	tests := []struct {
		name           string
		tag            string
		expectedName   string
		expectedSquash bool
	}{
		{
			name:           "simple name",
			tag:            `mapstructure:"endpoint"`,
			expectedName:   "endpoint",
			expectedSquash: false,
		},
		{
			name:           "skip field with dash",
			tag:            `mapstructure:"-"`,
			expectedName:   "-",
			expectedSquash: false,
		},
		{
			name:           "squash tag",
			tag:            `mapstructure:",squash"`,
			expectedName:   "",
			expectedSquash: true,
		},
		{
			name:           "name with squash",
			tag:            `mapstructure:"config,squash"`,
			expectedName:   "config",
			expectedSquash: true,
		},
		{
			name:           "empty mapstructure",
			tag:            `json:"foo"`,
			expectedName:   "",
			expectedSquash: false,
		},
		{
			name:           "omitempty option",
			tag:            `mapstructure:"field,omitempty"`,
			expectedName:   "field",
			expectedSquash: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, squash := parseTag(tc.tag)
			assert.Equal(t, tc.expectedName, name, "unexpected name")
			assert.Equal(t, tc.expectedSquash, squash, "unexpected squash")
		})
	}
}

func TestSetSchemaType(t *testing.T) {
	g := &SchemaGenerator{}

	tests := []struct {
		goType       string
		expectedType string
		format       string
		pattern      string
	}{
		{"string", "string", "", ""},
		{"bool", "boolean", "", ""},
		{"int", "integer", "", ""},
		{"int64", "integer", "", ""},
		{"float64", "number", "", ""},
		{"time.Duration", "string", "", `^(0|[-+]?((\d+(\.\d*)?|\.\d+)(ns|us|µs|μs|ms|s|m|h))+)$`},
		{"[]string", "array", "", ""},
		{"map[string]string", "object", "", ""},
		{"go.opentelemetry.io/collector/config/configopaque.String", "string", "", ""},
		{"go.opentelemetry.io/collector/config/configoptional.Optional[string]", "string", "", ""},
		{"go.opentelemetry.io/collector/config/configoptional.Optional[int]", "integer", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.goType, func(t *testing.T) {
			schema := &Schema{}
			g.SetSchemaType(schema, tc.goType)
			if schema.Type != tc.expectedType {
				t.Errorf("expected type %q, got %q", tc.expectedType, schema.Type)
			}
			if tc.format != "" && schema.Format != tc.format {
				t.Errorf("expected format %q, got %q", tc.format, schema.Format)
			}
			if tc.pattern != "" && schema.Pattern != tc.pattern {
				t.Errorf("expected pattern %q, got %q", tc.pattern, schema.Pattern)
			}
		})
	}
}

func TestSchemaValidation(t *testing.T) {
	// Load the YAML schema
	schemaPath := filepath.Join("testdata", "config_schema.yaml")
	schemaData, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "failed to read schema file")

	// Parse the schema YAML and convert to JSON-compatible format
	var schemaDoc any
	err = yaml.Unmarshal(schemaData, &schemaDoc)
	require.NoError(t, err, "failed to parse schema YAML")

	// Convert to JSON and back to ensure JSON-compatible types
	jsonBytes, err := json.Marshal(schemaDoc)
	require.NoError(t, err, "failed to convert schema to JSON")
	err = json.Unmarshal(jsonBytes, &schemaDoc)
	require.NoError(t, err, "failed to parse schema JSON")

	// Compile the schema
	compiler := jsonschema.NewCompiler()
	err = compiler.AddResource("config_schema.json", schemaDoc)
	require.NoError(t, err, "failed to add schema resource")

	schema, err := compiler.Compile("config_schema.json")
	require.NoError(t, err, "failed to compile schema")

	tests := []struct {
		name        string
		configFile  string
		expectValid bool
	}{
		{
			name:        "valid configuration",
			configFile:  "valid_config.yaml",
			expectValid: true,
		},
		{
			name:        "invalid configuration with type mismatches",
			configFile:  "invalid_config.yaml",
			expectValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Load the YAML config
			configPath := filepath.Join("testdata", tc.configFile)
			configData, err := os.ReadFile(configPath)
			require.NoError(t, err, "failed to read config file")

			// Parse YAML to interface{}
			var config any
			err = yaml.Unmarshal(configData, &config)
			require.NoError(t, err, "failed to parse YAML")

			// Convert to JSON-compatible format via round-trip
			jsonBytes, err := json.Marshal(config)
			require.NoError(t, err, "failed to marshal config to JSON")
			err = json.Unmarshal(jsonBytes, &config)
			require.NoError(t, err, "failed to unmarshal JSON")

			// Validate against schema
			validationErr := schema.Validate(config)

			if tc.expectValid {
				require.NoError(t, validationErr, "expected config to be valid")
			} else {
				require.Error(t, validationErr, "expected config to be invalid")
				t.Logf("Validation errors (expected): %v", validationErr)
			}
		})
	}
}

func TestSampleReceiver_GenerateSchema(t *testing.T) {
	testDir := filepath.Join(".", "samplereceiver")
	outputDir := t.TempDir()

	analyzer := NewPackageAnalyzer(testDir)
	generator := NewSchemaGenerator(outputDir, analyzer)

	// Test with explicit config type name (MyConfig)
	err := generator.GenerateSchema("receiver", "sample", "MyConfig", "")
	require.NoError(t, err)

	// Check that schema file was created
	schemaPath := filepath.Join(outputDir, "config_schema.yaml")
	_, err = os.Stat(schemaPath)
	require.NoError(t, err, "schema file was not created")

	// Read the generated schema
	generatedData, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	// Read the expected schema from testdata
	expectedPath := filepath.Join("testdata", "samplereceiver_schema.yaml")
	expectedData, err := os.ReadFile(expectedPath)
	require.NoError(t, err)

	// Compare YAML content by parsing both and comparing as JSON
	var generatedSchema, expectedSchema any
	require.NoError(t, yaml.Unmarshal(generatedData, &generatedSchema))
	require.NoError(t, yaml.Unmarshal(expectedData, &expectedSchema))

	generatedJSON, err := json.Marshal(generatedSchema)
	require.NoError(t, err)
	expectedJSON, err := json.Marshal(expectedSchema)
	require.NoError(t, err)

	assert.JSONEq(t, string(expectedJSON), string(generatedJSON))
}

func TestSampleReceiver_AnalyzeConfig(t *testing.T) {
	testDir := filepath.Join(".", "samplereceiver")
	analyzer := NewPackageAnalyzer(testDir)

	// Test with explicit config type name
	structInfo, err := analyzer.AnalyzeConfig("MyConfig", "")
	require.NoError(t, err)

	assert.Equal(t, "MyConfig", structInfo.Name)
	assert.Contains(t, structInfo.Package, "samplereceiver")

	// Verify fields were extracted
	assert.NotEmpty(t, structInfo.Fields)

	// Build map of fields and check for embedded Network
	fieldNames := make(map[string]bool)
	var hasEmbeddedNetwork bool
	for _, f := range structInfo.Fields {
		fieldNames[f.JSONName] = true
		// The Network field is embedded (squash) and should have host/port as nested fields
		if f.Embedded && f.Name == "Network" {
			hasEmbeddedNetwork = true
			// Verify Network has host and port
			nestedNames := make(map[string]bool)
			for _, nf := range f.Fields {
				nestedNames[nf.JSONName] = true
			}
			assert.True(t, nestedNames["host"], "expected 'host' in embedded Network")
			assert.True(t, nestedNames["port"], "expected 'port' in embedded Network")
		}
	}

	assert.True(t, hasEmbeddedNetwork, "expected embedded Network field")

	// Top-level fields
	assert.True(t, fieldNames["endpoint"], "expected 'endpoint' field")
	assert.True(t, fieldNames["timeout"], "expected 'timeout' field")
	assert.True(t, fieldNames["enabled"], "expected 'enabled' field")
	assert.True(t, fieldNames["batch_size"], "expected 'batch_size' field")
	assert.True(t, fieldNames["retry"], "expected 'retry' field")
	assert.True(t, fieldNames["tags"], "expected 'tags' field")
	assert.True(t, fieldNames["api_key"], "expected 'api_key' field")
	assert.True(t, fieldNames["optional_retry"], "expected 'optional_retry' field")
	assert.True(t, fieldNames["endpoints"], "expected 'endpoints' field")

	t.Logf("Analyzed MyConfig with %d fields", len(structInfo.Fields))
}

func TestSampleReceiver_SchemaValidation(t *testing.T) {
	// Generate schema first
	testDir := filepath.Join(".", "samplereceiver")
	outputDir := t.TempDir()

	analyzer := NewPackageAnalyzer(testDir)
	generator := NewSchemaGenerator(outputDir, analyzer)

	err := generator.GenerateSchema("receiver", "sample", "MyConfig", "")
	require.NoError(t, err)

	// Load the generated schema
	schemaPath := filepath.Join(outputDir, "config_schema.yaml")
	schemaData, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	// Parse and compile the schema
	var schemaDoc any
	err = yaml.Unmarshal(schemaData, &schemaDoc)
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(schemaDoc)
	require.NoError(t, err)
	err = json.Unmarshal(jsonBytes, &schemaDoc)
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	err = compiler.AddResource("samplereceiver_schema.json", schemaDoc)
	require.NoError(t, err)

	schema, err := compiler.Compile("samplereceiver_schema.json")
	require.NoError(t, err)

	tests := []struct {
		name        string
		configFile  string
		expectValid bool
	}{
		{
			name:        "valid sample receiver configuration",
			configFile:  "samplereceiver_config.yaml",
			expectValid: true,
		},
		{
			name:        "invalid sample receiver configuration",
			configFile:  "samplereceiver_invalid_config.yaml",
			expectValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join("testdata", tc.configFile)
			configData, err := os.ReadFile(configPath)
			require.NoError(t, err)

			var config any
			err = yaml.Unmarshal(configData, &config)
			require.NoError(t, err)

			jsonBytes, err := json.Marshal(config)
			require.NoError(t, err)
			err = json.Unmarshal(jsonBytes, &config)
			require.NoError(t, err)

			validationErr := schema.Validate(config)

			if tc.expectValid {
				require.NoError(t, validationErr, "expected config to be valid")
			} else {
				require.Error(t, validationErr, "expected config to be invalid")
				t.Logf("Validation errors (expected): %v", validationErr)
			}
		})
	}
}
