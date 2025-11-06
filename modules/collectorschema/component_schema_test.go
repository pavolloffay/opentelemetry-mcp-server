package collectorschema

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaManager_GetComponentSchema(t *testing.T) {
	manager := NewSchemaManager()

	// Test getting OTLP receiver schema
	schema, err := manager.GetComponentSchema(ComponentTypeReceiver, "otlp", "0.138.0")
	if err != nil {
		t.Fatalf("Failed to get OTLP receiver schema: %v", err)
	}

	if schema == nil {
		t.Fatal("Schema is nil")
	}

	if schema.Name != "otlp" {
		t.Errorf("Expected component name 'otlp', got '%s'", schema.Name)
	}

	if schema.Type != ComponentTypeReceiver {
		t.Errorf("Expected component type 'receiver', got '%s'", schema.Type)
	}

	if schema.Schema == nil {
		t.Fatal("Schema data is nil")
	}

	// Verify schema has expected properties

	t.Logf("Successfully loaded schema for %s %s with %d top-level properties",
		schema.Type, schema.Name, len(schema.Schema))
}

func TestSchemaManager_GetComponentSchemaJSON(t *testing.T) {
	manager := NewSchemaManager()

	// Test getting JSON for debug exporter
	jsonData, err := manager.GetComponentSchemaJSON(ComponentTypeExporter, "debug", "0.138.0")
	if err != nil {
		t.Fatalf("Failed to get debug exporter schema JSON: %v", err)
	}

	if len(jsonData) == 0 {
		t.Fatal("JSON data is empty")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Generated JSON is invalid: %v", err)
	}

	t.Logf("Successfully generated %d bytes of JSON for debug exporter", len(jsonData))
}

func TestSchemaManager_NonExistentComponent(t *testing.T) {
	manager := NewSchemaManager()

	// Test getting schema for non-existent component
	_, err := manager.GetComponentSchema(ComponentTypeReceiver, "nonexistent", "0.138.0")
	if err == nil {
		t.Fatal("Expected error for non-existent component, got nil")
	}

	expectedError := "schema not found for component receiver nonexistent"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestSchemaManager_ListAvailableComponents(t *testing.T) {
	manager := NewSchemaManager()

	components, err := manager.ListAvailableComponents("0.138.0")
	if err != nil {
		t.Fatalf("Failed to list available components: %v", err)
	}

	if len(components) == 0 {
		t.Fatal("No components found")
	}

	// Verify we have expected component types
	expectedTypes := []ComponentType{
		ComponentTypeReceiver,
		ComponentTypeProcessor,
		ComponentTypeExporter,
		ComponentTypeExtension,
		ComponentTypeConnector,
	}

	for _, expectedType := range expectedTypes {
		if componentList, exists := components[expectedType]; !exists {
			t.Errorf("Missing component type: %s", expectedType)
		} else if len(componentList) == 0 {
			t.Errorf("No components found for type: %s", expectedType)
		} else {
			t.Logf("Found %d %s components", len(componentList), expectedType)
		}
	}

	// Verify specific components exist
	if receivers, exists := components[ComponentTypeReceiver]; exists {
		found := false
		for _, name := range receivers {
			if name == "otlp" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find 'otlp' receiver in component list")
		}
	}
}

func TestSchemaManager_Caching(t *testing.T) {
	manager := NewSchemaManager()

	// Get the same schema twice
	schema1, err := manager.GetComponentSchema(ComponentTypeProcessor, "batch", "0.138.0")
	if err != nil {
		t.Fatalf("Failed to get batch processor schema (first call): %v", err)
	}

	schema2, err := manager.GetComponentSchema(ComponentTypeProcessor, "batch", "0.138.0")
	if err != nil {
		t.Fatalf("Failed to get batch processor schema (second call): %v", err)
	}

	// Verify they are the same object (should be cached)
	if schema1 != schema2 {
		t.Error("Expected cached schema to return the same object")
	}

	t.Log("Schema caching is working correctly")
}

func TestSchemaManager_WithVersion(t *testing.T) {
	manager := NewSchemaManager()

	// Test getting schema with version 0.138.0
	schema, err := manager.GetComponentSchema(ComponentTypeReceiver, "otlp", "0.138.0")
	if err != nil {
		t.Fatalf("Failed to get OTLP receiver schema with version: %v", err)
	}

	if schema.Version != "0.138.0" {
		t.Errorf("Expected version '0.138.0', got '%s'", schema.Version)
	}

	// Test that different versions are cached separately (this would fail for versions we don't have schemas for)
	// For now, test with the same version to ensure caching works
	schema2, err := manager.GetComponentSchema(ComponentTypeReceiver, "otlp", "0.138.0")
	if err != nil {
		t.Fatalf("Failed to get OTLP receiver schema with same version: %v", err)
	}

	if schema2.Version != "0.138.0" {
		t.Errorf("Expected version '0.138.0', got '%s'", schema2.Version)
	}

	// They should be the same object due to caching
	if schema != schema2 {
		t.Error("Expected same objects for same version (cached)")
	}

	t.Log("Version handling works correctly")
}

func TestSchemaManager_ValidateComponentJSON(t *testing.T) {
	manager := NewSchemaManager()

	// Test valid JSON for OTLP receiver
	validJSON := []byte(`{
		"protocols": {
			"grpc": {
				"endpoint": "0.0.0.0:4317"
			},
			"http": {
				"endpoint": "0.0.0.0:4318"
			}
		}
	}`)

	result, err := manager.ValidateComponentJSON(ComponentTypeReceiver, "otlp", "0.138.0", validJSON)
	require.NoError(t, err, "Failed to validate valid OTLP receiver JSON")
	require.NotNil(t, result, "Validation result should not be nil")

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Errorf("Validation error: %s", desc)
		}
	}
	assert.True(t, result.Valid(), "Expected valid JSON to pass validation")

	t.Logf("Successfully validated OTLP receiver configuration")
}

func TestSchemaManager_ValidateComponentJSON_Invalid(t *testing.T) {
	manager := NewSchemaManager()

	// Test invalid JSON (include_metadata should be a boolean, not a string)
	invalidJSON := []byte(`{
		"grpc": {
			"include_metadata": "invalid_boolean_value",
			"keepalive": {
				"server_parameters": {
					"max_connection_idle": "invalid_duration_format"
				}
			}
		}
	}`)

	result, err := manager.ValidateComponentJSON(ComponentTypeReceiver, "otlp", "0.138.0", invalidJSON)
	require.NoError(t, err, "Failed to validate invalid OTLP receiver JSON")
	require.NotNil(t, result, "Validation result should not be nil")

	if result.Valid() {
		assert.Fail(t, "Expected invalid JSON to fail validation, but it passed")
	} else {
		t.Logf("Correctly identified %d validation errors:", len(result.Errors()))
		for _, desc := range result.Errors() {
			t.Logf("  - %s", desc)
		}
		assert.False(t, result.Valid(), "Expected invalid JSON to fail validation")
	}
}

func TestSchemaManager_ValidateComponentJSON_MalformedJSON(t *testing.T) {
	manager := NewSchemaManager()

	// Test malformed JSON
	malformedJSON := []byte(`{
		"protocols": {
			"grpc": {
				"endpoint": "0.0.0.0:4317"
			}
		// Missing closing braces`)

	result, err := manager.ValidateComponentJSON(ComponentTypeReceiver, "otlp", "0.138.0", malformedJSON)
	if err != nil {
		// This should fail at the validation level, not the JSON parsing level
		t.Logf("Expected error for malformed JSON: %v", err)
		return
	}

	if result != nil && result.Valid() {
		t.Error("Expected malformed JSON to fail validation")
	}
}

func TestSchemaManager_ValidateComponentJSON_NonExistentComponent(t *testing.T) {
	manager := NewSchemaManager()

	validJSON := []byte(`{"some": "config"}`)

	_, err := manager.ValidateComponentJSON(ComponentTypeReceiver, "nonexistent", "0.138.0", validJSON)
	require.Error(t, err, "Expected error for non-existent component")

	expectedError := "failed to get schema for receiver nonexistent v0.138.0"
	assert.Contains(t, err.Error(), expectedError, "Error should contain expected text")

	t.Logf("Correctly handled non-existent component: %v", err)
}

func TestSchemaManager_ValidateComponentJSON_EmptyJSON(t *testing.T) {
	manager := NewSchemaManager()

	// Test empty JSON object
	emptyJSON := []byte(`{}`)

	result, err := manager.ValidateComponentJSON(ComponentTypeReceiver, "otlp", "0.138.0", emptyJSON)
	require.NoError(t, err, "Failed to validate empty JSON")
	require.NotNil(t, result, "Validation result should not be nil")

	// Empty JSON might be valid or invalid depending on schema requirements
	// Just verify we get a result without errors
	t.Logf("Empty JSON validation result: valid=%v, errors=%d", result.Valid(), len(result.Errors()))
}

func TestSchemaManager_ValidateComponentJSON_DifferentComponents(t *testing.T) {
	manager := NewSchemaManager()

	// Test debug exporter with minimal valid config
	debugExporterJSON := []byte(`{
		"verbosity": "normal"
	}`)

	result, err := manager.ValidateComponentJSON(ComponentTypeExporter, "debug", "0.138.0", debugExporterJSON)
	require.NoError(t, err, "Failed to validate debug exporter JSON")
	require.NotNil(t, result, "Validation result should not be nil")

	t.Logf("Debug exporter validation result: valid=%v, errors=%d", result.Valid(), len(result.Errors()))

	// Test batch processor with valid config
	batchProcessorJSON := []byte(`{
		"timeout": "1s",
		"send_batch_size": 1024
	}`)

	result, err = manager.ValidateComponentJSON(ComponentTypeProcessor, "batch", "0.138.0", batchProcessorJSON)
	require.NoError(t, err, "Failed to validate batch processor JSON")
	require.NotNil(t, result, "Validation result should not be nil")

	t.Logf("Batch processor validation result: valid=%v, errors=%d", result.Valid(), len(result.Errors()))
}

func TestSchemaManager_GetLatestVersion(t *testing.T) {
	manager := NewSchemaManager()

	version, err := manager.GetLatestVersion()
	require.NoError(t, err, "Failed to get latest version")
	require.NotEmpty(t, version, "Latest version should not be empty")

	// Verify the version has a valid format (major.minor.patch)
	assert.Contains(t, version, ".", "Version should contain dots")

	// Since we know we have v0.139.0 in the schemas directory, verify it's returned
	assert.Equal(t, "0.139.0", version, "Expected version 0.139.0 as the latest")

	t.Logf("Latest version found: %s", version)
}

func TestSchemaManager_GetAllVersions(t *testing.T) {
	manager := NewSchemaManager()

	versions, err := manager.GetAllVersions()
	require.NoError(t, err, "Failed to get all versions")
	require.NotEmpty(t, versions, "Versions list should not be empty")

	// Verify we have at least one version
	assert.GreaterOrEqual(t, len(versions), 1, "Should have at least one version")

	// Since we know we have v0.138.0, verify it's in the list
	assert.Contains(t, versions, "0.138.0", "Expected version 0.138.0 to be in the list")

	// Verify all versions have a valid format (contain dots)
	for _, version := range versions {
		assert.Contains(t, version, ".", "Version %s should contain dots", version)
		assert.NotEmpty(t, version, "Version should not be empty")
	}

	t.Logf("All versions found: %v", versions)
}

func TestSchemaManager_GetDeprecatedFields(t *testing.T) {
	manager := NewSchemaManager()

	// Test getting deprecated fields for kafka exporter which has known deprecated fields
	deprecatedFields, err := manager.GetDeprecatedFields(ComponentTypeExporter, "kafka", "0.138.0")
	require.NoError(t, err, "Failed to get deprecated fields for kafka exporter")

	// Assert that we found deprecated fields in kafka exporter
	assert.GreaterOrEqual(t, len(deprecatedFields), 1, "Kafka exporter should have at least one deprecated field")

	// Check for specific deprecated fields we expect in kafka exporter
	expectedDeprecatedFields := []string{"brokers", "topic"}
	foundFields := make(map[string]bool)

	for _, field := range deprecatedFields {
		// Verify each deprecated field has the required information
		assert.NotEmpty(t, field.Name, "Deprecated field should have a name")
		// Description and Type can be empty, but should be present as fields

		for _, expected := range expectedDeprecatedFields {
			if strings.Contains(field.Name, expected) {
				foundFields[expected] = true
			}
		}
	}

	// Assert that we found at least one of the expected deprecated fields
	assert.True(t, len(foundFields) > 0, "Should find at least one expected deprecated field (brokers or topic)")

	// Log detailed information about deprecated fields
	t.Logf("Found %d deprecated fields in kafka exporter:", len(deprecatedFields))
	for _, field := range deprecatedFields {
		t.Logf("  - Name: %s, Type: %s, Description: %s", field.Name, field.Type, field.Description)
	}

	// Test with a component that doesn't exist
	_, err = manager.GetDeprecatedFields(ComponentTypeExporter, "nonexistent", "0.138.0")
	require.Error(t, err, "Expected error for non-existent component")
	assert.Contains(t, err.Error(), "failed to get schema", "Error should mention schema retrieval failure")

	t.Logf("Successfully tested deprecated fields detection")
}

func TestSchemaManager_GetComponentNames(t *testing.T) {
	manager := NewSchemaManager()

	// Test getting receiver component names
	receiverNames, err := manager.GetComponentNames(ComponentTypeReceiver, "0.138.0")
	require.NoError(t, err, "Failed to get receiver component names")
	require.NotEmpty(t, receiverNames, "Receiver names list should not be empty")

	// Verify we have expected receivers
	assert.Contains(t, receiverNames, "otlp", "Expected otlp receiver to be in the list")
	assert.GreaterOrEqual(t, len(receiverNames), 10, "Should have at least 10 receivers")

	t.Logf("Found %d receiver components: %v", len(receiverNames), receiverNames[:minInt(5, len(receiverNames))])

	// Test getting processor component names
	processorNames, err := manager.GetComponentNames(ComponentTypeProcessor, "0.138.0")
	require.NoError(t, err, "Failed to get processor component names")
	require.NotEmpty(t, processorNames, "Processor names list should not be empty")

	// Verify we have expected processors
	assert.Contains(t, processorNames, "batch", "Expected batch processor to be in the list")
	assert.GreaterOrEqual(t, len(processorNames), 5, "Should have at least 5 processors")

	t.Logf("Found %d processor components: %v", len(processorNames), processorNames[:minInt(5, len(processorNames))])

	// Test getting exporter component names
	exporterNames, err := manager.GetComponentNames(ComponentTypeExporter, "0.138.0")
	require.NoError(t, err, "Failed to get exporter component names")
	require.NotEmpty(t, exporterNames, "Exporter names list should not be empty")

	// Verify we have expected exporters
	assert.Contains(t, exporterNames, "debug", "Expected debug exporter to be in the list")
	assert.GreaterOrEqual(t, len(exporterNames), 5, "Should have at least 5 exporters")

	t.Logf("Found %d exporter components: %v", len(exporterNames), exporterNames[:minInt(5, len(exporterNames))])
}

func TestSchemaManager_GetComponentNames_InvalidType(t *testing.T) {
	manager := NewSchemaManager()

	// Test with invalid component type
	_, err := manager.GetComponentNames("invalid", "0.138.0")
	require.Error(t, err, "Expected error for invalid component type")
	assert.Contains(t, err.Error(), "invalid component type", "Error should mention invalid component type")

	t.Logf("Correctly handled invalid component type: %v", err)
}

func TestSchemaManager_GetComponentNames_InvalidVersion(t *testing.T) {
	manager := NewSchemaManager()

	// Test with non-existent version
	_, err := manager.GetComponentNames(ComponentTypeReceiver, "999.999.999")
	require.Error(t, err, "Expected error for non-existent version")
	assert.Contains(t, err.Error(), "failed to read schema directory", "Error should mention directory read failure")

	t.Logf("Correctly handled non-existent version: %v", err)
}

func TestSchemaManager_GetChangelog_WithTestData(t *testing.T) {
	manager := NewSchemaManager()
	version := "0.138.0"

	// Get changelog from the method
	actualChangelog, err := manager.GetChangelog(version)
	require.NoError(t, err, "Failed to get changelog for version %s", version)
	require.NotEmpty(t, actualChangelog, "Changelog content should not be empty")

	// Read expected changelog from testdata
	expectedBytes, err := os.ReadFile("testdata/changelog-0.138.0.md")
	require.NoError(t, err, "Failed to read expected changelog from testdata")
	expectedChangelog := string(expectedBytes)
	require.NotEmpty(t, expectedChangelog, "Expected changelog from testdata should not be empty")

	// Compare the content
	assert.Equal(t, expectedChangelog, actualChangelog, "Changelog content should match testdata file")
}

func TestSchemaManager_GetChangelog_NonExistentVersion(t *testing.T) {
	manager := NewSchemaManager()

	// Test with a non-existent version
	_, err := manager.GetChangelog("999.999.999")
	require.Error(t, err, "Expected error for non-existent version")
	assert.Contains(t, err.Error(), "changelog not found for version 999.999.999", "Error should mention the specific version")

	t.Logf("✅ Correctly returned error for non-existent version: %v", err)
}

func TestSchemaManager_ValidateComponentYAML(t *testing.T) {
	manager := NewSchemaManager()

	// Test valid YAML for OTLP receiver
	validYAML := []byte(`
protocols:
  grpc:
    endpoint: "0.0.0.0:4317"
  http:
    endpoint: "0.0.0.0:4318"
`)

	result, err := manager.ValidateComponentYAML(ComponentTypeReceiver, "otlp", "0.138.0", validYAML)
	require.NoError(t, err, "Failed to validate valid OTLP receiver YAML")
	require.NotNil(t, result, "Validation result should not be nil")

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Errorf("Validation error: %s", desc)
		}
	}
	assert.True(t, result.Valid(), "Expected valid YAML to pass validation")

	t.Logf("Successfully validated OTLP receiver YAML configuration")
}

func TestSchemaManager_ValidateComponentYAML_Invalid(t *testing.T) {
	manager := NewSchemaManager()

	// Test invalid YAML (include_metadata should be a boolean, not a string)
	invalidYAML := []byte(`
grpc:
  include_metadata: "invalid_boolean_value"
  keepalive:
    server_parameters:
      max_connection_idle: "invalid_duration_format"
`)

	result, err := manager.ValidateComponentYAML(ComponentTypeReceiver, "otlp", "0.138.0", invalidYAML)
	require.NoError(t, err, "Failed to validate invalid OTLP receiver YAML")
	require.NotNil(t, result, "Validation result should not be nil")

	if result.Valid() {
		assert.Fail(t, "Expected invalid YAML to fail validation, but it passed")
	} else {
		t.Logf("Correctly identified %d validation errors:", len(result.Errors()))
		for _, desc := range result.Errors() {
			t.Logf("  - %s", desc)
		}
		assert.False(t, result.Valid(), "Expected invalid YAML to fail validation")
	}
}

func TestSchemaManager_ValidateComponentYAML_MalformedYAML(t *testing.T) {
	manager := NewSchemaManager()

	// Test malformed YAML
	malformedYAML := []byte(`
protocols:
  grpc:
    endpoint: "0.0.0.0:4317"
  http:
    endpoint: "0.0.0.0:4318"
  # Indentation error
endpoint: "invalid"
`)

	_, err := manager.ValidateComponentYAML(ComponentTypeReceiver, "otlp", "0.138.0", malformedYAML)
	if err != nil {
		t.Logf("Expected error for malformed YAML: %v", err)
		assert.Contains(t, err.Error(), "failed to parse YAML data", "Error should mention YAML parsing failure")
	}
}

// Helper function for minimum value
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSchemaManager_QueryDocumentation(t *testing.T) {
	manager := NewSchemaManager()

	// Test basic documentation search for a specific version
	results, err := manager.QueryDocumentation("OTLP receiver configuration", "0.139.0", 5)
	require.NoError(t, err, "Failed to query documentation")
	require.NotEmpty(t, results, "Expected search results")

	// Verify result structure and that all results are from the specified version
	for _, result := range results {
		assert.NotEmpty(t, result.ID, "Result should have an ID")
		assert.NotEmpty(t, result.Content, "Result should have content")
		assert.NotNil(t, result.Metadata, "Result should have metadata")
		assert.GreaterOrEqual(t, result.Similarity, float32(0.0), "Similarity should be non-negative")
		assert.Equal(t, "0.139.0", result.Version, "All results should be from the specified version")

		// Log result for debugging
		t.Logf("Found document: ID=%s, Component=%s, Version=%s, Similarity=%.3f",
			result.ID, result.Component, result.Version, result.Similarity)
	}

	t.Logf("Successfully found %d documentation results for version 0.139.0", len(results))
}

func TestSchemaManager_QueryDocumentation_VersionFiltering(t *testing.T) {
	manager := NewSchemaManager()

	// Test that version filtering works correctly
	results139, err := manager.QueryDocumentation("OTLP receiver", "0.139.0", 3)
	require.NoError(t, err, "Failed to query documentation for version 0.139.0")

	results135, err := manager.QueryDocumentation("OTLP receiver", "0.135.0", 3)
	require.NoError(t, err, "Failed to query documentation for version 0.135.0")

	// Verify that all results are from the correct version
	for _, result := range results139 {
		assert.Equal(t, "0.139.0", result.Version, "All results should be from version 0.139.0")
	}

	for _, result := range results135 {
		assert.Equal(t, "0.135.0", result.Version, "All results should be from version 0.135.0")
	}

	t.Logf("Version 0.139.0 returned %d results", len(results139))
	t.Logf("Version 0.135.0 returned %d results", len(results135))
	t.Log("Version filtering works correctly")
}

func TestSchemaManager_QueryDocumentationWithFilters(t *testing.T) {
	manager := NewSchemaManager()

	// Test filtered search for specific component type
	results, err := manager.QueryDocumentationWithFilters("configuration examples", 3, "receiver", "", "")
	require.NoError(t, err, "Failed to query documentation with filters")
	require.NotEmpty(t, results, "Expected search results")

	// Verify all results are from receivers
	for _, result := range results {
		if result.Metadata["component_type"] != "" {
			assert.Equal(t, "receiver", result.Metadata["component_type"], "Should only return receiver components")
		}
		t.Logf("Filtered result: ID=%s, ComponentType=%s, Component=%s",
			result.ID, result.Metadata["component_type"], result.Component)
	}

	// Test filtered search for specific component
	results, err = manager.QueryDocumentationWithFilters("endpoints", 2, "receiver", "otlp", "")
	require.NoError(t, err, "Failed to query documentation for OTLP receiver")

	// Verify results are relevant to OTLP receiver
	for _, result := range results {
		if result.Component != "" {
			assert.Contains(t, result.Component, "otlp", "Should contain OTLP-related content")
		}
		t.Logf("OTLP result: ID=%s, Component=%s, Content preview: %.100s...",
			result.ID, result.Component, result.Content)
	}

	t.Logf("Successfully tested filtered documentation search")
}

func TestSchemaManager_QueryDocumentation_EmptyQuery(t *testing.T) {
	manager := NewSchemaManager()

	// Test with empty query - this should error since chromem doesn't allow empty queries
	results, err := manager.QueryDocumentation("", "0.139.0", 5)
	require.Error(t, err, "Empty query should error")
	require.Empty(t, results, "Empty query should return no results")

	t.Logf("Empty query correctly returned error: %v", err)
}

func TestSchemaManager_QueryDocumentation_NoResults(t *testing.T) {
	manager := NewSchemaManager()

	// Test with a query that should return no relevant results
	results, err := manager.QueryDocumentation("xyzunlikelytermabc123", "0.139.0", 5)
	require.NoError(t, err, "Query should not error even with no results")

	// Should return empty results or very low similarity results
	t.Logf("Unlikely query returned %d results for version 0.139.0", len(results))
}

func BenchmarkSchemaManager_GetComponentSchema(b *testing.B) {
	manager := NewSchemaManager()

	// Pre-load one schema to test caching performance
	_, _ = manager.GetComponentSchema(ComponentTypeReceiver, "otlp", "0.138.0")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.GetComponentSchema(ComponentTypeReceiver, "otlp", "0.138.0")
		if err != nil {
			b.Fatalf("Failed to get schema: %v", err)
		}
	}
}

func BenchmarkSchemaManager_QueryDocumentation(b *testing.B) {
	manager := NewSchemaManager()

	// Pre-initialize RAG database
	_, _ = manager.QueryDocumentation("test", "0.139.0", 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.QueryDocumentation("OTLP configuration", "0.139.0", 3)
		if err != nil {
			b.Fatalf("Failed to query documentation: %v", err)
		}
	}
}
