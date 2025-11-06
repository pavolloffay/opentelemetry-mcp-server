# OpenTelemetry MCP Server Tools Documentation

This document lists all available tools from the OpenTelemetry MCP server with their descriptions and parameters.

## Available Tools

### 1. opentelemetry-collector-changelog
**Description:** Returns OpenTelemetry collector changelog

**Parameters:**
- `version` (optional, string): The OpenTelemetry Collector version e.g. 0.138.0

---

### 2. opentelemetry-collector-component-deprecated-fields
**Description:** Return deprecated OpenTelemetry collector receiver, exporter, processor, connector and extension configuration fields

**Parameters:**
- `kind` (required, string): Collector component kind. It can be receiver, exporter, extension.
- `names` (required, array of strings): Collector component names e.g. ["otlp", "jaeger"]
- `version` (optional, string): The OpenTelemetry Collector version e.g. 0.138.0

---

### 3. opentelemetry-collector-component-schema
**Description:** Explain OpenTelemetry collector receiver, exporter, processor, connector and extension configuration schema

**Parameters:**
- `kind` (required, string): Collector component kind. It can be receiver, exporter, processor, connector and extension.
- `name` (required, string): Collector component name e.g. otlp
- `version` (optional, string): The OpenTelemetry Collector version e.g. 0.138.0

---

### 4. opentelemetry-collector-component-schema-validation
**Description:** Validate OpenTelemetry collector processor, receiver, exporter, extension configuration JSON

**Parameters:**
- `kind` (required, string): Collector component kind. It can be receiver, exporter, processor, connector and extension.
- `name` (required, string): Collector component name e.g. otlp
- `config` (required, string): Collector component configuration JSON
- `version` (optional, string): The OpenTelemetry Collector version e.g. 0.138.0

---

### 5. opentelemetry-collector-components
**Description:** Get all OpenTelemetry collector components

**Parameters:**
- `kind` (required, string): Collector component kind. It can be receiver, exporter, processor, connector and extension.
- `version` (optional, string): The OpenTelemetry Collector version e.g. 0.138.0

---

### 6. opentelemetry-collector-get-versions
**Description:** Get all supported OpenTelemetry collector versions by this tool

**Parameters:**
- No parameters required (empty object)

---

### 7. opentelemetry-collector-rag
**Description:** Answer questions about OpenTelemetry collector

**Parameters:**
- `query` (required, string): Query about OpenTelemetry collector's documentation
- `version` (required, string): The OpenTelemetry Collector version e.g. 0.138.0
- `kind` (optional, string): Collector component kind. It can be receiver, exporter, processor, connector and extension. If kind is provided name has to be provided as well.
- `name` (optional, string): Collector component name e.g. otlp. If name is provided kind has to be provided as well.

---

### 8. opentelemetry-collector-readme
**Description:** Explain OpenTelemetry collector processor, receiver, exporter, extension functionality and use-cases

**Parameters:**
- `kind` (required, string): Collector component kind. It can be receiver, exporter, processor, connector and extension.
- `name` (required, string): Collector component name e.g. otlp
- `version` (optional, string): The OpenTelemetry Collector version e.g. 0.138.0

---