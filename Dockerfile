ARG BASE_IMAGE=registry.access.redhat.com/ubi9-micro:latest

FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

ARG GIT_SHA=unknown
ARG GIT_DIRTY=""
ARG BUILD_DATE=""
ARG APP_VERSION="0.0.0-dev"

# Install make as root (UBI9 go-toolset doesn't include it), then switch back to non-root.
USER root
RUN dnf install -y make && dnf clean all
WORKDIR /build
RUN chown 1001:0 /build
USER 1001

# Install bingo tools (mockgen, oapi-codegen) under /build/.gobin and add to PATH
# so "go generate" can find them. ENV persists for all subsequent RUN commands.
ENV GOBIN=/build/.gobin
RUN mkdir -p $GOBIN
ENV PATH="${GOBIN}:${PATH}"

COPY --chown=1001:0 go.mod go.sum ./
RUN --mount=type=cache,target=/opt/app-root/src/go/pkg/mod,uid=1001 \
    go mod download

COPY --chown=1001:0 . .

# CGO_ENABLED=0 produces a static binary. The default ubi9-micro runtime
# supports both static and dynamically linked binaries.
# For FIPS-compliant builds, use CGO_ENABLED=1 + GOEXPERIMENT=boringcrypto.
RUN --mount=type=cache,target=/opt/app-root/src/go/pkg/mod,uid=1001 \
    --mount=type=cache,target=/opt/app-root/src/.cache/go-build,uid=1001 \
    CGO_ENABLED=0 GOOS=linux \
    GIT_SHA=${GIT_SHA} GIT_DIRTY=${GIT_DIRTY} BUILD_DATE=${BUILD_DATE} \
    make build

# Runtime stage
FROM ${BASE_IMAGE}

WORKDIR /app

# ubi9-micro doesn't include CA certificates; copy from builder for TLS (e.g. Google Pub/Sub)
COPY --from=builder /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem
COPY --from=builder /build/bin/hyperfleet-api /app/hyperfleet-api
COPY --from=builder /build/openapi/openapi.yaml /app/openapi/openapi.yaml

ENV HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH=/app/openapi/openapi.yaml

USER 65532:65532

EXPOSE 8000

ENTRYPOINT ["/app/hyperfleet-api"]
CMD ["serve"]

ARG APP_VERSION="0.0.0-dev"
LABEL name="hyperfleet-api" \
      vendor="Red Hat" \
      version="${APP_VERSION}" \
      summary="HyperFleet API - Cluster Lifecycle Management Service" \
      description="HyperFleet API for cluster lifecycle management"
