package main

import (
	"github.com/pavolloffay/opentelemetry-mcp-server/modules/collectorschema"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

// SchemaGeneratorWrapper wraps the collectorschema.SchemaGenerator for use in the build package
type SchemaGeneratorWrapper struct {
	*collectorschema.SchemaGenerator
}

// NewSchemaGenerator creates a new schema generator that outputs to the specified directory
func NewSchemaGenerator(outputDir string) *SchemaGeneratorWrapper {
	return &SchemaGeneratorWrapper{
		SchemaGenerator: collectorschema.NewSchemaGenerator(outputDir),
	}
}

// GenerateAllSchemas generates YAML schemas for all components using the local components() function
func (sg *SchemaGeneratorWrapper) GenerateAllSchemas() error {
	factories, err := components()
	if err != nil {
		return err
	}
	return sg.SchemaGenerator.GenerateAllSchemas(factories)
}

// generateSchemaForComponent is a helper for tests that need to generate individual component schemas
func (sg *SchemaGeneratorWrapper) generateSchemaForComponent(componentCategory string, componentType component.Type, factory component.Factory) error {
	return sg.SchemaGenerator.GenerateSchemaForComponent(componentCategory, componentType, factory)
}

// generateYAMLSchema is a helper for tests that need to generate YAML schema from config
func (sg *SchemaGeneratorWrapper) generateYAMLSchema(config component.Config) (map[string]interface{}, error) {
	return sg.SchemaGenerator.GenerateYAMLSchema(config)
}

// copyAllReadmeFiles copies README files for all components
func (sg *SchemaGeneratorWrapper) copyAllReadmeFiles(factories *otelcol.Factories) error {
	return sg.SchemaGenerator.CopyAllReadmeFiles(factories)
}
