.PHONY: build docker-build

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o opentelemetry-mcp-server .

docker-build:
	docker build -t opentelemetry-mcp-server:latest .
