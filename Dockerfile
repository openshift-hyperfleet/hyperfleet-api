ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot

# OpenAPI generation stage
FROM golang:1.25 AS builder

ARG GIT_SHA=unknown
ARG GIT_DIRTY=""

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux make build

# Runtime stage
FROM ${BASE_IMAGE}

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/hyperfleet-api /app/hyperfleet-api

# Copy OpenAPI schema for validation (uses the source spec, not the generated one)
COPY --from=builder /build/openapi/openapi.yaml /app/openapi/openapi.yaml

# Set default schema path (can be overridden by Helm for provider-specific schemas)
ENV OPENAPI_SCHEMA_PATH=/app/openapi/openapi.yaml

EXPOSE 8000

ENTRYPOINT ["/app/hyperfleet-api"]
CMD ["serve"]

LABEL name="hyperfleet-api" \
      vendor="Red Hat" \
      version="0.0.1" \
      summary="HyperFleet API - Cluster Lifecycle Management Service" \
      description="HyperFleet API for cluster lifecycle management"
