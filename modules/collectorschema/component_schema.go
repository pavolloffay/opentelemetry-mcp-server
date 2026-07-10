package collectorschema

import (
	"embed"

	"github.com/pavolloffay/opentelemetry-mcp-server/modules/schemagen"
)

//go:embed schemas
var embeddedSchemas embed.FS

// NewSchemaManager creates a new schema manager using embedded schemas
func NewSchemaManager() *schemagen.SchemaManager {
	return schemagen.NewSchemaManagerFromFS(embeddedSchemas, "schemas")
}
