# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

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
