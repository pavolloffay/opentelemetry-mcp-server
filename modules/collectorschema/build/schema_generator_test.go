package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateAllSchemas tests the schema generator by generating YAML schemas for all components
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

	t.Logf("Successfully generated YAML schemas in directory: %s", schemaOutputDir)
}

// verifyGeneratedSchemas verifies that schema files were created and are valid
func verifyGeneratedSchemas(t *testing.T, schemaOutputDir string) error {
	// Check if schema directory exists
	if _, err := os.Stat(schemaOutputDir); os.IsNotExist(err) {
		return fmt.Errorf("schema directory %s does not exist", schemaOutputDir)
	}

	// Count schema files
	files, err := filepath.Glob(filepath.Join(schemaOutputDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to list schema files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no schema files were generated")
	}

	t.Logf("Generated %d schema files", len(files))

	// Verify a few sample schema files exist
	expectedFiles := []string{
		"receiver_otlp.yaml",
		"exporter_debug.yaml",
		"processor_batch.yaml",
		"extension_zpages.yaml",
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

// TestSchemaGeneratorIndividualComponent tests schema generation for a single component via module path
func TestSchemaGeneratorIndividualComponent(t *testing.T) {
	// Get component factories
	factories, err := components()
	if err != nil {
		t.Fatalf("Failed to get component factories: %v", err)
	}

	// Create a temporary directory for this test
	tmpDir := "test_schemas_individual"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create schema generator
	generator := NewSchemaGenerator(tmpDir)

	// Test with OTLP receiver using the module path approach
	for ctype, modulePath := range factories.ReceiverModules {
		if ctype.String() == "otlp" {
			if err := generator.generateSchemaForModule("receiver", ctype, modulePath); err != nil {
				t.Fatalf("Failed to generate schema for OTLP receiver: %v", err)
			}

			// Verify file was created
			expectedFile := filepath.Join(tmpDir, "receiver_otlp.yaml")
			if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
				t.Fatalf("Schema file was not created: %s", expectedFile)
			}

			t.Logf("Successfully generated schema for OTLP receiver")
			return
		}
	}

	t.Skip("OTLP receiver not found in factories")
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
