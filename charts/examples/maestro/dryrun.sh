#!/usr/bin/env bash
set -euo pipefail

# Run the maestro chart example as a dry-run inside a container.
# The binary is built on the host (for Linux, matching host architecture) and
# mounted into a minimal container. The resource file is mounted to /etc/adapter/
# so that the task config's manifest.ref: /etc/adapter/manifestwork.yaml resolves correctly.
#
# Execute from the repository root:
#   bash charts/examples/maestro/dryrun.sh

REPO_ROOT=$(git rev-parse --show-toplevel)
EXAMPLE_DIR="charts/examples/maestro"
DRYRUN_DIR="test/testdata/dryrun"

# Map host arch to Go/Docker equivalents
case "$(uname -m)" in
  arm64|aarch64) GOARCH=arm64; PLATFORM=linux/arm64 ;;
  x86_64|amd64)  GOARCH=amd64; PLATFORM=linux/amd64 ;;
  *) echo "Unsupported architecture: $(uname -m)"; exit 1 ;;
esac

BINARY="$REPO_ROOT/bin/hyperfleet-adapter-linux-$GOARCH"

if [[ ! -f "$BINARY" ]]; then
  echo "Binary not found at bin/hyperfleet-adapter-linux-$GOARCH — building for linux/$GOARCH..."
  (cd "$REPO_ROOT" && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$BINARY" ./cmd/adapter)
fi

CONTAINER_TOOL=$(command -v podman 2>/dev/null || command -v docker 2>/dev/null)

$CONTAINER_TOOL run --rm \
  --platform "$PLATFORM" \
  -v "$BINARY":/usr/local/bin/hyperfleet-adapter:z \
  -v "$REPO_ROOT/$EXAMPLE_DIR/adapter-task-config.yaml":/etc/adapter/adapter-task-config.yaml:z \
  -v "$REPO_ROOT/$EXAMPLE_DIR/adapter-task-resource-manifestwork.yaml":/etc/adapter/manifestwork.yaml:z \
  -v "$REPO_ROOT/$EXAMPLE_DIR/dryrun-discovery.json":/example/dryrun-discovery.json:z \
  -v "$REPO_ROOT/$DRYRUN_DIR":/dryrun:z \
  alpine:3.21 \
  /usr/local/bin/hyperfleet-adapter serve \
    --dry-run-verbose \
    --config /dryrun/dryrun-maestro-adapter-config.yaml \
    --task-config /etc/adapter/adapter-task-config.yaml \
    --dry-run-event /dryrun/event.json \
    --dry-run-api-responses /dryrun/dryrun-api-responses.json \
    --dry-run-discovery /example/dryrun-discovery.json
