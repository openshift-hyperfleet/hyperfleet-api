ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

ARG GIT_SHA=unknown
ARG GIT_DIRTY=""
ARG BUILD_DATE=""
ARG VERSION=""

# Install make as root (UBI9 go-toolset doesn't include it), then switch back to non-root.
USER root
RUN dnf install -y make && dnf clean all
WORKDIR /build
RUN chown 1001:0 /build
USER 1001

# Install bingo tools (mockgen, oapi-codegen) under /build/.gobin and add to PATH
# so "go generate" can find them. ENV persists for all subsequent RUN commands.
ENV GOBIN=/build/.gobin
ENV PATH="${GOBIN}:${PATH}"

COPY --chown=1001:0 go.mod go.sum ./
RUN --mount=type=cache,target=/opt/app-root/src/go/pkg/mod,uid=1001 \
    go mod download

COPY --chown=1001:0 . .

# CGO_ENABLED=0 produces a static binary required for distroless runtime.
# For FIPS-compliant builds (CGO_ENABLED=1 + GOEXPERIMENT=boringcrypto), use a
# runtime image with glibc (e.g. ubi9-micro) instead of distroless.
RUN --mount=type=cache,target=/opt/app-root/src/go/pkg/mod,uid=1001 \
    --mount=type=cache,target=/opt/app-root/src/.cache/go-build,uid=1001 \
    mkdir -p $GOBIN && \
    CGO_ENABLED=0 GOOS=linux \
    GIT_SHA=${GIT_SHA} GIT_DIRTY=${GIT_DIRTY} BUILD_DATE=${BUILD_DATE} VERSION=${VERSION} \
    make build

# Runtime stage
FROM ${BASE_IMAGE}

WORKDIR /app

COPY --from=builder /build/bin/hyperfleet-api /app/hyperfleet-api
COPY --from=builder /build/openapi/openapi.yaml /app/openapi/openapi.yaml

ENV OPENAPI_SCHEMA_PATH=/app/openapi/openapi.yaml

USER 65532:65532

EXPOSE 8000

ENTRYPOINT ["/app/hyperfleet-api"]
CMD ["serve"]

ARG VERSION=""
LABEL name="hyperfleet-api" \
      vendor="Red Hat" \
      version="${VERSION}" \
      summary="HyperFleet API - Cluster Lifecycle Management Service" \
      description="HyperFleet API for cluster lifecycle management"
