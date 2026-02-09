ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH=amd64

# Build stage - explicitly use amd64 for cross-compilation from x86 hosts
FROM --platform=linux/amd64 golang:1.25 AS builder

ARG GIT_SHA=unknown
ARG GIT_DIRTY=""
ARG TARGETARCH

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary for target architecture
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} make build

# Runtime stage - use target architecture for the base image
ARG BASE_IMAGE
ARG TARGETARCH
FROM --platform=linux/${TARGETARCH} ${BASE_IMAGE}

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/hyperfleet-api /app/hyperfleet-api

# CRD definitions are now loaded from Kubernetes API at runtime
# OpenAPI schema is generated dynamically from CRDs
# For provider-specific schemas, set OPENAPI_SCHEMA_PATH to override

EXPOSE 8000

ENTRYPOINT ["/app/hyperfleet-api"]
CMD ["serve"]

LABEL name="hyperfleet-api" \
      vendor="Red Hat" \
      version="0.0.1" \
      summary="HyperFleet API - Cluster Lifecycle Management Service" \
      description="HyperFleet API for cluster lifecycle management"
