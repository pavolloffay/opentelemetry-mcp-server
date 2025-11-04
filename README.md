# OpenTelemetry Model Context Protocol (MCP) Server

The OpenTelemetry MCP server enables LLM to efficiently use OpenTelemetry stack.

The MCP uses [opentelemetry-collector-config-schema](https://github.com/pavolloffay/opentelemetry-collector-config-schema/tree/main) for the collector config validation.


https://github.com/user-attachments/assets/af56cda4-9ab0-4c93-969e-a2ee7b9f0480


## Install & Run

```bash
go install github.com/pavolloffay/opentelemetry-mcp-server@latest
opentelemetry-mcp-server --protocol http --addr 0.0.0.0:8080

# or docker run --rm -it -p 8080:8080 ghcr.io/pavolloffay/opentelemetry-mcp-server:latest --protocol http --addr 0.0.0.0:8080 

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
* Enable LLM to understand configuration differences between collector versions.

### Instrumentation - Future work

* Enable LLM to correctly instrument code.
  * Enable LLM identify uninstrumented code (e.g. RPC frameworks that should be instrumented).
* Enable LLM to identify high cardinality attributes explicitly added.
* Enable LLM to identify PII attributes explicitly added.
* Enable LLM to fix issues in the codebase based on the telemetry data.

## Future work / Roadmap

* Enable LLM to understand data the collector is processing. 
  * What are the (resource) attributes and values?
  * It can help LLM to write PII (filtering) rules, specific for an organization.
* Enable LLM to understand which workloads are sending telemetry data.
* Enable LLM to understand how much data each workload is sending.
* Enable LLM to tweak sampling configuration based on the collected data.
* Enable LLM to size storage based on the collected data volumes.
* Enable LLM to validate OpenTelemetry transformation language.

## References

* https://github.com/mottibec/otelcol-mcp - collector config
* https://github.com/shiftyp/otel-mcp-server - data profiling, requires OpenSearch
* https://github.com/austinlparker/otel-mcp - config, data profiling
* https://github.com/liatrio-labs/otel-instrumentation-mcp - instrumentation
