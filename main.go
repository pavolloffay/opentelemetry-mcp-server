package main

import (
	"log"
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/pavolloffay/opentelemetry-mcp-server/internal/tools"
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

func runServer(cmd *cobra.Command, _ []string) error {
	protocol, _ := cmd.Flags().GetString("protocol")
	addr, _ := cmd.Flags().GetString("addr")

	// Create a new MCP server
	s := server.NewMCPServer(
		"otel-mcp-server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Get all tools from the tools package
	allTools, err := tools.GetAllTools()
	if err != nil {
		return err
	}

	// Register all tools with the server
	for _, tool := range allTools {
		s.AddTool(tool.Tool, tool.Handler)
	}

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
		log.Fatalf("unsupported protocol: %s", protocol)
		return nil
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
