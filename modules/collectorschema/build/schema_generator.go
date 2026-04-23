package main

import (
	"github.com/pavolloffay/opentelemetry-mcp-server/modules/collectorschema/schemagen"
)

// GenerateAllSchemas generates YAML schemas and copies README files for all components.
func GenerateAllSchemas(outputDir string) error {
	factories, err := components()
	if err != nil {
		return err
	}

	generator := schemagen.NewCollectorSchemaGenerator(outputDir, "vendor")

	if err := generator.GenerateFromFactories(factories); err != nil {
		return err
	}

	return generator.CopyReadmeFiles(factories)
}
