#!/usr/bin/env bash
# EP-6 kill test: three etcd members across kind management clusters, kill one
# MC, watch the client fail over and the replacement land on a spare MC, then
# exercise controller-free rejoin and snapshot/restore.
#
# Fabric note (PoC substitution): all kind clusters share podman's "kind"
# bridge, and a node container's IP is the node InternalIP. Per-MC Service DNS
# does not cross clusters, so members advertise https://<nodeIP>:<NodePort>
# with stable per-slot NodePorts, and member certs carry that node IP as a SAN.
# NodePort SNAT hides the peer source IP, so peer TLS skips client SAN checks.
# The server and this client run as containers attached to the same kind
# network; their transport kubeconfigs point at https://<nodeIP>:6443.
# Production keeps the design's fabric DNS plus per-MC Services.
#
# Live cert material lives in .fleet/ (gitignored); never commit it.
set -euo pipefail

if [[ ${CLM_POC_E2E:-} != 1 ]]; then
	echo "SKIP: this live kill test only runs on demand." >&2
	echo "      export CLM_POC_E2E=1 to run it." >&2
	exit 2
fi

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
CLUSTER=${CLM_POC_ETCD_CLUSTER:-etcd-kill-$(date +%Y%m%d%H%M%S)}
FLEET_MCS=${FLEET_MCS:-"etcd-mc-0 etcd-mc-1 etcd-mc-2 etcd-mc-3"}
FLEET_DIR=${FLEET_DIR:-$REPO_ROOT/.fleet}
CONTAINER_FLEET=${CONTAINER_FLEET:-$REPO_ROOT/.fleet-container}
STATE_DIR=${STATE_DIR:-$FLEET_DIR/etcd-state}
API_PORT=${CLM_POC_API_PORT:-8000}
API="http://localhost:$API_PORT/api/hyperfleet/v1"
PSQL=clm-etcd-kill-psql
SERVER=clm-etcd-kill-server
STANDIN=clm-etcd-kill-client
IMAGE=registry.access.redhat.com/ubi9/ubi-minimal
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"; podman rm -f "$STANDIN" >/dev/null 2>&1 || true' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

for cmd in go podman kubectl curl jq base64 awk; do
	command -v "$cmd" >/dev/null || fail "required command not found: $cmd"
done
export KIND_EXPERIMENTAL_PROVIDER=podman
kind() {
	go tool -modfile="$REPO_ROOT/tools/go.mod" kind "$@"
}

echo "==> Building server and client"
make -C "$REPO_ROOT" build >/dev/null
CGO_ENABLED=0 go build -o "$TMP/etcd-client-standin" ./cmd/etcd-client-standin
cd "$REPO_ROOT"

echo "==> Ensuring four management clusters (reusing existing)"
FLEET_MCS="$FLEET_MCS" FLEET_DIR="$FLEET_DIR" hack/fleet.sh up >/dev/null
mkdir -p "$CONTAINER_FLEET" "$STATE_DIR"
for mc in $FLEET_MCS; do
	ip=$(podman inspect "$mc-control-plane" \
		--format '{{(index .NetworkSettings.Networks "kind").IPAddress}}')
	sed -E "s#server: https://127\.0\.0\.1:[0-9]+#server: https://$ip:6443#" \
		"$FLEET_DIR/$mc.kubeconfig" > "$CONTAINER_FLEET/$mc.kubeconfig"
done

echo "==> Cleaning stale etcd namespaces"
ns="etcd-$CLUSTER"
for mc in $FLEET_MCS; do
	stale_names=$(kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" get ns -o name 2>/dev/null |
		grep '^namespace/etcd-' | cut -d/ -f2 || true)
	for stale in $stale_names; do
		kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" delete ns "$stale" \
			--ignore-not-found --wait=true >/dev/null 2>&1 || true
	done
done

echo "==> Starting postgres and the no-auth server"
podman rm -f "$PSQL" "$SERVER" >/dev/null 2>&1 || true
podman run -d --name "$PSQL" --network kind \
	-e POSTGRES_DB=hyperfleet -e POSTGRES_USER=hyperfleet \
	-e POSTGRES_PASSWORD=foobar-bizz-buzz docker.io/library/postgres:14.24 >/dev/null
ready=0
for _ in $(seq 1 180); do
	if podman exec "$PSQL" pg_isready -U hyperfleet >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 1
done
[[ $ready == 1 ]] || fail "postgres did not start"

run_server() {
	podman run --name "$SERVER" --network kind -p "$API_PORT:8000" \
		-v "$REPO_ROOT/bin/hyperfleet-api:/hyperfleet-api:ro,Z" \
		-v "$REPO_ROOT/configs:/configs:ro,Z" \
		-v "$CONTAINER_FLEET:/fleet:ro,Z" \
		-v "$STATE_DIR:/state:Z" \
		-v "$REPO_ROOT/openapi:/openapi:ro,Z" \
		-e HYPERFLEET_SERVER_HOST=0.0.0.0 \
		-e HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH=/openapi/openapi.yaml \
		-e HYPERFLEET_SERVER_JWT_ENABLED=false \
		-e HYPERFLEET_SERVER_TLS_ENABLED=false \
		-e HYPERFLEET_SERVER_TENANT_ENABLED=false \
		-e HYPERFLEET_SERVER_TENANT_CLAIMS_ENABLED=false \
		-e HYPERFLEET_ORCHESTRATOR_MAX_AGE=30s \
		-e HYPERFLEET_ETCD_FLEET_DIR=/fleet \
		-e HYPERFLEET_ETCD_STATE_DIR=/state \
		-e HYPERFLEET_ETCD_PREFERRED_MC=etcd-mc-0 \
		--entrypoint /hyperfleet-api -d "$IMAGE" serve --config /configs/dev.yaml \
		--db-host "$PSQL" --db-port 5432 --db-name hyperfleet \
		--db-username hyperfleet --db-password foobar-bizz-buzz >/dev/null
}

podman run --rm --network kind \
	-v "$REPO_ROOT/bin/hyperfleet-api:/hyperfleet-api:ro,Z" \
	-v "$REPO_ROOT/configs:/configs:ro,Z" \
	--entrypoint /hyperfleet-api "$IMAGE" migrate --config /configs/dev.yaml \
	--db-host "$PSQL" --db-port 5432 --db-name hyperfleet \
	--db-username hyperfleet --db-password foobar-bizz-buzz >/dev/null
rm -f "$STATE_DIR/$CLUSTER.json"
run_server
for _ in $(seq 1 60); do
	[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1" 2>/dev/null) == 200 ]] && break
	sleep 1
done
[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1") == 200 ]] ||
	fail "server did not become ready"

echo "==> Creating cluster $CLUSTER"
body=$(curl -sS -X POST "$API/clusters" -H 'Content-Type: application/json' \
	--data "{\"kind\":\"Cluster\",\"name\":\"$CLUSTER\",\"spec\":{\"topology\":\"hosted\",\"platform\":\"etcd\",\"etcd\":{\"managed\":\"clm\"}}}")
id=$(jq -r '.id // empty' <<<"$body")
[[ -n $id ]] || fail "create returned no id: $body"
formation_start=$(date +%s)

member_list() {
	local mc=$1 pod=$2
	kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" -n "$ns" exec "$pod" -c etcd -- \
		sh -c "export ETCDCTL_API=3 ETCDCTL_CACERT=/etc/etcd/tls/ca.crt ETCDCTL_CERT=/etc/etcd/tls/etcd-client.crt ETCDCTL_KEY=/etc/etcd/tls/etcd-client.key ETCDCTL_ENDPOINTS='$MEMBER_ENDPOINTS'; etcdctl member list -w table"
}

echo "==> Waiting for initial quorum formation"
for _ in $(seq 1 300); do
	data=$(curl -sS "$API/clusters/$id/statuses" 2>/dev/null |
		jq -c '[.items[]? | select(.adapter=="etcd")][0].data // {}')
	if [[ $(jq -r '.quorum // false' <<<"$data") == true &&
		$(jq -r '.membersReady // 0' <<<"$data") -ge 3 ]]; then
		break
	fi
	sleep 2
done
[[ $(jq -r '.quorum // false' <<<"$data") == true ]] || fail "quorum did not form"
[[ $(jq -r '.membersReady // 0' <<<"$data") -ge 3 ]] || fail "members did not become ready"
endpoints=$(jq -r '.endpoints | join(",")' <<<"$data")
MEMBER_ENDPOINTS=$endpoints
members=$(member_list etcd-mc-0 "$CLUSTER-etcd-m0-0")
[[ $(grep -c '| *started *|' <<<"$members" || true) -ge 3 ]] ||
	fail "initial member list is not three started members: $members"
formation_secs=$(( $(date +%s) - formation_start ))

echo "==> Extracting the SAN-less client cert for the stand-in"
mkdir -p "$TMP/certs"
for key in ca.crt etcd-client.crt etcd-client.key; do
	kubectl --kubeconfig "$FLEET_DIR/etcd-mc-1.kubeconfig" -n "$ns" \
		get secret "$CLUSTER-etcd-m1-tls" -o go-template="{{index .data \"$key\"}}" |
		base64 -d > "$TMP/certs/$key"
done
chmod -R a+rX "$TMP/certs"

echo "==> Starting the client stand-in"
podman run -d --name "$STANDIN" --network kind \
	-v "$TMP/certs:/certs:Z" -v "$TMP/etcd-client-standin:/standin:ro,Z" \
	--entrypoint /standin "$IMAGE" \
	-endpoints "$endpoints" -ca /certs/ca.crt -cert /certs/etcd-client.crt \
	-key /certs/etcd-client.key -interval 500ms >/dev/null
sleep 5
podman logs "$STANDIN" > "$TMP/client-start.log" 2>&1 || true
grep -q '^OK' "$TMP/client-start.log" || fail "client stand-in wrote nothing"

echo "==> Killing etcd-mc-0 (member 0)"
kill_ns=$(date +%s%N)
kind delete cluster --name etcd-mc-0 >/dev/null
rm -f "$FLEET_DIR/etcd-mc-0.kubeconfig" "$CONTAINER_FLEET/etcd-mc-0.kubeconfig"

echo "==> Waiting for the replacement on etcd-mc-3"
phase=""
for _ in $(seq 1 300); do
	phase=$(kubectl --kubeconfig "$FLEET_DIR/etcd-mc-3.kubeconfig" -n "$ns" \
		get pod "$CLUSTER-etcd-m0-0" -o jsonpath='{.status.phase}' 2>/dev/null || true)
	if members=$(member_list etcd-mc-1 "$CLUSTER-etcd-m1-0" 2>/dev/null); then
		started=$(grep -c '| *started *|' <<<"$members" || true)
		if [[ $started -ge 3 && $phase == Running ]] &&
			grep -q "$CLUSTER-etcd-m0" <<<"$members"; then
			break
		fi
	fi
	sleep 2
done
[[ $(grep -c '| *started *|' <<<"$members" || true) -ge 3 ]] ||
	fail "replacement did not start: $members"
[[ $phase == Running ]] || fail "replacement pod is not running on etcd-mc-3"
repair_ns=$(date +%s%N)
repair_secs=$(( (repair_ns - kill_ns) / 1000000000 ))

echo "==> Waiting for status to settle after repair"
for _ in $(seq 1 120); do
	status=$(curl -sS "$API/clusters/$id" 2>/dev/null |
		jq -r '[.status.conditions[]? | select(.type=="EtcdSuccessful")][0].reason // ""' || true)
	[[ $status == Healthy ]] && break
	sleep 2
done
[[ ${status:-} == Healthy ]] || echo "WARN: etcd status is ${status:-unknown} after repair"

podman stop "$STANDIN" >/dev/null
podman logs "$STANDIN" > "$TMP/client.log" 2>&1
read -r last_ok first_ok errors <<<"$(awk -v kill="$kill_ns" '
	$1 == "OK" { if ($2 + 0 < kill) last = $2; else if (first == 0) first = $2 }
	$1 == "ERR" { errors++ }
	END { print last + 0, first + 0, errors + 0 }' "$TMP/client.log")"
if [[ $last_ok -gt 0 && $first_ok -gt 0 ]]; then
	gap_ms=$(( (first_ok - last_ok) / 1000000 ))
else
	gap_ms=unknown
fi

echo "==> Degraded mode: CLM down, member pod and storage lost"
podman stop "$SERVER" >/dev/null
degraded_start=$(date +%s)
old_m1_id=$(grep "$CLUSTER-etcd-m1" <<<"$members" | awk -F'|' '{gsub(/ /, "", $2); print $2}')
kubectl --kubeconfig "$FLEET_DIR/etcd-mc-1.kubeconfig" -n "$ns" \
	scale statefulset "$CLUSTER-etcd-m1" --replicas=0 >/dev/null
for _ in $(seq 1 60); do
	[[ -z $(kubectl --kubeconfig "$FLEET_DIR/etcd-mc-1.kubeconfig" -n "$ns" \
		get pods -l "$(printf 'etcd.hyperfleet.io/member=m1')" -o name 2>/dev/null) ]] && break
	sleep 1
done
kubectl --kubeconfig "$FLEET_DIR/etcd-mc-1.kubeconfig" -n "$ns" \
	delete pvc "data-$CLUSTER-etcd-m1-0" --wait=true >/dev/null
kubectl --kubeconfig "$FLEET_DIR/etcd-mc-1.kubeconfig" -n "$ns" \
	scale statefulset "$CLUSTER-etcd-m1" --replicas=1 >/dev/null
phase=""
for _ in $(seq 1 180); do
	phase=$(kubectl --kubeconfig "$FLEET_DIR/etcd-mc-1.kubeconfig" -n "$ns" \
		get pod "$CLUSTER-etcd-m1-0" -o jsonpath='{.status.phase}' 2>/dev/null || true)
	if members=$(member_list etcd-mc-2 "$CLUSTER-etcd-m2-0" 2>/dev/null); then
		m1_line=$(grep "$CLUSTER-etcd-m1" <<<"$members" || true)
		if [[ $phase == Running && -n $m1_line && $m1_line != *"$old_m1_id"* ]] &&
			[[ $(grep -c '| *started *|' <<<"$members" || true) -ge 3 ]]; then
			break
		fi
	fi
	sleep 2
done
[[ $phase == Running ]] || fail "degraded-mode member pod did not start"
[[ $(grep -c '| *started *|' <<<"$members" || true) -ge 3 ]] ||
	fail "degraded-mode member did not rejoin: $members"
[[ ${m1_line:-} != *"$old_m1_id"* ]] || fail "degraded member kept its old etcd ID"
degraded_secs=$(( $(date +%s) - degraded_start ))
podman start "$SERVER" >/dev/null
for _ in $(seq 1 60); do
	[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1" 2>/dev/null) == 200 ]] && break
	sleep 1
done
for _ in $(seq 1 120); do
	status=$(curl -sS "$API/clusters/$id" 2>/dev/null |
		jq -r '[.status.conditions[]? | select(.type=="EtcdSuccessful")][0].reason // ""' || true)
	[[ $status == Healthy ]] && break
	sleep 2
done
[[ ${status:-} == Healthy ]] || fail "status did not converge after restart"

echo "==> Snapshot and restore drill"
snapshot_out=$(kubectl --kubeconfig "$FLEET_DIR/etcd-mc-2.kubeconfig" -n "$ns" \
	exec "$CLUSTER-etcd-m2-0" -c etcd -- sh -c '
	set -e
	export ETCDCTL_API=3 ETCDCTL_CACERT=/etc/etcd/tls/ca.crt ETCDCTL_CERT=/etc/etcd/tls/etcd-client.crt ETCDCTL_KEY=/etc/etcd/tls/etcd-client.key ETCDCTL_ENDPOINTS=https://'"$CLUSTER"'-etcd-m2.etcd-'"$CLUSTER"'.svc:2379
	rm -rf /tmp/ep6-snap.db /tmp/ep6-restore
	etcdctl snapshot save /tmp/ep6-snap.db >/dev/null 2>&1
	etcdctl snapshot status /tmp/ep6-snap.db -w table
	start=$(date +%s%N)
	etcdctl snapshot restore /tmp/ep6-snap.db --data-dir /tmp/ep6-restore \
		--bump-revision=1 --mark-compacted \
		--name '"$CLUSTER"'-etcd-m2 \
		--initial-cluster '"$CLUSTER"'-etcd-m2=http://127.0.0.1:12380 \
		--initial-advertise-peer-urls http://127.0.0.1:12380 >/dev/null 2>&1
	end=$(date +%s%N)
	echo "RESTORE_MS=$(( (end - start) / 1000000 ))"
	if etcdctl snapshot restore /tmp/ep6-snap.db --data-dir /tmp/ep6-restore \
		--bump-revision=1 --mark-compacted \
		--name '"$CLUSTER"'-etcd-m2 \
		--initial-cluster '"$CLUSTER"'-etcd-m2=http://127.0.0.1:12380 \
		--initial-advertise-peer-urls http://127.0.0.1:12380 >/dev/null 2>&1; then
		echo "NONEMPTY_GUARD=failed"
	else
		echo "NONEMPTY_GUARD=ok"
	fi
	unset ETCDCTL_CACERT ETCDCTL_CERT ETCDCTL_KEY ETCDCTL_ENDPOINTS
	etcd --data-dir /tmp/ep6-restore --name '"$CLUSTER"'-etcd-m2 --force-new-cluster \
		--listen-client-urls http://127.0.0.1:12379 \
		--advertise-client-urls http://127.0.0.1:12379 \
		--listen-peer-urls http://127.0.0.1:12380 \
		--initial-advertise-peer-urls http://127.0.0.1:12380 \
		--initial-cluster '"$CLUSTER"'-etcd-m2=http://127.0.0.1:12380 \
		>/tmp/ep6-restored-etcd.log 2>&1 &
	pid=$!
	for _ in $(seq 1 60); do
		if etcdctl --endpoints http://127.0.0.1:12379 get --prefix standin \
			>/tmp/ep6-keys.txt 2>/dev/null && [ -s /tmp/ep6-keys.txt ]; then
			break
		fi
		sleep 0.5
	done
	echo "RESTORED_KEYS=$(grep -c . /tmp/ep6-keys.txt || true)"
	kill "$pid" 2>/dev/null || true
	wait "$pid" 2>/dev/null || true
' 2>&1)
restore_ms=$(grep -o 'RESTORE_MS=[0-9]*' <<<"$snapshot_out" | cut -d= -f2)
restored_keys=$(grep -o 'RESTORED_KEYS=[0-9]*' <<<"$snapshot_out" | cut -d= -f2)
[[ $(grep -o 'NONEMPTY_GUARD=ok' <<<"$snapshot_out") == NONEMPTY_GUARD=ok ]] ||
	fail "restore did not refuse a non-empty data dir"
[[ ${restored_keys:-0} -gt 0 ]] || fail "restored data had no keys"

summary="$FLEET_DIR/kill-test-$CLUSTER.summary"
{
	echo "cluster=$CLUSTER"
	echo "fabric=kind/podman shared bridge, node IP + NodePort (peer SAN check skipped)"
	echo "initial_formation_secs=$formation_secs"
	echo "failover_gap_ms=$gap_ms"
	echo "client_errors_total=$errors"
	echo "repair_rto_secs=$repair_secs"
	echo "degraded_rejoin_secs=$degraded_secs"
	echo "restore_ms=$restore_ms"
	echo "restored_keys=$restored_keys"
} | tee "$summary"

echo
echo "PASS: kill test, degraded mode, and restore drill completed."
echo "Snapshot is small and fast at this scale; a 5-minute RPO cadence is feasible."
