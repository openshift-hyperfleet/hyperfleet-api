#!/usr/bin/env bash
# Dev-fleet harness: manages the three local kind management clusters
# (etcd-mc-0, etcd-mc-1, etcd-mc-2) that the etcd-manager PoC runs
# against, exporting their kubeconfigs into the fleet directory (one
# <name>.kubeconfig file per cluster).
#
# Usage: hack/fleet.sh {up|down|status}
#
# kind runs on podman and is resolved through the pinned tools module
# (tools/go.mod); it is never installed or downloaded separately.
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
FLEET_DIR=${FLEET_DIR:-$REPO_ROOT/.fleet}
KIND_CONFIG=$REPO_ROOT/hack/kind-config.yaml
read -r -a MCS <<< "${FLEET_MCS:-etcd-mc-0 etcd-mc-1 etcd-mc-2}"

cd "$REPO_ROOT"
export KIND_EXPERIMENTAL_PROVIDER=podman

kind() {
    go tool -modfile=tools/go.mod kind "$@"
}

# kind v0.32 cannot list clusters through podman 6, so query podman directly.
cluster_exists() {
    podman ps -a --filter "label=io.x-k8s.kind.cluster=$1" --format '{{.Names}}' | grep -q .
}

cmd_up() {
    mkdir -p "$FLEET_DIR"
    local name
    for name in "${MCS[@]}"; do
        if cluster_exists "$name"; then
            echo "$name: cluster already exists"
        else
            echo "$name: creating cluster"
            kind create cluster --name "$name" --config "$KIND_CONFIG" --kubeconfig "$FLEET_DIR/$name.kubeconfig"
        fi
        if [[ ! -s "$FLEET_DIR/$name.kubeconfig" ]]; then
            kind get kubeconfig --name "$name" > "$FLEET_DIR/$name.kubeconfig"
        fi
        # Nested control planes exceed kind's default 2048 pids cap.
        podman update --pids-limit 8192 "$name-control-plane"
    done
    echo "fleet directory: $FLEET_DIR"
}

cmd_down() {
    local name
    for name in "${MCS[@]}"; do
        if cluster_exists "$name"; then
            echo "$name: deleting cluster"
            kind delete cluster --name "$name"
        else
            echo "$name: no cluster, skipping"
        fi
        rm -f "$FLEET_DIR/$name.kubeconfig"
    done
}

cmd_status() {
    local name state kubeconfig
    for name in "${MCS[@]}"; do
        state=absent
        kubeconfig=missing
        if cluster_exists "$name"; then
            state=up
        fi
        if [[ -f "$FLEET_DIR/$name.kubeconfig" ]]; then
            kubeconfig=present
        fi
        printf '%s\tcluster=%s\tkubeconfig=%s\n' "$name" "$state" "$kubeconfig"
    done
}

for bin in go podman; do
    if ! command -v "$bin" >/dev/null 2>&1; then
        echo "error: $bin is required but not on PATH" >&2
        exit 1
    fi
done

case "${1:-}" in
    up) cmd_up ;;
    down) cmd_down ;;
    status) cmd_status ;;
    *)
        echo "usage: $0 {up|down|status}" >&2
        exit 2
        ;;
esac
