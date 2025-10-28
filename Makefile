.PHONY: build clean test

# Build the MCP server binary
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o mcp-server .

# Clean build artifacts
clean:
	rm -f mcp-server

# Run tests
test:
	go test ./...

# Build Docker image
docker-build:
	docker build -t opentelemetry-mcp-server:latest .

# Run the server
run:
	./mcp-server

# Development build (with debug info)
build-dev:
	go build -o mcp-server .