#!/usr/bin/env bash
# DM-5 demo: the hosted model, live. One cluster object on the hosted/hcp
# chain runs a fleet-spread etcd quorum plus a real OpenShift control plane
# in a pod next to etcd member 0. Killing that management cluster takes out
# both workloads; the quorum holds on the survivors, the member is replaced
# on the spare, and the control plane re-homes cold. Its data lives in the
# fleet etcd, so kubectl resumes with everything intact.
#
# Fabric note (PoC substitution): all kind clusters share podman's "kind"
# bridge, so etcd members advertise https://<nodeIP>:<NodePort> and the
# nested pod reaches them over that bridge. The coreshift image is minted
# locally and loaded into every management cluster because a re-home can
# land on any survivor. The pull secret is handed to coreshift by path and
# never enters the repo, logs, or status payloads.
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
CLUSTER=${CLM_POC_HCP_CLUSTER:-demo}
FLEET_MCS=${FLEET_MCS:-"etcd-mc-0 etcd-mc-1 etcd-mc-2 etcd-mc-3"}
FLEET_DIR=${FLEET_DIR:-$REPO_ROOT/.fleet}
CONTAINER_FLEET=${CONTAINER_FLEET:-$REPO_ROOT/.fleet-container}
STATE_DIR=${STATE_DIR:-$FLEET_DIR/etcd-state}
API_PORT=${CLM_POC_API_PORT:-8000}
API="http://localhost:$API_PORT/api/hyperfleet/v1"
PSQL=${CLM_POC_HCP_PSQL:-clm-poc-hcp-psql}
SERVER=${CLM_POC_HCP_SERVER:-clm-poc-hcp-server}
IMAGE=registry.access.redhat.com/ubi9/ubi-minimal
CORESHIFT_REPO=${CORESHIFT_REPO:-$HOME/Projects/vkareh/coreshift}
MINT_PORT=${CLM_POC_HCP_MINT_PORT:-16443}
API_FORWARD_PORT=6443
NS="hcp-$CLUSTER"
ETCD_NS="etcd-$CLUSTER"
DATA_NAMESPACE=${CLM_POC_HCP_DATA_NS:-demo-data}
DATA_CONFIGMAP=demo-proof
DATA_MESSAGE=${CLM_POC_HCP_DATA_MESSAGE:-data-survived-the-kill}
FORMATION_TIMEOUT=${CLM_POC_FORMATION_TIMEOUT:-600}
CP_TIMEOUT=${CLM_POC_CP_TIMEOUT:-1800}
REHOME_TIMEOUT=${CLM_POC_REHOME_TIMEOUT:-1800}
DELETE_TIMEOUT=${CLM_POC_DELETE_TIMEOUT:-900}

TMP=$(mktemp -d)
MINT_STATE=$TMP/mint-state
MINT_NAME=clm-mint-$$
MINT_PID=""
FWD_PID=""
ADMIN_KUBECONFIG=$TMP/admin.kubeconfig

stop_forward() {
	if [[ -n $FWD_PID ]] && kill -0 "$FWD_PID" 2>/dev/null; then
		kill "$FWD_PID" 2>/dev/null || true
		wait "$FWD_PID" 2>/dev/null || true
	fi
	FWD_PID=""
}

cleanup() {
	stop_forward
	if [[ -n $MINT_PID ]] && kill -0 "$MINT_PID" 2>/dev/null; then
		kill "$MINT_PID" 2>/dev/null || true
		wait "$MINT_PID" 2>/dev/null || true
	fi
	if podman container exists "coreshift-$MINT_NAME" >/dev/null 2>&1; then
		XDG_STATE_HOME="$MINT_STATE" "${CORESHIFT_BIN:-coreshift}" delete cluster \
			--name "$MINT_NAME" >/dev/null 2>&1 ||
			podman rm -f "coreshift-$MINT_NAME" >/dev/null 2>&1 || true
	fi
	rm -rf "$TMP"
}
trap cleanup EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

for cmd in go podman kubectl oc curl jq base64 awk make; do
	command -v "$cmd" >/dev/null || fail "required command not found: $cmd"
done
PULL_SECRET=${HYPERFLEET_CORESHIFT_PULL_SECRET_FILE:-}
[[ -n $PULL_SECRET ]] ||
	fail "HYPERFLEET_CORESHIFT_PULL_SECRET_FILE is not set; export the pull-secret path"
[[ -r $PULL_SECRET ]] ||
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

status_data() {
	local id=$1 adapter=$2
	curl -sS "$API/clusters/$id/statuses" 2>/dev/null |
		jq -c --arg a "$adapter" '[.items[]? | select(.adapter==$a)][0].data // {}'
}

adapter_reason() {
	local id=$1 condition=$2
	curl -sS "$API/clusters/$id" 2>/dev/null |
		jq -r --arg c "$condition" \
			'[.status.conditions[]? | select(.type==$c)][0].reason // ""'
}

etcd_member_mc() {
	local data=$1 name=$2
	jq -r --arg n "$name" '.members[]? | select(.name==$n) | .mc' <<<"$data"
}

etcd_member_healthy() {
	local data=$1 name=$2
	jq -r --arg n "$name" \
		'.members[]? | select(.name==$n) | (.healthy // false)' <<<"$data"
}

start_forward() {
	local mc=$1 pod=$2
	stop_forward
	: >"$TMP/forward.log"
	kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" -n "$NS" \
		port-forward "pod/$pod" "$API_FORWARD_PORT:6443" >"$TMP/forward.log" 2>&1 &
	FWD_PID=$!
	for _ in $(seq 1 20); do
		if kubectl --kubeconfig "$ADMIN_KUBECONFIG" get ns --request-timeout=3s \
			>/dev/null 2>&1; then
			return 0
		fi
		kill -0 "$FWD_PID" 2>/dev/null || break
		sleep 1
	done
	return 1
}

namespace_count() {
	oc --kubeconfig "$ADMIN_KUBECONFIG" get ns --no-headers | wc -l | tr -d ' '
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

echo "==> Ensuring four management clusters (reusing existing)"
FLEET_MCS="$FLEET_MCS" FLEET_DIR="$FLEET_DIR" hack/fleet.sh up >/dev/null
mkdir -p "$CONTAINER_FLEET" "$STATE_DIR"
for mc in $FLEET_MCS; do
	ip=$(podman inspect "$mc-control-plane" \
		--format '{{(index .NetworkSettings.Networks "kind").IPAddress}}')
	sed -E "s#server: https://127\.0\.0\.1:[0-9]+#server: https://$ip:6443#" \
		"$FLEET_DIR/$mc.kubeconfig" > "$CONTAINER_FLEET/$mc.kubeconfig"
done

echo "==> Minting the coreshift image (throwaway create, state discarded)"
mkdir -p "$MINT_STATE"
mint_log=$TMP/mint.log
XDG_STATE_HOME="$MINT_STATE" "$CORESHIFT_BIN" create cluster \
	--name "$MINT_NAME" --port "$MINT_PORT" \
	--pull-secret "$PULL_SECRET" >"$mint_log" 2>&1 &
MINT_PID=$!
mint_container=coreshift-$MINT_NAME
mint_start=$SECONDS
until podman container exists "$mint_container" >/dev/null 2>&1; do
	if ! kill -0 "$MINT_PID" 2>/dev/null; then
		tail -n 20 "$mint_log" >&2
		fail "coreshift create failed before minting the image"
	fi
	if (( SECONDS - mint_start > 600 )); then
		tail -n 20 "$mint_log" >&2
		fail "timed out minting the coreshift image"
	fi
	sleep 1
done
image=$(podman inspect "$mint_container" --format '{{.Config.Image}}')
kill "$MINT_PID" 2>/dev/null || true
wait "$MINT_PID" 2>/dev/null || true
MINT_PID=""
XDG_STATE_HOME="$MINT_STATE" "$CORESHIFT_BIN" delete cluster \
	--name "$MINT_NAME" >/dev/null 2>&1 || true
[[ $image == localhost/coreshift:* ]] || fail "unexpected coreshift image ref: $image"
podman image exists "$image" || fail "minted image is not in the podman store: $image"
echo "    image: $image"

echo "==> Loading the image into all four management clusters"
for mc in $FLEET_MCS; do
	kind load docker-image "$image" --name "$mc" >/dev/null
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
		-v "$PULL_SECRET:/pull-secret:ro,Z" \
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
		-e HYPERFLEET_HCP_IMAGE="$image" \
		-e HYPERFLEET_CORESHIFT_PULL_SECRET_FILE=/pull-secret \
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
run_server
for _ in $(seq 1 60); do
	[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1" 2>/dev/null) == 200 ]] && break
	sleep 1
done
[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1") == 200 ]] ||
	fail "server did not become ready"

echo "==> Deleting any existing hosted clusters"
code=$(http GET "/clusters?size=100")
[[ $code == 200 ]] || fail "list clusters returned HTTP $code: $(<"$TMP/body")"
mapfile -t ids < <(jq -r '.items[]? | select((.spec.topology // "") == "hosted") | .id' "$TMP/body")
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

echo "==> Clearing stale namespaces and state"
for mc in $FLEET_MCS; do
	kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" delete ns "$NS" "$ETCD_NS" \
		--ignore-not-found --wait=true >/dev/null 2>&1 || true
done
rm -f "$STATE_DIR/$CLUSTER.json"

echo "==> Minting cluster $CLUSTER on hosted/hcp"
body=$(curl -sS -X POST "$API/clusters" -H 'Content-Type: application/json' \
	--data "{\"kind\":\"Cluster\",\"name\":\"$CLUSTER\",\"spec\":{\"topology\":\"hosted\",\"platform\":\"hcp\",\"etcd\":{\"managed\":\"clm\"}}}")
id=$(jq -r '.id // empty' <<<"$body")
[[ -n $id ]] || fail "create returned no id: $body"
mint_cluster_start=$SECONDS

echo "==> Waiting for etcd quorum formation"
etcd_data='{}'
while (( SECONDS - mint_cluster_start < FORMATION_TIMEOUT )); do
	etcd_data=$(status_data "$id" etcd)
	if [[ $(jq -r '.quorum // false' <<<"$etcd_data") == true &&
		$(jq -r '.membersReady // 0' <<<"$etcd_data") -ge 3 ]]; then
		break
	fi
	sleep 2
done
[[ $(jq -r '.quorum // false' <<<"$etcd_data") == true ]] || fail "quorum did not form"
[[ $(jq -r '.membersReady // 0' <<<"$etcd_data") -ge 3 ]] ||
	fail "members did not become ready"
formation_secs=$(( SECONDS - mint_cluster_start ))
member0="$CLUSTER-etcd-m0"
member0_mc=$(etcd_member_mc "$etcd_data" "$member0")
echo "    quorum ready in ${formation_secs}s; $member0 is on $member0_mc"

echo "==> Waiting for the control plane pod next to member 0"
hcp_data='{}'
hcp_mc=""
for _ in $(seq 1 120); do
	hcp_data=$(status_data "$id" hcp)
	hcp_mc=$(jq -r '.mc // ""' <<<"$hcp_data")
	[[ $hcp_mc == "$member0_mc" ]] && break
	sleep 2
done
[[ $hcp_mc == "$member0_mc" ]] ||
	fail "hcp adapter placed the control plane on ${hcp_mc:-nothing}, wanted $member0_mc"
echo "    control plane scheduled on $hcp_mc, co-located with $member0"

echo "==> Booting the hosted control plane (payload pull happens here)"
for _ in $(seq 1 $(( CP_TIMEOUT / 5 ))); do
	hcp_data=$(status_data "$id" hcp)
	[[ $(jq -r '.ready // false' <<<"$hcp_data") == true ]] && break
	sleep 5
done
[[ $(jq -r '.ready // false' <<<"$hcp_data") == true ]] ||
	fail "control plane did not become ready within ${CP_TIMEOUT}s"
status_ready_secs=$(( SECONDS - mint_cluster_start ))
echo "    kubeconfig available in ${status_ready_secs}s; waiting for the API to answer"

echo "==> Attaching to the control plane and proving data"
served=0
for attempt in $(seq 1 $(( CP_TIMEOUT / 5 ))); do
	hcp_data=$(status_data "$id" hcp)
	hcp_mc=$(jq -r '.mc // ""' <<<"$hcp_data")
	hcp_pod=$(jq -r '.pod // ""' <<<"$hcp_data")
	hcp_kubeconfig=$(jq -r '.kubeconfigRef // ""' <<<"$hcp_data")
	if [[ -n $hcp_mc && -n $hcp_pod && -n $hcp_kubeconfig &&
		$(jq -r '.ready // false' <<<"$hcp_data") == true ]]; then
		printf '%s\n' "$hcp_kubeconfig" > "$ADMIN_KUBECONFIG"
		chmod 600 "$ADMIN_KUBECONFIG"
		if start_forward "$hcp_mc" "$hcp_pod" &&
			oc --kubeconfig "$ADMIN_KUBECONFIG" get ns >/dev/null 2>&1; then
			served=1
			break
		fi
	fi
	stop_forward
	sleep 5
done
[[ $served == 1 ]] || fail "control plane API never answered within ${CP_TIMEOUT}s"
cp_first_serve_secs=$(( SECONDS - mint_cluster_start ))
echo "    API answered in ${cp_first_serve_secs}s (pod $hcp_pod); waiting for the boot to settle"

last_count=""
steady=0
for _ in $(seq 1 $(( CP_TIMEOUT / 5 ))); do
	if [[ -z $FWD_PID ]] || ! kill -0 "$FWD_PID" 2>/dev/null; then
		hcp_data=$(status_data "$id" hcp)
		hcp_mc=$(jq -r '.mc // ""' <<<"$hcp_data")
		hcp_pod=$(jq -r '.pod // ""' <<<"$hcp_data")
		hcp_kubeconfig=$(jq -r '.kubeconfigRef // ""' <<<"$hcp_data")
		printf '%s\n' "$hcp_kubeconfig" > "$ADMIN_KUBECONFIG"
		chmod 600 "$ADMIN_KUBECONFIG"
		start_forward "$hcp_mc" "$hcp_pod" >/dev/null 2>&1 || true
	fi
	count=$(namespace_count 2>/dev/null || true)
	if [[ -n $count ]]; then
		if [[ $count == "$last_count" ]]; then
			steady=$(( steady + 1 ))
		else
			last_count=$count
			steady=1
		fi
		(( steady >= 6 )) && break
	else
		stop_forward
	fi
	sleep 5
done
(( steady >= 6 )) || fail "control plane namespace set never settled"
cp_boot_secs=$(( SECONDS - mint_cluster_start ))
echo "    control plane booted in ${cp_boot_secs}s with $last_count namespaces"
oc --kubeconfig "$ADMIN_KUBECONFIG" create ns "$DATA_NAMESPACE" >/dev/null 2>&1 || true
oc --kubeconfig "$ADMIN_KUBECONFIG" -n "$DATA_NAMESPACE" create configmap "$DATA_CONFIGMAP" \
	--from-literal=message="$DATA_MESSAGE" --dry-run=client -o yaml |
	oc --kubeconfig "$ADMIN_KUBECONFIG" apply -f - >/dev/null
pre_kill_ns=$(namespace_count)
echo "    created namespace $DATA_NAMESPACE and configmap $DATA_CONFIGMAP"
echo "    oc get ns -> $pre_kill_ns namespaces"
oc --kubeconfig "$ADMIN_KUBECONFIG" get ns | sed 's/^/    /'

echo "==> Killing etcd-mc-0 with both workloads"
stop_forward
kill_ns=$(date +%s%N)
kind delete cluster --name etcd-mc-0 >/dev/null 2>&1
rm -f "$FLEET_DIR/etcd-mc-0.kubeconfig" "$CONTAINER_FLEET/etcd-mc-0.kubeconfig"

echo "==> Watching quorum hold, member replacement, and re-home"
quorum_hold_secs=""
repair_secs=""
rehome_seen_secs=""
rehome_ready_secs=""
rehome_mc=""
deadline=$(( SECONDS + REHOME_TIMEOUT ))
while (( SECONDS < deadline )); do
	etcd_data=$(status_data "$id" etcd)
	hcp_data=$(status_data "$id" hcp)
	quorum=$(jq -r 'if has("quorum") then .quorum else "unknown" end' <<<"$etcd_data")
	members_ready=$(jq -r '.membersReady // 0' <<<"$etcd_data")
	# A tick that raced the kill can publish an empty error report; wait for
	# the next good report instead of calling that a lost quorum.
	if [[ $quorum == false ]]; then
		fail "quorum was lost after the kill: $etcd_data"
	fi
	if [[ $quorum == true && -z $quorum_hold_secs ]] && (( members_ready < 3 )); then
		quorum_hold_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		echo "    quorum held on $members_ready members at +${quorum_hold_secs}s"
	fi
	if [[ -z $repair_secs ]] &&
		[[ $(etcd_member_mc "$etcd_data" "$member0") == etcd-mc-3 ]] &&
		[[ $(etcd_member_healthy "$etcd_data" "$member0") == true ]]; then
		repair_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		echo "    $member0 replaced on etcd-mc-3 at +${repair_secs}s"
	fi
	hcp_mc=$(jq -r '.mc // ""' <<<"$hcp_data")
	if [[ -z $rehome_seen_secs && -n $hcp_mc && $hcp_mc != "$member0_mc" ]]; then
		rehome_seen_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		rehome_mc=$hcp_mc
		echo "    control plane re-homing to $rehome_mc at +${rehome_seen_secs}s"
	fi
	if [[ -n $repair_secs && -n $rehome_seen_secs &&
		$(jq -r '.ready // false' <<<"$hcp_data") == true ]]; then
		rehome_ready_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		break
	fi
	sleep 2
done
[[ -n $quorum_hold_secs ]] || echo "    WARN: repair outran the quorum dip sample"
[[ -n $repair_secs ]] || fail "$member0 was not replaced within ${REHOME_TIMEOUT}s"
[[ -n $rehome_seen_secs ]] || fail "control plane did not re-home within ${REHOME_TIMEOUT}s"
[[ -n $rehome_ready_secs ]] || fail "re-homed control plane did not become ready"

echo "==> Reattaching to the re-homed control plane"
post_ns=""
for attempt in $(seq 1 $(( REHOME_TIMEOUT / 5 ))); do
	hcp_data=$(status_data "$id" hcp)
	hcp_mc=$(jq -r '.mc // ""' <<<"$hcp_data")
	hcp_pod=$(jq -r '.pod // ""' <<<"$hcp_data")
	hcp_kubeconfig=$(jq -r '.kubeconfigRef // ""' <<<"$hcp_data")
	if [[ $hcp_mc != "$member0_mc" && -n $hcp_mc && -n $hcp_pod &&
		-n $hcp_kubeconfig && $(jq -r '.ready // false' <<<"$hcp_data") == true ]]; then
		printf '%s\n' "$hcp_kubeconfig" > "$ADMIN_KUBECONFIG"
		chmod 600 "$ADMIN_KUBECONFIG"
		if start_forward "$hcp_mc" "$hcp_pod"; then
			post_ns=$(namespace_count)
			if [[ $post_ns == "$pre_kill_ns" ]]; then
				sleep 5
				if [[ $(namespace_count) == "$pre_kill_ns" ]]; then
					break
				fi
			fi
		fi
	fi
	stop_forward
	sleep 5
done
[[ -n $post_ns ]] || fail "re-homed control plane never served the cluster again"
serving_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
message=$(oc --kubeconfig "$ADMIN_KUBECONFIG" -n "$DATA_NAMESPACE" \
	get configmap "$DATA_CONFIGMAP" -o jsonpath='{.data.message}' 2>/dev/null || true)
[[ $post_ns == "$pre_kill_ns" ]] ||
	fail "namespace count changed across the kill: $pre_kill_ns -> $post_ns"
[[ $message == "$DATA_MESSAGE" ]] ||
	fail "data object did not survive the kill (got ${message:-nothing})"
echo "    oc get ns -> $post_ns namespaces (same as before)"
echo "    configmap $DATA_NAMESPACE/$DATA_CONFIGMAP survived intact"
oc --kubeconfig "$ADMIN_KUBECONFIG" get ns | sed 's/^/    /'

echo "==> Tearing down the demo cluster"
stop_forward
teardown_start=$SECONDS
code=$(http DELETE "/clusters/$id")
[[ $code == 202 || $code == 404 ]] ||
	fail "delete returned HTTP $code: $(<"$TMP/body")"
wait_for_deletion "$id" ||
	fail "cluster did not finish deleting within ${DELETE_TIMEOUT}s"
teardown_secs=$(( SECONDS - teardown_start ))
remaining=0
for _ in $(seq 1 60); do
	remaining=0
	for mc in $FLEET_MCS; do
		[[ $mc == "$member0_mc" ]] && continue
		if kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" get ns "$NS" \
			>/dev/null 2>&1; then
			remaining=1
		fi
	done
	(( remaining == 0 )) && break
	sleep 2
done
(( remaining == 0 )) || fail "hcp namespace $NS still exists after teardown"

echo
echo "PASS: the hosted control plane re-homed and kept all of its data."
printf '%-36s %ss\n' 'etcd quorum formation:' "$formation_secs"
printf '%-36s %ss\n' 'first control-plane boot:' "$cp_boot_secs"
printf '%-36s %ss\n' 'kill -> quorum held:' "$quorum_hold_secs"
printf '%-36s %ss\n' 'kill -> member replaced:' "$repair_secs"
printf '%-36s %ss\n' 'kill -> re-homed (kubeconfig):' "$rehome_ready_secs"
printf '%-36s %ss\n' 'kill -> control plane serving:' "$serving_secs"
printf '%-36s %ss\n' 'total recovery RTO:' "$serving_secs"
printf '%-36s %ss\n' 'teardown:' "$teardown_secs"
printf '%-36s %s\n' 're-home target:' "$rehome_mc (co-located with a surviving member)"
printf '%-36s %s\n' 'namespaces before/after kill:' "$pre_kill_ns/$post_ns"
