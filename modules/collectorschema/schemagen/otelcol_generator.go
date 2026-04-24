// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package schemagen

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

// CollectorSchemaGenerator generates schemas for OpenTelemetry Collector components.
type CollectorSchemaGenerator struct {
	outputDir string
	vendorDir string
}

// NewCollectorSchemaGenerator creates a new generator for collector schemas.
func NewCollectorSchemaGenerator(outputDir, vendorDir string) *CollectorSchemaGenerator {
	return &CollectorSchemaGenerator{
		outputDir: outputDir,
		vendorDir: vendorDir,
	}
}

// GenerateFromFactories generates schemas for all components in the factories.
func (g *CollectorSchemaGenerator) GenerateFromFactories(factories otelcol.Factories) error {
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	componentTypes := []struct {
		kind      string
		factories map[component.Type]component.Factory
	}{
		{"receiver", toFactoryMap(factories.Receivers)},
		{"processor", toFactoryMap(factories.Processors)},
		{"exporter", toFactoryMap(factories.Exporters)},
		{"extension", toFactoryMap(factories.Extensions)},
		{"connector", toFactoryMap(factories.Connectors)},
	}

	for _, ct := range componentTypes {
		for compType, factory := range ct.factories {
			if err := g.GenerateForFactory(ct.kind, compType.String(), factory); err != nil {
				fmt.Printf("Warning: failed to generate schema for %s %s: %v\n", ct.kind, compType, err)
			}
		}
	}

	return nil
}

// GenerateForFactory generates a schema for a single component factory.
func (g *CollectorSchemaGenerator) GenerateForFactory(componentKind, componentName string, factory component.Factory) error {
	pkgPath := GetFactoryPackagePath(factory)
	if pkgPath == "" {
		return fmt.Errorf("could not determine package path")
	}

	vendorPath := filepath.Join(g.vendorDir, pkgPath)
	if _, err := os.Stat(vendorPath); os.IsNotExist(err) {
		return fmt.Errorf("vendor directory not found: %s", vendorPath)
	}

	analyzer := NewPackageAnalyzer(vendorPath)
	generator := NewSchemaGenerator(g.outputDir, analyzer)

	outputFile := filepath.Join(g.outputDir, fmt.Sprintf("%s_%s.yaml", componentKind, componentName))
	if err := generator.GenerateSchemaToFile(componentKind, componentName, "", "", outputFile); err != nil {
		return err
	}

	fmt.Printf("Generated schema for %s %s\n", componentKind, componentName)
	return nil
}

// CopyReadmeFiles copies README files for all components using module paths from factories.
func (g *CollectorSchemaGenerator) CopyReadmeFiles(factories otelcol.Factories) error {
	if _, err := os.Stat(g.vendorDir); os.IsNotExist(err) {
		return fmt.Errorf("vendor directory %s not found", g.vendorDir)
	}

	componentTypes := []struct {
		kind    string
		modules map[component.Type]string
	}{
		{"receiver", factories.ReceiverModules},
		{"processor", factories.ProcessorModules},
		{"exporter", factories.ExporterModules},
		{"extension", factories.ExtensionModules},
		{"connector", factories.ConnectorModules},
	}

	for _, ct := range componentTypes {
		for compType, modulePath := range ct.modules {
			if err := g.CopyReadmeForModule(ct.kind, compType.String(), modulePath); err != nil {
				fmt.Printf("Warning: failed to copy README for %s %s: %v\n", ct.kind, compType, err)
			}
		}
	}

	return nil
}

// CopyReadmeForModule copies the README file for a component from its module path.
func (g *CollectorSchemaGenerator) CopyReadmeForModule(componentKind, componentName, modulePath string) error {
	pkgPath := parseModulePath(modulePath)
	readmePath := filepath.Join(g.vendorDir, pkgPath, "README.md")

	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		return fmt.Errorf("README.md not found at %s", readmePath)
	}

	destPath := filepath.Join(g.outputDir, fmt.Sprintf("%s_%s.md", componentKind, componentName))

	srcData, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := os.WriteFile(destPath, srcData, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	fmt.Printf("Copied README for %s %s\n", componentKind, componentName)
	return nil
}

// parseModulePath extracts the package path from a module string like
// "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpreceiver v0.147.0"
func parseModulePath(modulePath string) string {
	parts := strings.Fields(modulePath)
	if len(parts) > 0 {
		return parts[0]
	}
	return modulePath
}

// GetFactoryPackagePath returns the package path where the factory's config is defined.
func GetFactoryPackagePath(factory component.Factory) string {
	cfg := factory.CreateDefaultConfig()
	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if pkgPath := t.PkgPath(); pkgPath != "" {
		return pkgPath
	}

	// Fallback: parse the type string
	factoryType := fmt.Sprintf("%T", factory)
	if idx := strings.LastIndex(factoryType, "."); idx != -1 {
		pkgPart := factoryType[:idx]
		pkgPart = strings.TrimPrefix(pkgPart, "*")
		return pkgPart
	}
	return ""
}

// toFactoryMap converts typed factory maps to generic component.Factory map.
func toFactoryMap[T component.Factory](m map[component.Type]T) map[component.Type]component.Factory {
	result := make(map[component.Type]component.Factory, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
