# OpenTelemetry Model Context Protocol (MCP) Server

The OpenTelemetry MCP server enables LLM to efficiently use OpenTelemetry stack.

The MCP uses [opentelemetry-collector-config-schema](https://github.com/pavolloffay/opentelemetry-collector-config-schema/tree/main) for the collector config validation.

## Install & Run

```bash
go install github.com/pavolloffay/opentelemetry-mcp-server@latest
opentelemetry-mcp-server --protocol http --addr 0.0.0.0:8080

claude mcp add --transport=http otel http://localhost:8080/mcp --scope user
```

## Functionality

This MCP server helps with the following use-cases:

### Collector

* Enable LLM to understand OpenTelemetry collector use-cases for each collector component.
* Enable LLM to understand which collector components are included in each version.
* Enable LLM to construct a valid collector configuration.
* Enable LLM to validate collector configuration.
* Enable LLM to find and fix deprecated configuration.
* Enable LLM to understand deprecated configuration between versions.

### Instrumentation

TBD

## Future work / Roadmap

* Enable LLM to understand data the collector is processing. 
  * What are the (resource) attributes and values?
  * It can help LLM to write PII (filtering) rules, specific for an organization.
* Enable LLM to understand which workloads are sending telemetry data.
* Enable LLM to understand how much data each workload is sending.


## References

* https://github.com/mottibec/otelcol-mcp
* https://github.com/shiftyp/otel-mcp-server - requires OpenSearch