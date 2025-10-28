FROM golang:latest AS builder

WORKDIR /app

COPY ./ ./
RUN make build

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /app
COPY --from=builder /app/opentelemetry-mcp-server /app/opentelemetry-mcp-server
USER 65532:65532
ENTRYPOINT ["/app/opentelemetry-mcp-server"]

EXPOSE 8080
