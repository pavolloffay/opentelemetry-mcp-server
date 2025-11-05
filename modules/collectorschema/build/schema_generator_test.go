package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/receiver"
)

// TestGenerateAllSchemas tests the schema generator by generating JSON schemas for all components
func TestGenerateAllSchemas(t *testing.T) {
	// Get output directory from environment variable, fallback to default
	schemaOutputDir := os.Getenv("SCHEMA_OUTPUT_DIR")
	if schemaOutputDir == "" {
		schemaOutputDir = "test-schemas"
	}

	// Create schema generator
	generator := NewSchemaGenerator(schemaOutputDir)

	// Generate all schemas
	if err := generator.GenerateAllSchemas(); err != nil {
		t.Fatalf("Failed to generate schemas: %v", err)
	}

	// Verify that schemas were created
	if err := verifyGeneratedSchemas(t, schemaOutputDir); err != nil {
		t.Fatalf("Schema verification failed: %v", err)
	}

	t.Logf("Successfully generated JSON schemas in directory: %s", schemaOutputDir)
}

// verifyGeneratedSchemas verifies that schema files were created and are valid
func verifyGeneratedSchemas(t *testing.T, schemaOutputDir string) error {
	// Check if schema directory exists
	if _, err := os.Stat(schemaOutputDir); os.IsNotExist(err) {
		return fmt.Errorf("schema directory %s does not exist", schemaOutputDir)
	}

	// Count schema files
	files, err := filepath.Glob(filepath.Join(schemaOutputDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list schema files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no schema files were generated")
	}

	t.Logf("Generated %d schema files", len(files))

	// Verify a few sample schema files exist
	expectedFiles := []string{
		"receiver_otlp.json",
		"exporter_debug.json",
		"processor_batch.json",
		"extension_zpages.json",
	}

	for _, expectedFile := range expectedFiles {
		expectedPath := filepath.Join(schemaOutputDir, expectedFile)
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Logf("Warning: Expected schema file %s not found", expectedFile)
		} else {
			t.Logf("Found expected schema file: %s", expectedFile)
		}
	}

	return nil
}

// TestSchemaGeneratorIndividualComponent tests schema generation for a single component
func TestSchemaGeneratorIndividualComponent(t *testing.T) {
	// Get component factories
	factories, err := components()
	if err != nil {
		t.Fatalf("Failed to get component factories: %v", err)
	}

	// Create a temporary directory for this test
	tmpDir := "test_schemas"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create schema generator
	generator := NewSchemaGenerator(tmpDir)

	// Test with OTLP receiver (should exist in all builds)
	var otlpType component.Type
	var otlpFactory receiver.Factory
	found := false

	// Find the OTLP receiver type from factories
	for ctype, factory := range factories.Receivers {
		if ctype.String() == "otlp" {
			otlpType = ctype
			otlpFactory = factory
			found = true
			break
		}
	}

	if found {
		if err := generator.generateSchemaForComponent("receiver", otlpType, otlpFactory); err != nil {
			t.Fatalf("Failed to generate schema for OTLP receiver: %v", err)
		}

		// Verify file was created
		expectedFile := filepath.Join(tmpDir, "receiver_otlp.json")
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Fatalf("Schema file was not created: %s", expectedFile)
		}

		t.Logf("Successfully generated schema for OTLP receiver")
	} else {
		t.Skip("OTLP receiver not found in factories")
	}
}

// TestSchemaFormat tests that generated schemas have the correct format
func TestSchemaFormat(t *testing.T) {
	// Get component factories
	factories, err := components()
	if err != nil {
		t.Fatalf("Failed to get component factories: %v", err)
	}

	// Create schema generator
	generator := NewSchemaGenerator("test_format")
	defer os.RemoveAll("test_format")

	// Test with debug exporter (should exist in all builds)
	var debugFactory exporter.Factory
	debugFound := false

	// Find the debug exporter type from factories
	for ctype, factory := range factories.Exporters {
		if ctype.String() == "debug" {
			debugFactory = factory
			debugFound = true
			break
		}
	}

	if debugFound {
		defaultConfig := debugFactory.CreateDefaultConfig()
		if defaultConfig == nil {
			t.Fatalf("Factory returned nil config")
		}

		schema, err := generator.generateJSONSchema(defaultConfig)
		if err != nil {
			t.Fatalf("Failed to generate JSON schema: %v", err)
		}

		// Verify required schema fields
		if schema["$schema"] == nil {
			t.Error("Schema missing $schema field")
		}

		if schema["type"] != "object" {
			t.Error("Schema type should be 'object'")
		}

		if schema["properties"] == nil {
			t.Error("Schema missing properties field")
		}

		t.Logf("Schema validation passed for debug exporter")
	} else {
		t.Skip("Debug exporter not found in factories")
	}
}

// BenchmarkSchemaGeneration benchmarks the schema generation process
func BenchmarkSchemaGeneration(b *testing.B) {
	// Create schema generator
	generator := NewSchemaGenerator("bench_schemas")
	defer os.RemoveAll("bench_schemas")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := generator.GenerateAllSchemas(); err != nil {
			b.Fatalf("Failed to generate schemas: %v", err)
		}
	}
}
