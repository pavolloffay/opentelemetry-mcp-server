Go MCP Server Plan & Instructions
This project provides a simple MCP server written in Go, satisfying the following requirements:

Written in Go 1.25

Uses github.com/mark3labs/mcp-go as the MCP framework.

Uses github.com/spf13/cobra for configuration and command-line parsing.

Exposes a single example tool (greet).

Can run over two different protocols:

stdio (referred to as "studio" in requirements): This is the standard transport for local clients like Cursor or Claude Desktop.

http: This exposes the server over a network endpoint.

File Structure
README.md: This file, outlining the plan and usage.

go.mod: Defines the module, Go version, and dependencies.

main.go: The complete application source code.

Code Breakdown (main.go)
Imports: We import cobra, mcp-go/mcp, and mcp-go/server, along with standard libraries.

Cobra Command (rootCmd):

A root command mcp-server is created using cobra.

Two flags are added:

--protocol (string, default: "stdio"): Lets you choose the transport. Can be set to "stdio" or "http".

--addr (string, default: ":8080"): Specifies the listen address for the http protocol.

The RunE function of the command contains the main startup logic.

Server Creation (runServer function):

This function is called by rootCmd->RunE.

It initializes a new server.NewMCPServer.

It defines a single example tool, greetTool, using mcp.NewTool. This tool takes one required string argument, name.

It defines a handler, greetHandler, which implements the tool's logic: it extracts the name argument and returns a simple greeting.

It registers the tool and its handler with the server using s.AddTool.

Protocol Handling:

A switch statement checks the --protocol flag.

stdio: If "stdio", it logs that it's starting and calls server.ServeStdio(s), which blocks and serves over standard input/output.

http: If "http", it creates an http.ServeMux, wraps the MCP server in a server.NewStreamableHTTPServer, and registers it at the /mcp endpoint. It then starts a standard Go HTTP server.

main() Function:

The main function is minimal and just executes the Cobra root command.
How to Build and Run
Initialize Project:

# Tidy dependencies
go mod tidy

Build:

# Build the binary
go build -o mcp-server .

Run (Option 1: stdio): This is the default. This mode is used by clients like Cursor or Claude Desktop, which will execute the binary directly.

./mcp-server
# Or explicitly:
./mcp-server --protocol=stdio

The server will start and wait for JSON-RPC messages on stdin.

Run (Option 2: http): This will run a network server.

./mcp-server --protocol=http --addr=":8080"

The server will start, and you can now (for testing) send MCP requests to http://localhost:8080/mcp.