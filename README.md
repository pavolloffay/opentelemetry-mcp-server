# OpenTelemetry Model Context Protocol (MCP) Server

The OpenTelemetry MCP server enables LLM to efficiently use OpenTelemetry stack.

The MCP uses [opentelemetry-collector-config-schema](./modules/collectorschema) for the collector config validation.


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

## Other OpenTelemetry MCP servers

* https://github.com/mottibec/otelcol-mcp - collector config
* https://github.com/shiftyp/otel-mcp-server - data profiling, requires OpenSearch
* https://github.com/austinlparker/otel-mcp - config, data profiling
* https://github.com/liatrio-labs/otel-instrumentation-mcp - instrumentation

### https://github.com/austinlparker/otel-mcp

```bash
docker run --rm -it -v $(pwd)/collector.yaml:/tmp/collector.yaml:Z pavolloffay/otelcol-with-mcp:0.1 --config
claude mcp add --transport=http otelcol http://localhost:9999/mcp --scope user
```

This MCP server is implemented as collector extension and connector which provides live view on the data.

There are tool to get the collector config, schema for each component which is build into the collector and perform validation.

The returned schema is incomplete, does not contain field explanation and type.

```bash
● OTLP Receiver Schema

  Component Type: otlpKind: receiverConfig Type: *otlpreceiver.Config

  Schema Structure

  {
    "protocols": {
      "grpc": null,
      "http": null
    }
  }

  Current Configuration (with defaults)

  {
    "protocols": {
      "grpc": {
        "endpoint": "0.0.0.0:4317",
        "keepalive": {
          "enforcement_policy": {},
          "server_parameters": {}
        },
        "read_buffer_size": 524288,
        "transport": "tcp"
      },
      "http": {
        "cors": null,
        "endpoint": "0.0.0.0:4318",
        "idle_timeout": 0,
        "keep_alives_enabled": true,
        "logs_url_path": "/v1/logs",
        "metrics_url_path": "/v1/metrics",
        "read_header_timeout": 0,
        "tls": null,
        "traces_url_path": "/v1/traces",
        "write_timeout": 0
      }
    }
  }
```