#!/usr/bin/env bash
# DM-2 demo: a real OpenShift control plane served by a CLM-minted etcd
# quorum spread across the kind fleet, then the same quorum surviving the
# loss of a management cluster while the control plane stays up.
#
# Fabric note (PoC substitution): all kind clusters share podman's "kind"
# bridge, so members advertise https://<nodeIP>:<NodePort> with fixed
# per-slot NodePorts. The coreshift container runs on its own "coreshift"
# bridge and is attached to the kind bridge so the apiserver can reach
# those node IPs. Cert material lives in a temp dir; the pull secret is
# only ever handed to coreshift by path.
#
# Re-runnable: deletes stale etcd cluster records and containers first, and
# recreates mc-0 if a prior run killed it. The kill/repair phase itself must
# run to completion once started.
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
CLUSTER=${CLM_POC_DEMO_CLUSTER:-demo}
CP_NAME=${CLM_POC_DEMO_CP_NAME:-demo-cp}
CP_CONTAINER=coreshift-$CP_NAME
CP_PORT=${CLM_POC_DEMO_CP_PORT:-6443}
FLEET_MCS=${FLEET_MCS:-"mc-0 mc-1 mc-2 mc-3"}
FLEET_DIR=${FLEET_DIR:-$REPO_ROOT/.fleet}
CONTAINER_FLEET=${CONTAINER_FLEET:-$REPO_ROOT/.fleet-container}
STATE_DIR=${STATE_DIR:-$FLEET_DIR/etcd-state}
API_PORT=${CLM_POC_API_PORT:-8000}
API="http://localhost:$API_PORT/api/hyperfleet/v1"
PSQL=${CLM_POC_DEMO_PSQL:-clm-poc-etcd-psql}
SERVER=${CLM_POC_DEMO_SERVER:-clm-poc-etcd-server}
IMAGE=registry.access.redhat.com/ubi9/ubi-minimal
CORESHIFT_REPO=${CORESHIFT_REPO:-$HOME/Projects/vkareh/coreshift}
FORMATION_TIMEOUT=${CLM_POC_FORMATION_TIMEOUT:-600}
CP_TIMEOUT=${CLM_POC_CP_TIMEOUT:-1800}
DELETE_TIMEOUT=${CLM_POC_DELETE_TIMEOUT:-600}
NS="etcd-$CLUSTER"
MEMBER_ENDPOINTS=""

source "$REPO_ROOT/hack/takeover-api-port.sh"

TMP=$(mktemp -d)
CP_LOG=$TMP/coreshift-create.log
KUBECTL_LOG=$TMP/kubectl-loop.log
loop_pid=""
cp_pid=""

stop_loop() {
	if [[ -n $loop_pid ]] && kill -0 "$loop_pid" 2>/dev/null; then
		kill "$loop_pid" 2>/dev/null || true
		wait "$loop_pid" 2>/dev/null || true
	fi
}
trap 'stop_loop; kill "${cp_pid:-}" >/dev/null 2>&1 || true; rm -rf "$TMP"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

for cmd in go podman kubectl curl jq base64 awk make; do
	command -v "$cmd" >/dev/null || fail "required command not found: $cmd"
done
[[ -n ${HYPERFLEET_CORESHIFT_PULL_SECRET_FILE:-} ]] ||
	fail "HYPERFLEET_CORESHIFT_PULL_SECRET_FILE is not set; export the pull-secret path"
[[ -r ${HYPERFLEET_CORESHIFT_PULL_SECRET_FILE:-} ]] ||
	fail "pull secret is not readable at HYPERFLEET_CORESHIFT_PULL_SECRET_FILE"

export KIND_EXPERIMENTAL_PROVIDER=podman
kind() {
	go tool -modfile="$REPO_ROOT/tools/go.mod" kind "$@"
}

http() {
	local method=$1 path=$2 data=${3:-}
	local args=(-sS --max-time 30 -o "$TMP/body" -w '%{http_code}'
		-X "$method" "$API$path")
	if [[ -n $data ]]; then
		args+=(-H 'Content-Type: application/json' --data "$data")
	fi
	curl "${args[@]}"
}

wait_for_deletion() {
	local id=$1 start=$SECONDS code
	while (( SECONDS - start < DELETE_TIMEOUT )); do
		code=$(http GET "/clusters/$id")
		[[ $code == 404 ]] && return 0
		sleep 2
	done
	return 1
}

member_list() {
	local mc=$1 pod=$2
	kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" -n "$NS" exec "$pod" -c etcd -- \
		sh -c "export ETCDCTL_API=3 ETCDCTL_CACERT=/etc/etcd/tls/ca.crt ETCDCTL_CERT=/etc/etcd/tls/etcd-client.crt ETCDCTL_KEY=/etc/etcd/tls/etcd-client.key ETCDCTL_ENDPOINTS='$MEMBER_ENDPOINTS'; etcdctl member list -w table"
}

echo "==> Building the API server and locating coreshift"
make -C "$REPO_ROOT" build >/dev/null
cd "$REPO_ROOT"
if [[ -n ${CORESHIFT_BIN:-} ]]; then
	[[ -x $CORESHIFT_BIN ]] || fail "CORESHIFT_BIN is not executable: $CORESHIFT_BIN"
else
	make -C "$CORESHIFT_REPO" build >/dev/null
	CORESHIFT_BIN=$CORESHIFT_REPO/coreshift
fi
"$CORESHIFT_BIN" create cluster --help >"$TMP/coreshift-help" 2>&1 || true
grep -q -- '--external-etcd-endpoints' "$TMP/coreshift-help" ||
	fail "coreshift at $CORESHIFT_BIN has no external-etcd flags; rebuild it"
"$CORESHIFT_BIN" delete cluster --name "$CP_NAME" >/dev/null 2>&1 || true
podman rm -f "$CP_CONTAINER" >/dev/null 2>&1 || true

echo "==> Ensuring four management clusters (reusing existing)"
FLEET_MCS="$FLEET_MCS" FLEET_DIR="$FLEET_DIR" hack/fleet.sh up >/dev/null
mkdir -p "$CONTAINER_FLEET" "$STATE_DIR"
for mc in $FLEET_MCS; do
	ip=$(podman inspect "$mc-control-plane" \
		--format '{{(index .NetworkSettings.Networks "kind").IPAddress}}')
	sed -E "s#server: https://127\.0\.0\.1:[0-9]+#server: https://$ip:6443#" \
		"$FLEET_DIR/$mc.kubeconfig" > "$CONTAINER_FLEET/$mc.kubeconfig"
done

echo "==> Starting postgres and the no-auth server on the kind network"
if podman container exists "$PSQL" >/dev/null 2>&1; then
	podman start "$PSQL" >/dev/null 2>&1 || true
else
	podman run -d --name "$PSQL" --network kind \
		-e POSTGRES_DB=hyperfleet -e POSTGRES_USER=hyperfleet \
		-e POSTGRES_PASSWORD=foobar-bizz-buzz docker.io/library/postgres:14.24 >/dev/null
fi
ready=0
for _ in $(seq 1 180); do
	if podman exec "$PSQL" pg_isready -U hyperfleet >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 1
done
[[ $ready == 1 ]] || fail "postgres did not start"

podman rm -f "$SERVER" >/dev/null 2>&1 || true
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
		-e HYPERFLEET_ETCD_PREFERRED_MC=mc-0 \
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
takeover_api_port
run_server
for _ in $(seq 1 60); do
	[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1" 2>/dev/null) == 200 ]] && break
	sleep 1
done
[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1") == 200 ]] ||
	fail "server did not become ready"

echo "==> Deleting any existing hosted/etcd clusters"
code=$(http GET "/clusters?size=100")
[[ $code == 200 ]] || fail "list clusters returned HTTP $code: $(<"$TMP/body")"
mapfile -t ids < <(jq -r '.items[]? | select((.spec.platform // "") == "etcd") | .id' "$TMP/body")
if [[ ${#ids[@]} -eq 0 ]]; then
	echo "    none"
else
	for id in "${ids[@]}"; do
		echo "    finalizing $id"
		code=$(http DELETE "/clusters/$id")
		[[ $code == 202 || $code == 404 ]] ||
			fail "delete cluster $id returned HTTP $code: $(<"$TMP/body")"
		wait_for_deletion "$id" ||
			fail "cluster $id did not finish deleting within ${DELETE_TIMEOUT}s"
	done
fi

echo "==> Clearing stale etcd namespaces and state"
for mc in $FLEET_MCS; do
	kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" delete ns "$NS" \
		--ignore-not-found --wait=true >/dev/null 2>&1 || true
done
rm -f "$STATE_DIR/$CLUSTER.json"

echo "==> Minting cluster $CLUSTER"
body=$(curl -sS -X POST "$API/clusters" -H 'Content-Type: application/json' \
	--data "{\"kind\":\"Cluster\",\"name\":\"$CLUSTER\",\"spec\":{\"topology\":\"hosted\",\"platform\":\"etcd\",\"etcd\":{\"managed\":\"clm\"}}}")
id=$(jq -r '.id // empty' <<<"$body")
[[ -n $id ]] || fail "create returned no id: $body"
formation_start=$SECONDS

echo "==> Waiting for quorum formation"
data='{}'
while (( SECONDS - formation_start < FORMATION_TIMEOUT )); do
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
[[ -n $endpoints ]] || fail "status has no etcd endpoints"
MEMBER_ENDPOINTS=$endpoints
formation_secs=$(( SECONDS - formation_start ))
echo "    quorum ready in ${formation_secs}s: $endpoints"

echo "==> Extracting client material from a member secret"
mkdir -p "$TMP/certs"
for key in ca.crt etcd-client.crt etcd-client.key; do
	kubectl --kubeconfig "$FLEET_DIR/mc-1.kubeconfig" -n "$NS" \
		get secret "$CLUSTER-etcd-m1-tls" -o go-template="{{index .data \"$key\"}}" |
		base64 -d > "$TMP/certs/$key"
done
chmod -R a+rX "$TMP/certs"

echo "==> Creating the coreshift control plane on the fleet etcd"
"$CORESHIFT_BIN" create cluster --name "$CP_NAME" --port "$CP_PORT" \
	--pull-secret "$HYPERFLEET_CORESHIFT_PULL_SECRET_FILE" \
	--external-etcd-endpoints="$endpoints" \
	--external-etcd-ca="$TMP/certs/ca.crt" \
	--external-etcd-cert="$TMP/certs/etcd-client.crt" \
	--external-etcd-key="$TMP/certs/etcd-client.key" >"$CP_LOG" 2>&1 &
cp_pid=$!
create_start=$SECONDS

attached=0
for _ in $(seq 1 240); do
	if podman container exists "$CP_CONTAINER" >/dev/null 2>&1; then
		attached=1
		break
	fi
	kill -0 "$cp_pid" 2>/dev/null || break
	sleep 1
done
[[ $attached == 1 ]] || { tail -n 20 "$CP_LOG" >&2; fail "coreshift did not start $CP_CONTAINER"; }
connected=0
for _ in $(seq 1 10); do
	if podman network connect kind "$CP_CONTAINER" >/dev/null 2>&1; then
		connected=1
		break
	fi
	sleep 1
done
[[ $connected == 1 ]] || fail "could not attach $CP_CONTAINER to the kind network"
echo "    attached $CP_CONTAINER to the kind network"

while kill -0 "$cp_pid" 2>/dev/null; do
	if (( SECONDS - create_start > CP_TIMEOUT )); then
		kill "$cp_pid" 2>/dev/null || true
		fail "coreshift did not become ready within ${CP_TIMEOUT}s"
	fi
	sleep 1
done
if ! wait "$cp_pid"; then
	tail -n 20 "$CP_LOG" >&2
	fail "coreshift create failed (log: $CP_LOG)"
fi
cp_pid=""
cp_ready_secs=$(( SECONDS - create_start ))
echo "    control plane ready in ${cp_ready_secs}s"

echo
echo "*** A real OpenShift API server is now serving off the fleet etcd ***"
kubeconfig=$("$CORESHIFT_BIN" get kubeconfig --name "$CP_NAME")
[[ -r $kubeconfig ]] || fail "coreshift did not provide a kubeconfig"
ns_count=$(kubectl --kubeconfig "$kubeconfig" get ns --no-headers | wc -l | tr -d ' ')
echo "    kubectl get ns -> $ns_count namespaces"
kubectl --kubeconfig "$kubeconfig" get ns | sed 's/^/    /'
echo

echo "==> Killing mc-0 (member 0) while kubectl keeps polling"
(
	while true; do
		if kubectl --kubeconfig "$kubeconfig" get ns --request-timeout=5s \
			>/dev/null 2>>"$TMP/kubectl-errors.log"; then
			printf 'OK %s\n' "$(date +%s%N)"
		else
			printf 'ERR %s\n' "$(date +%s%N)"
		fi
		sleep 1
	done
) >>"$KUBECTL_LOG" 2>&1 &
loop_pid=$!
for _ in $(seq 1 30); do
	grep -q '^OK' "$KUBECTL_LOG" 2>/dev/null && break
	sleep 1
done
grep -q '^OK' "$KUBECTL_LOG" 2>/dev/null ||
	fail "kubectl was not healthy before the kill"

kill_ns=$(date +%s%N)
kind delete cluster --name mc-0 >/dev/null
rm -f "$FLEET_DIR/mc-0.kubeconfig" "$CONTAINER_FLEET/mc-0.kubeconfig"

echo "==> Waiting for the replacement on mc-3"
phase=""
members=""
for _ in $(seq 1 300); do
	phase=$(kubectl --kubeconfig "$FLEET_DIR/mc-3.kubeconfig" -n "$NS" \
		get pod "$CLUSTER-etcd-m0-0" -o jsonpath='{.status.phase}' 2>/dev/null || true)
	if members=$(member_list mc-1 "$CLUSTER-etcd-m1-0" 2>/dev/null); then
		started=$(grep -c '| *started *|' <<<"$members" || true)
		if [[ $started -ge 3 && $phase == Running ]] &&
			grep -q "$CLUSTER-etcd-m0" <<<"$members"; then
			break
		fi
	fi
	sleep 2
done
if [[ $(grep -c '| *started *|' <<<"$members" || true) -lt 3 ]]; then
	if [[ $phase == Running ]]; then
		echo "    replacement reset log:" >&2
		kubectl --kubeconfig "$FLEET_DIR/mc-3.kubeconfig" -n "$NS" \
			logs "$CLUSTER-etcd-m0-0" -c reset-member 2>&1 | tail -n 3 >&2 || true
	fi
	fail "replacement did not join the etcd cluster: $members"
fi
[[ $phase == Running ]] || fail "replacement pod is not running on mc-3"
repair_end=$(date +%s%N)
repair_secs=$(( (repair_end - kill_ns) / 1000000000 ))
sleep 3

stop_loop
read -r last_ok first_ok errors <<<"$(awk -v kill="$kill_ns" '
	$1 == "OK" { if ($2 + 0 < kill) last = $2; else if (first == 0) first = $2 }
	$1 == "ERR" { errors++ }
	END { print last + 0, first + 0, errors + 0 }' "$KUBECTL_LOG")"
if [[ $last_ok -gt 0 && $first_ok -gt 0 ]]; then
	gap_ms=$(( (first_ok - last_ok) / 1000000 ))
else
	gap_ms=unknown
fi
[[ $gap_ms != unknown ]] || fail "kubectl never recovered after the kill"

echo "==> Tearing down the coreshift control plane"
"$CORESHIFT_BIN" delete cluster --name "$CP_NAME" >/dev/null

echo "==> Deleting the CLM cluster record"
teardown_start=$SECONDS
code=$(http DELETE "/clusters/$id")
[[ $code == 202 || $code == 404 ]] ||
	fail "delete returned HTTP $code: $(<"$TMP/body")"
wait_for_deletion "$id" ||
	fail "cluster did not finish deleting within ${DELETE_TIMEOUT}s"
teardown_secs=$(( SECONDS - teardown_start ))

echo
echo "PASS: a real OpenShift API server ran on a fleet-spread etcd quorum."
printf '%-18s %ss\n' 'formation:' "$formation_secs"
printf '%-18s %ss\n' 'coreshift ready:' "$cp_ready_secs"
printf '%-18s %sms\n' 'kill gap:' "$gap_ms"
printf '%-18s %ss\n' 'repair:' "$repair_secs"
printf '%-18s %ss\n' 'teardown:' "$teardown_secs"
printf '%-18s %s\n' 'kubectl errors:' "$errors"
