FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY ./ ./
RUN make build

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /app
COPY --from=builder /app/mcp-server /app/mcp-server
USER 65532:65532
ENTRYPOINT ["/app/mcp-server", "--protocol", "http", "--addr", ":8080"]

EXPOSE 8080
