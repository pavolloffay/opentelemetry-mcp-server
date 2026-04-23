package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pavolloffay/opentelemetry-mcp-server/modules/collectorschema/schemagen"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

// SchemaGeneratorWrapper wraps the schemagen package for use in the build package
type SchemaGeneratorWrapper struct {
	outputDir string
	vendorDir string
}

// NewSchemaGenerator creates a new schema generator that outputs to the specified directory
func NewSchemaGenerator(outputDir string) *SchemaGeneratorWrapper {
	return &SchemaGeneratorWrapper{
		outputDir: outputDir,
		vendorDir: "vendor",
	}
}

// GenerateAllSchemas generates YAML schemas for all components using the local components() function
func (sg *SchemaGeneratorWrapper) GenerateAllSchemas() error {
	factories, err := components()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(sg.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := sg.generateSchemasForModules("extension", factories.ExtensionModules); err != nil {
		return fmt.Errorf("failed to generate extension schemas: %w", err)
	}

	if err := sg.generateSchemasForModules("receiver", factories.ReceiverModules); err != nil {
		return fmt.Errorf("failed to generate receiver schemas: %w", err)
	}

	if err := sg.generateSchemasForModules("processor", factories.ProcessorModules); err != nil {
		return fmt.Errorf("failed to generate processor schemas: %w", err)
	}

	if err := sg.generateSchemasForModules("exporter", factories.ExporterModules); err != nil {
		return fmt.Errorf("failed to generate exporter schemas: %w", err)
	}

	if err := sg.generateSchemasForModules("connector", factories.ConnectorModules); err != nil {
		return fmt.Errorf("failed to generate connector schemas: %w", err)
	}

	if err := sg.copyAllReadmeFiles(&factories); err != nil {
		return fmt.Errorf("failed to copy README files: %w", err)
	}

	return nil
}

// generateSchemasForModules generates schemas for all components of a given type
func (sg *SchemaGeneratorWrapper) generateSchemasForModules(componentCategory string, modules map[component.Type]string) error {
	fmt.Printf("Generating schemas for %d %ss...\n", len(modules), componentCategory)

	for componentType, modulePath := range modules {
		if err := sg.generateSchemaForModule(componentCategory, componentType, modulePath); err != nil {
			fmt.Printf("Warning: failed to generate schema for %s %s: %v\n", componentCategory, componentType, err)
			continue
		}
	}
	return nil
}

// generateSchemaForModule generates a schema for a single component using static analysis
func (sg *SchemaGeneratorWrapper) generateSchemaForModule(componentCategory string, componentType component.Type, modulePath string) error {
	pkgPath := parseModulePath(modulePath)
	pkgDir := filepath.Join(sg.vendorDir, pkgPath)

	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return fmt.Errorf("package directory not found: %s", pkgDir)
	}

	analyzer := schemagen.NewPackageAnalyzer(pkgDir)
	generator := schemagen.NewSchemaGenerator(sg.outputDir, analyzer)

	outputPath := filepath.Join(sg.outputDir, fmt.Sprintf("%s_%s.yaml", componentCategory, componentType))

	if err := generator.GenerateSchemaToFile(componentCategory, componentType.String(), "", "", outputPath); err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	fmt.Printf("Generated schema for %s %s -> %s_%s.yaml\n", componentCategory, componentType, componentCategory, componentType)
	return nil
}

// parseModulePath extracts the package path from a module string like
// "github.com/open-telemetry/opentelemetry-collector-contrib/extension/bearertokenauthextension v0.147.0"
func parseModulePath(modulePath string) string {
	parts := strings.Fields(modulePath)
	if len(parts) > 0 {
		return parts[0]
	}
	return modulePath
}

// copyAllReadmeFiles copies README files for all components
func (sg *SchemaGeneratorWrapper) copyAllReadmeFiles(factories *otelcol.Factories) error {
	if _, err := os.Stat(sg.vendorDir); os.IsNotExist(err) {
		fmt.Printf("Warning: vendor directory %s not found, skipping README copy\n", sg.vendorDir)
		return nil
	}

	fmt.Println("Copying README files for all components...")

	componentTypes := []struct {
		name    string
		modules map[component.Type]string
	}{
		{"extension", factories.ExtensionModules},
		{"receiver", factories.ReceiverModules},
		{"processor", factories.ProcessorModules},
		{"exporter", factories.ExporterModules},
		{"connector", factories.ConnectorModules},
	}

	for _, compType := range componentTypes {
		for componentType, modulePath := range compType.modules {
			if err := sg.copyReadmeForComponent(compType.name, componentType, modulePath); err != nil {
				fmt.Printf("Warning: failed to copy README for %s %s: %v\n", compType.name, componentType, err)
				continue
			}
		}
	}

	fmt.Println("Successfully copied all README files!")
	return nil
}

// copyReadmeForComponent copies README file for a specific component
func (sg *SchemaGeneratorWrapper) copyReadmeForComponent(componentCategory string, componentType component.Type, modulePath string) error {
	pkgPath := parseModulePath(modulePath)
	readmePath := filepath.Join(sg.vendorDir, pkgPath, "README.md")

	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		return fmt.Errorf("README.md not found at %s", readmePath)
	}

	destFilename := fmt.Sprintf("%s_%s.md", componentCategory, componentType)
	destPath := filepath.Join(sg.outputDir, destFilename)

	srcData, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := os.WriteFile(destPath, srcData, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	fmt.Printf("Copied README for %s %s -> %s\n", componentCategory, componentType, destFilename)
	return nil
}
