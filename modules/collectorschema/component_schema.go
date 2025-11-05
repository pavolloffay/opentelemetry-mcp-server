package collectorschema

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

//go:embed schemas
var embeddedSchemas embed.FS

// ComponentType represents the type of OpenTelemetry component
type ComponentType string

const (
	ComponentTypeReceiver  ComponentType = "receiver"
	ComponentTypeProcessor ComponentType = "processor"
	ComponentTypeExporter  ComponentType = "exporter"
	ComponentTypeExtension ComponentType = "extension"
	ComponentTypeConnector ComponentType = "connector"
)

// ComponentSchema represents a JSON schema for an OpenTelemetry component
type ComponentSchema struct {
	Name    string                 `json:"name"`
	Type    ComponentType          `json:"type"`
	Version string                 `json:"version,omitempty"`
	Schema  map[string]interface{} `json:"schema"`
}

// DeprecatedField represents a deprecated field with its information
type DeprecatedField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// SchemaManager manages component schemas
type SchemaManager struct {
	cache map[string]*ComponentSchema
}

// NewSchemaManager creates a new schema manager
func NewSchemaManager() *SchemaManager {
	return &SchemaManager{
		cache: make(map[string]*ComponentSchema),
	}
}

// GetComponentSchema returns the JSON schema for a specific component
func (sm *SchemaManager) GetComponentSchema(componentType ComponentType, componentName string, version string) (*ComponentSchema, error) {
	// Create cache key
	cacheKey := fmt.Sprintf("%s_%s_%s", componentType, componentName, version)

	// Check cache first
	if schema, exists := sm.cache[cacheKey]; exists {
		return schema, nil
	}

	// Load schema from file
	schema, err := sm.loadSchemaFromFile(componentType, componentName, version)
	if err != nil {
		return nil, err
	}

	// Cache the result
	sm.cache[cacheKey] = schema

	return schema, nil
}

// GetComponentSchemaJSON returns the JSON schema as a JSON byte array
func (sm *SchemaManager) GetComponentSchemaJSON(componentType ComponentType, componentName string, version string) ([]byte, error) {
	schema, err := sm.GetComponentSchema(componentType, componentName, version)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(schema.Schema, "", "  ")
}

// ListAvailableComponents returns a list of all available components by type
func (sm *SchemaManager) ListAvailableComponents(version string) (map[ComponentType][]string, error) {
	return sm.listEmbeddedComponents(version)
}

// ValidateComponentJSON validates a component configuration JSON against its schema
func (sm *SchemaManager) ValidateComponentJSON(componentType ComponentType, componentName string, version string, jsonData []byte) (*gojsonschema.Result, error) {
	// Get the component schema
	componentSchema, err := sm.GetComponentSchema(componentType, componentName, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for %s %s v%s: %w", componentType, componentName, version, err)
	}

	// Convert schema to JSON bytes for gojsonschema
	schemaBytes, err := json.Marshal(componentSchema.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema for %s %s: %w", componentType, componentName, err)
	}

	// Create schema loader
	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)

	// Create document loader from the provided JSON data
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	// Validate the document against the schema
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return nil, fmt.Errorf("validation failed for %s %s: %w", componentType, componentName, err)
	}

	return result, nil
}

// GetComponentReadme returns the README content for a specific component
func (sm *SchemaManager) GetComponentReadme(componentType ComponentType, componentName string, version string) (string, error) {
	// Construct filename (format: type_name.md)
	filename := fmt.Sprintf("%s_%s.md", componentType, componentName)

	// Load from embedded filesystem
	schemaPath := fmt.Sprintf("schemas/%s", version)
	embeddedFilepath := filepath.Join(schemaPath, filename)
	data, err := fs.ReadFile(embeddedSchemas, embeddedFilepath)
	if err != nil {
		return "", fmt.Errorf("README not found for component %s %s v%s", componentType, componentName, version)
	}

	return string(data), nil
}

// GetChangelog returns the changelog content for a specific collector version
func (sm *SchemaManager) GetChangelog(version string) (string, error) {
	// Load changelog.md from embedded filesystem
	schemaPath := fmt.Sprintf("schemas/%s", version)
	embeddedFilepath := filepath.Join(schemaPath, "changelog.md")
	data, err := fs.ReadFile(embeddedSchemas, embeddedFilepath)
	if err != nil {
		return "", fmt.Errorf("changelog not found for version %s", version)
	}

	return string(data), nil
}

// listEmbeddedComponents lists components from embedded filesystem
func (sm *SchemaManager) listEmbeddedComponents(version string) (map[ComponentType][]string, error) {
	components := make(map[ComponentType][]string)

	// Read embedded directory
	schemaPath := fmt.Sprintf("schemas/%s", version)
	entries, err := fs.ReadDir(embeddedSchemas, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded schema directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Remove .json extension
		name := strings.TrimSuffix(entry.Name(), ".json")

		// Parse component type and name from filename (format: type_name.json)
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			continue // Skip files that don't match the expected format
		}

		componentType := ComponentType(parts[0])
		componentName := parts[1]

		// Validate component type
		if !isValidComponentType(componentType) {
			continue
		}

		components[componentType] = append(components[componentType], componentName)
	}

	return components, nil
}

// loadSchemaFromFile loads a schema from embedded files
func (sm *SchemaManager) loadSchemaFromFile(componentType ComponentType, componentName string, version string) (*ComponentSchema, error) {
	// Construct filename (format: type_name.json)
	filename := fmt.Sprintf("%s_%s.json", componentType, componentName)

	// Load from embedded filesystem
	schemaPath := fmt.Sprintf("schemas/%s", version)
	embeddedFilepath := filepath.Join(schemaPath, filename)
	data, err := fs.ReadFile(embeddedSchemas, embeddedFilepath)
	if err != nil {
		return nil, fmt.Errorf("schema not found for component %s %s", componentType, componentName)
	}

	// Parse JSON schema
	var schemaData map[string]interface{}
	if err := json.Unmarshal(data, &schemaData); err != nil {
		return nil, fmt.Errorf("failed to parse schema JSON for %s %s: %w", componentType, componentName, err)
	}

	// Use the provided version
	componentVersion := version

	return &ComponentSchema{
		Name:    componentName,
		Type:    componentType,
		Version: componentVersion,
		Schema:  schemaData,
	}, nil
}

// isValidComponentType checks if the component type is valid
func isValidComponentType(componentType ComponentType) bool {
	switch componentType {
	case ComponentTypeReceiver, ComponentTypeProcessor, ComponentTypeExporter, ComponentTypeExtension, ComponentTypeConnector:
		return true
	default:
		return false
	}
}

// GetLatestVersion returns the latest version available in the schemas directory
func (sm *SchemaManager) GetLatestVersion() (string, error) {
	entries, err := fs.ReadDir(embeddedSchemas, "schemas")
	if err != nil {
		return "", fmt.Errorf("failed to read schemas directory: %w", err)
	}

	var latestVersion string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory name looks like a version (contains dots)
			version := entry.Name()
			if strings.Contains(version, ".") {
				if latestVersion == "" || version > latestVersion {
					latestVersion = version
				}
			}
		}
	}

	if latestVersion == "" {
		return "", fmt.Errorf("no versions found in schemas directory")
	}

	return latestVersion, nil
}

// GetAllVersions returns all versions available in the schemas directory
func (sm *SchemaManager) GetAllVersions() ([]string, error) {
	entries, err := fs.ReadDir(embeddedSchemas, "schemas")
	if err != nil {
		return nil, fmt.Errorf("failed to read schemas directory: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory name looks like a version (contains dots)
			version := entry.Name()
			if strings.Contains(version, ".") {
				versions = append(versions, version)
			}
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found in schemas directory")
	}

	return versions, nil
}

// GetComponentNames returns all component names for a given version and component type
func (sm *SchemaManager) GetComponentNames(componentType ComponentType, version string) ([]string, error) {
	// Validate component type
	if !isValidComponentType(componentType) {
		return nil, fmt.Errorf("invalid component type: %s", componentType)
	}

	// Read embedded directory for the specific version
	schemaPath := fmt.Sprintf("schemas/%s", version)
	entries, err := fs.ReadDir(embeddedSchemas, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema directory for version %s: %w", version, err)
	}

	var componentNames []string
	prefix := string(componentType) + "_"

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Check if the file matches the component type pattern (e.g., "receiver_otlp.json")
		if strings.HasPrefix(entry.Name(), prefix) {
			// Extract component name by removing prefix and .json suffix
			name := strings.TrimSuffix(entry.Name(), ".json")
			componentName := strings.TrimPrefix(name, prefix)
			if componentName != "" {
				componentNames = append(componentNames, componentName)
			}
		}
	}

	if len(componentNames) == 0 {
		return nil, fmt.Errorf("no %s components found for version %s", componentType, version)
	}

	return componentNames, nil
}

// GetDeprecatedFields returns a list of deprecated fields with their information for a specific component
func (sm *SchemaManager) GetDeprecatedFields(componentType ComponentType, componentName string, version string) ([]DeprecatedField, error) {
	// Get the component schema
	schema, err := sm.GetComponentSchema(componentType, componentName, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for %s %s v%s: %w", componentType, componentName, version, err)
	}

	var deprecatedFields []DeprecatedField

	// Recursively traverse the schema to find deprecated fields
	sm.findDeprecatedFields(schema.Schema, "", &deprecatedFields)

	return deprecatedFields, nil
}

// findDeprecatedFields recursively searches for deprecated fields in a JSON schema
func (sm *SchemaManager) findDeprecatedFields(schema map[string]interface{}, currentPath string, deprecatedFields *[]DeprecatedField) {
	// Check if this schema has properties
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		// Iterate through all properties
		for fieldName, fieldSchema := range properties {
			// Build the current field path
			var fieldPath string
			if currentPath == "" {
				fieldPath = fieldName
			} else {
				fieldPath = currentPath + "." + fieldName
			}

			// Check if the field schema is a map
			if fieldSchemaMap, ok := fieldSchema.(map[string]interface{}); ok {
				// Check if this field is marked as deprecated
				if deprecated, exists := fieldSchemaMap["deprecated"]; exists {
					if deprecatedBool, ok := deprecated.(bool); ok && deprecatedBool {
						// Extract field information
						description := ""
						if desc, exists := fieldSchemaMap["description"]; exists {
							if descStr, ok := desc.(string); ok {
								description = descStr
							}
						}

						fieldType := ""
						if fType, exists := fieldSchemaMap["type"]; exists {
							if typeStr, ok := fType.(string); ok {
								fieldType = typeStr
							}
						}

						deprecatedField := DeprecatedField{
							Name:        fieldPath,
							Description: description,
							Type:        fieldType,
						}

						*deprecatedFields = append(*deprecatedFields, deprecatedField)
					}
				}

				// Recursively check nested objects
				sm.findDeprecatedFields(fieldSchemaMap, fieldPath, deprecatedFields)
			}
		}
	}
}
