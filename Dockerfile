# OpenAPI generation stage
FROM openapitools/openapi-generator-cli:v7.16.0 AS openapi-gen

WORKDIR /local

# Copy OpenAPI spec
COPY openapi/openapi.yaml /local/openapi/openapi.yaml

# Generate Go client/models from OpenAPI spec
RUN bash /usr/local/bin/docker-entrypoint.sh generate \
    -i /local/openapi/openapi.yaml \
    -g go \
    -o /local/pkg/api/openapi && \
    rm -f /local/pkg/api/openapi/go.mod /local/pkg/api/openapi/go.sum && \
    rm -rf /local/pkg/api/openapi/test

# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy generated OpenAPI code from openapi-gen stage
COPY --from=openapi-gen /local/pkg/api/openapi ./pkg/api/openapi

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o hyperfleet-api ./cmd/hyperfleet-api

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/hyperfleet-api /app/hyperfleet-api

EXPOSE 8000

ENTRYPOINT ["/app/hyperfleet-api"]
CMD ["serve"]

LABEL name="hyperfleet-api" \
      vendor="Red Hat" \
      version="0.0.1" \
      summary="HyperFleet API - Cluster Lifecycle Management Service" \
      description="HyperFleet API for cluster lifecycle management"
