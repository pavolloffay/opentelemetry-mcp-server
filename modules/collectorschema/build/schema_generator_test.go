package main

import (
	"os"
	"testing"
)

// TestGenerateAllSchemas tests the schema generator by generating YAML schemas for all components
func TestGenerateAllSchemas(t *testing.T) {
	// Get output directory from environment variable, fallback to default
	schemaOutputDir := os.Getenv("SCHEMA_OUTPUT_DIR")
	if schemaOutputDir == "" {
		schemaOutputDir = "test-schemas"
	}

	// Generate all schemas
	if err := GenerateAllSchemas(schemaOutputDir); err != nil {
		t.Fatalf("Failed to generate schemas: %v", err)
	}

	t.Logf("Successfully generated YAML schemas in directory: %s", schemaOutputDir)
}
