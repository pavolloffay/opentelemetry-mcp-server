package main

import (
	"os"
	"testing"

	"github.com/pavolloffay/opentelemetry-mcp-server/modules/schemagen"
)

// TestGenerateAllSchemas tests the schema generator by generating YAML schemas for all components
func TestGenerateAllSchemas(t *testing.T) {
	schemaOutputDir := os.Getenv("SCHEMA_OUTPUT_DIR")
	if schemaOutputDir == "" {
		schemaOutputDir = "test-schemas"
	}

	factories, err := components()
	if err != nil {
		t.Fatalf("Failed to get components: %v", err)
	}

	generator := schemagen.NewCollectorSchemaGenerator(schemaOutputDir, "vendor")

	if err := generator.GenerateFromFactories(factories); err != nil {
		t.Fatalf("Failed to generate schemas: %v", err)
	}

	if err := generator.CopyReadmeFiles(factories); err != nil {
		t.Fatalf("Failed to copy README files: %v", err)
	}

	t.Logf("Successfully generated YAML schemas in directory: %s", schemaOutputDir)
}
