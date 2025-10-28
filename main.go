package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	collectorschema "github.com/pavolloffay/opentelemetry-collector-config-schema"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mcp-server",
	Short: "A simple MCP server written in Go",
	RunE:  runServer,
}

func init() {
	rootCmd.Flags().String("protocol", "stdio", "Transport protocol: stdio or http")
	rootCmd.Flags().String("addr", ":8080", "Listen address for http protocol")
}

func runServer(cmd *cobra.Command, args []string) error {
	protocol, _ := cmd.Flags().GetString("protocol")
	addr, _ := cmd.Flags().GetString("addr")

	// Create a new MCP server
	s := server.NewMCPServer(
		"otel-mcp-server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	schemaManager := collectorschema.NewSchemaManager()
	latestCollectorVersion, err := schemaManager.GetLatestVersion()
	if err != nil {
		return err
	}
	collectorVersionsTool := mcp.NewTool("opentelemetry-collector-get-versions",
		mcp.WithDescription("Get all supported OpenTelemetry collector versions by this tool"),
	)
	collectorVersionsHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		versions, err := schemaManager.GetAllVersions()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get all supported versions by this toool: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("versions: %s", versions)), nil
	}

	collectorComponentsTool := mcp.NewTool("opentelemetry-collector-components",
		mcp.WithDescription("Get all OpenTelemetry collector components"),
		mcp.WithString("collector-version",
			mcp.Description("The OpenTelemetry Collector version e.g. 0.138.0"),
		),
		mcp.WithString("type",
			mcp.Required(),
			mcp.Description("Collector component type. It can be receiver, exporter, extension, processor, connector."),
		),
	)
	collectorComponentsHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		componentType, err := request.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("type argument is required: %v", err)), nil
		}
		version := request.GetString("version", latestCollectorVersion)

		components, err := schemaManager.GetComponentNames(collectorschema.ComponentType(componentType), version)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get components for %s: %v", componentType, err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s", components)), nil
	}

	collectorReadmeTool := mcp.NewTool("opentelemetry-collector-readme",
		mcp.WithDescription("Explain OpenTelemetry collector processor, receiver, exporter, extension functionality and use-cases"),
		mcp.WithString("collector-version",
			mcp.Description("The OpenTelemetry Collector version e.g. 0.138.0"),
		),
		mcp.WithString("type",
			mcp.Required(),
			mcp.Description("Collector component type. It can be receiver, exporter, extension."),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Collector component name e.g. otlp"),
		),
	)
	collectorReadmeHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		componentType, err := request.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("type argument is required: %v", err)), nil
		}
		componentName, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("name argument is required: %v", err)), nil
		}
		version := request.GetString("version", latestCollectorVersion)

		readme, err := schemaManager.GetComponentReadme(collectorschema.ComponentType(componentType), componentName, version)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get readme for %s %s: %v", componentType, componentName, err)), nil
		}
		return mcp.NewToolResultText(readme), nil
	}

	collectorSchemaGetTool := mcp.NewTool("opentelemetry-collector-component-schema",
		mcp.WithDescription("Explain OpenTelemetry collector processor, receiver, exporter, extension, connector configuration schema"),
		mcp.WithString("collector-version",
			mcp.Description("The OpenTelemetry Collector version e.g. 0.138.0"),
		),
		mcp.WithString("type",
			mcp.Required(),
			mcp.Description("Collector component type. It can be receiver, exporter, extension."),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Collector component name e.g. otlp"),
		),
	)
	collectorSchemaGetHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		componentType, err := request.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("type argument is required: %v", err)), nil
		}
		componentName, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("name argument is required: %v", err)), nil
		}
		version := request.GetString("version", latestCollectorVersion)

		schemaJSON, err := schemaManager.GetComponentSchemaJSON(collectorschema.ComponentType(componentType), componentName, version)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get schema for %s/%s@%s: %v", componentType, componentName, version, err)), nil
		}
		return mcp.NewToolResultText(string(schemaJSON)), nil
	}

	collectorSchemaValidationTool := mcp.NewTool("opentelemetry-collector-component-schema-validation",
		mcp.WithDescription("Validate OpenTelemetry collector processor, receiver, exporter, extension configuration JSON"),
		mcp.WithString("collector-version",
			mcp.Description("The OpenTelemetry Collector version e.g. 0.138.0"),
		),
		mcp.WithString("type",
			mcp.Required(),
			mcp.Description("Collector component type. It can be receiver, exporter, extension."),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Collector component name e.g. otlp"),
		),
		mcp.WithString("config",
			mcp.Required(),
			mcp.Description("Collector component configuration JSON"),
		),
	)
	collectorSchemaValidationHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		componentType, err := request.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("type argument is required: %v", err)), nil
		}
		componentName, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("name argument is required: %v", err)), nil
		}
		config, err := request.RequireString("config")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("config argument is required: %v", err)), nil
		}
		version := request.GetString("version", latestCollectorVersion)

		validationResult, err := schemaManager.ValidateComponentJSON(collectorschema.ComponentType(componentType), componentName, version, []byte(config))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to validate json for %s/%s@%s: %v", componentType, componentName, version, err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("is valid: %v, errors: %v", validationResult.Valid(), validationResult.Errors())), nil
	}

	s.AddTool(collectorReadmeTool, collectorReadmeHandler)
	s.AddTool(collectorSchemaGetTool, collectorSchemaGetHandler)
	s.AddTool(collectorSchemaValidationTool, collectorSchemaValidationHandler)
	s.AddTool(collectorVersionsTool, collectorVersionsHandler)
	s.AddTool(collectorComponentsTool, collectorComponentsHandler)

	// Handle different protocols
	switch protocol {
	case "stdio":
		log.Println("Starting MCP server on stdio...")
		return server.ServeStdio(s)
	case "http":
		log.Printf("Starting MCP server on http at %s...", addr)
		mux := http.NewServeMux()
		httpServer := server.NewStreamableHTTPServer(s)
		mux.Handle("/mcp", httpServer)

		return http.ListenAndServe(addr, mux)
	default:
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
