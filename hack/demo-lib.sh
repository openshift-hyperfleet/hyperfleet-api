#!/usr/bin/env bash
# Shared machinery for the hosted-control-plane demo scripts. Source this
# file; it defines configuration and functions, then each script runs only
# the phases it needs. The pull secret is handed to coreshift by path and
# never enters the repo, logs, or status payloads.

DEMO_LIB_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
REPO_ROOT=${REPO_ROOT:-$DEMO_LIB_ROOT}
CLUSTER=${CLM_POC_HCP_CLUSTER:-demo}
FLEET_MCS=${FLEET_MCS:-"mc-0 mc-1 mc-2 mc-3"}
FLEET_DIR=${FLEET_DIR:-$REPO_ROOT/.fleet}
CONTAINER_FLEET=${CONTAINER_FLEET:-$REPO_ROOT/.fleet-container}
STATE_DIR=${STATE_DIR:-$FLEET_DIR/etcd-state}
API_PORT=${CLM_POC_API_PORT:-8000}
API=${API:-http://localhost:$API_PORT/api/hyperfleet/v1}
PSQL=${CLM_POC_HCP_PSQL:-clm-poc-hcp-psql}
SERVER=${CLM_POC_HCP_SERVER:-clm-poc-hcp-server}
IMAGE=${IMAGE:-registry.access.redhat.com/ubi9/ubi-minimal}
CORESHIFT_REPO=${CORESHIFT_REPO:-$HOME/Projects/vkareh/coreshift}
MINT_PORT=${CLM_POC_HCP_MINT_PORT:-16443}
API_FORWARD_PORT=${API_FORWARD_PORT:-6443}
NS=${NS:-"hcp-$CLUSTER"}
ETCD_NS=${ETCD_NS:-"etcd-$CLUSTER"}
DATA_NAMESPACE=${CLM_POC_HCP_DATA_NS:-demo-data}
DATA_CONFIGMAP=${DATA_CONFIGMAP:-demo-proof}
DATA_MESSAGE=${CLM_POC_HCP_DATA_MESSAGE:-data-survived-the-kill}
FORMATION_TIMEOUT=${CLM_POC_FORMATION_TIMEOUT:-600}
CP_TIMEOUT=${CLM_POC_CP_TIMEOUT:-1800}
REHOME_TIMEOUT=${CLM_POC_REHOME_TIMEOUT:-1800}
DELETE_TIMEOUT=${CLM_POC_DELETE_TIMEOUT:-900}
HCP_KUBECONFIG=${CLM_POC_HCP_KUBECONFIG:-$FLEET_DIR/$CLUSTER-hcp.kubeconfig}
FORWARD_PID_FILE=${CLM_POC_HCP_FORWARD_PID_FILE:-/tmp/clm-poc-hcp-port-forward.pid}
FORWARD_LOG=${CLM_POC_HCP_FORWARD_LOG:-/tmp/clm-poc-hcp-port-forward.log}
DEMO_DETACHED_FORWARD=${DEMO_DETACHED_FORWARD:-false}

export KIND_EXPERIMENTAL_PROVIDER=podman

source "$DEMO_LIB_ROOT/hack/takeover-api-port.sh"

TMP=""
MINT_STATE=""
MINT_NAME=""
MINT_PID=""
MINTED_IMAGE=""
FWD_PID=""
ADMIN_KUBECONFIG=""

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

require_commands() {
	local cmd
	for cmd in "$@"; do
		command -v "$cmd" >/dev/null || fail "required command not found: $cmd"
	done
}

demo_init() {
	TMP=$(mktemp -d)
	MINT_STATE=$TMP/mint-state
	MINT_NAME=clm-mint-$$
	ADMIN_KUBECONFIG=${ADMIN_KUBECONFIG:-$TMP/admin.kubeconfig}
}

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
		code=$(http GET "/clusters/$id" || true)
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

stop_forward() {
	if [[ -n $FWD_PID ]] && kill -0 "$FWD_PID" 2>/dev/null; then
		kill "$FWD_PID" 2>/dev/null || true
		wait "$FWD_PID" 2>/dev/null || true
	fi
	FWD_PID=""
}

forward_alive() {
	local pid=""
	if [[ -n $FWD_PID ]]; then
		kill -0 "$FWD_PID" 2>/dev/null
		return
	fi
	[[ -f $FORWARD_PID_FILE ]] && pid=$(<"$FORWARD_PID_FILE")
	[[ -n $pid ]] && kill -0 "$pid" 2>/dev/null
}

start_detached_forward() {
	local mc=$1 pod=$2 pid
	stop_detached_forward
	: >"$FORWARD_LOG"
	setsid kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" -n "$NS" \
		port-forward "pod/$pod" "$API_FORWARD_PORT:6443" \
		>"$FORWARD_LOG" 2>&1 </dev/null &
	pid=$!
	echo "$pid" >"$FORWARD_PID_FILE"
	for _ in $(seq 1 20); do
		if kubectl --kubeconfig "$ADMIN_KUBECONFIG" get ns --request-timeout=3s \
			>/dev/null 2>&1; then
			return 0
		fi
		kill -0 "$pid" 2>/dev/null || break
		sleep 1
	done
	return 1
}

stop_detached_forward() {
	local pid=""
	[[ -f $FORWARD_PID_FILE ]] && pid=$(<"$FORWARD_PID_FILE")
	if [[ -n $pid ]] && kill -0 "$pid" 2>/dev/null; then
		kill "$pid" 2>/dev/null || true
	fi
	rm -f "$FORWARD_PID_FILE"
}

start_forward() {
	local mc=$1 pod=$2
	if [[ $DEMO_DETACHED_FORWARD == true ]]; then
		start_detached_forward "$mc" "$pod"
		return
	fi
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

demo_forget_mc() {
	local mc=$1
	rm -f "$FLEET_DIR/$mc.kubeconfig" "$CONTAINER_FLEET/$mc.kubeconfig"
}

demo_delete_mc() {
	local mc=$1
	kind delete cluster --name "$mc" >/dev/null 2>&1
	demo_forget_mc "$mc"
}

demo_stop_mint() {
	if [[ -n $MINT_PID ]] && kill -0 "$MINT_PID" 2>/dev/null; then
		kill "$MINT_PID" 2>/dev/null || true
		wait "$MINT_PID" 2>/dev/null || true
	fi
	MINT_PID=""
	if [[ -n $MINT_NAME ]] && podman container exists "coreshift-$MINT_NAME" >/dev/null 2>&1; then
		XDG_STATE_HOME="$MINT_STATE" "${CORESHIFT_BIN:-coreshift}" delete cluster \
			--name "$MINT_NAME" >/dev/null 2>&1 ||
			podman rm -f "coreshift-$MINT_NAME" >/dev/null 2>&1 || true
	fi
}

demo_cleanup_temp() {
	demo_stop_mint
	[[ -n $TMP ]] && rm -rf "$TMP"
}

demo_cleanup() {
	stop_forward
	demo_cleanup_temp
}

demo_preflight() {
	require_commands go podman kubectl oc curl jq base64 awk make
	PULL_SECRET=${HYPERFLEET_CORESHIFT_PULL_SECRET_FILE:-}
	[[ -n $PULL_SECRET ]] ||
		fail "HYPERFLEET_CORESHIFT_PULL_SECRET_FILE is not set; export the pull-secret path"
	[[ -r $PULL_SECRET ]] ||
		fail "pull secret is not readable at HYPERFLEET_CORESHIFT_PULL_SECRET_FILE"

	echo "==> Building the API server and locating coreshift"
	make -C "$REPO_ROOT" build >/dev/null
	cd "$REPO_ROOT"
	if [[ -n ${CORESHIFT_BIN:-} ]]; then
		[[ -x $CORESHIFT_BIN ]] || fail "CORESHIFT_BIN is not executable: $CORESHIFT_BIN"
	else
		make -C "$CORESHIFT_REPO" build >/dev/null
		CORESHIFT_BIN=$CORESHIFT_REPO/coreshift
	fi
}

demo_ensure_fleet() {
	echo "==> Ensuring four management clusters (reusing existing)"
	FLEET_MCS="$FLEET_MCS" FLEET_DIR="$FLEET_DIR" hack/fleet.sh up >/dev/null
	mkdir -p "$CONTAINER_FLEET" "$STATE_DIR"
	local mc ip
	for mc in $FLEET_MCS; do
		ip=$(podman inspect "$mc-control-plane" \
			--format '{{(index .NetworkSettings.Networks "kind").IPAddress}}')
		sed -E "s#server: https://127\.0\.0\.1:[0-9]+#server: https://$ip:6443#" \
			"$FLEET_DIR/$mc.kubeconfig" > "$CONTAINER_FLEET/$mc.kubeconfig"
	done
}

demo_mint_image() {
	echo "==> Minting the coreshift image (throwaway create, state discarded)"
	mkdir -p "$MINT_STATE"
	local mint_log=$TMP/mint.log
	local mint_container=coreshift-$MINT_NAME
	local mint_start
	XDG_STATE_HOME="$MINT_STATE" "$CORESHIFT_BIN" create cluster \
		--name "$MINT_NAME" --port "$MINT_PORT" \
		--pull-secret "$PULL_SECRET" >"$mint_log" 2>&1 &
	MINT_PID=$!
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
	MINTED_IMAGE=$(podman inspect "$mint_container" --format '{{.Config.Image}}')
	kill "$MINT_PID" 2>/dev/null || true
	wait "$MINT_PID" 2>/dev/null || true
	MINT_PID=""
	XDG_STATE_HOME="$MINT_STATE" "$CORESHIFT_BIN" delete cluster \
		--name "$MINT_NAME" >/dev/null 2>&1 || true
	[[ $MINTED_IMAGE == localhost/coreshift:* ]] ||
		fail "unexpected coreshift image ref: $MINTED_IMAGE"
	podman image exists "$MINTED_IMAGE" ||
		fail "minted image is not in the podman store: $MINTED_IMAGE"
	echo "    image: $MINTED_IMAGE"
}

demo_load_image() {
	echo "==> Loading the image into all four management clusters"
	local mc
	for mc in $FLEET_MCS; do
		kind load docker-image "$MINTED_IMAGE" --name "$mc" >/dev/null
	done
}

demo_start_postgres() {
	echo "==> Starting postgres and the no-auth server on the kind network"
	if podman container exists "$PSQL" >/dev/null 2>&1; then
		podman start "$PSQL" >/dev/null 2>&1 || true
	else
		podman run -d --name "$PSQL" --network kind \
			-e POSTGRES_DB=hyperfleet -e POSTGRES_USER=hyperfleet \
			-e POSTGRES_PASSWORD=foobar-bizz-buzz docker.io/library/postgres:14.24 >/dev/null
	fi
	local ready=0
	for _ in $(seq 1 180); do
		if podman exec "$PSQL" pg_isready -U hyperfleet >/dev/null 2>&1; then
			ready=1
			break
		fi
		sleep 1
	done
	[[ $ready == 1 ]] || fail "postgres did not start"
}

demo_start_server() {
	podman rm -f "$SERVER" >/dev/null 2>&1 || true
	podman run --rm --network kind \
		-v "$REPO_ROOT/bin/hyperfleet-api:/hyperfleet-api:ro,Z" \
		-v "$REPO_ROOT/configs:/configs:ro,Z" \
		--entrypoint /hyperfleet-api "$IMAGE" migrate --config /configs/dev.yaml \
		--db-host "$PSQL" --db-port 5432 --db-name hyperfleet \
		--db-username hyperfleet --db-password foobar-bizz-buzz >/dev/null
	takeover_api_port
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
		-e HYPERFLEET_ETCD_PREFERRED_MC=mc-0 \
		-e HYPERFLEET_HCP_IMAGE="$MINTED_IMAGE" \
		-e HYPERFLEET_CORESHIFT_PULL_SECRET_FILE=/pull-secret \
		--entrypoint /hyperfleet-api -d "$IMAGE" serve --config /configs/dev.yaml \
		--db-host "$PSQL" --db-port 5432 --db-name hyperfleet \
		--db-username hyperfleet --db-password foobar-bizz-buzz >/dev/null
	for _ in $(seq 1 60); do
		[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1" 2>/dev/null) == 200 ]] && break
		sleep 1
	done
	[[ $(curl -sS -o /dev/null -w '%{http_code}' "$API/clusters?size=1") == 200 ]] ||
		fail "server did not become ready"
}

demo_clean_start() {
	echo "==> Deleting any existing hosted clusters"
	local code id
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
	local mc
	for mc in $FLEET_MCS; do
		kubectl --kubeconfig "$FLEET_DIR/$mc.kubeconfig" delete ns "$NS" "$ETCD_NS" \
			--ignore-not-found --wait=true >/dev/null 2>&1 || true
	done
	rm -f "$STATE_DIR/$CLUSTER.json"
}

demo_mint_cluster() {
	echo "==> Minting cluster $CLUSTER on hosted/hcp"
	local body
	body=$(curl -sS -X POST "$API/clusters" -H 'Content-Type: application/json' \
		--data "{\"kind\":\"Cluster\",\"name\":\"$CLUSTER\",\"spec\":{\"topology\":\"hosted\",\"platform\":\"hcp\",\"etcd\":{\"managed\":\"clm\"}}}")
	CLUSTER_ID=$(jq -r '.id // empty' <<<"$body")
	[[ -n $CLUSTER_ID ]] || fail "create returned no id: $body"
	MINT_CLUSTER_START=$SECONDS
}

demo_wait_for_quorum() {
	echo "==> Waiting for etcd quorum formation"
	ETCD_DATA='{}'
	while (( SECONDS - MINT_CLUSTER_START < FORMATION_TIMEOUT )); do
		ETCD_DATA=$(status_data "$CLUSTER_ID" etcd)
		if [[ $(jq -r '.quorum // false' <<<"$ETCD_DATA") == true &&
			$(jq -r '.membersReady // 0' <<<"$ETCD_DATA") -ge 3 ]]; then
			break
		fi
		sleep 2
	done
	[[ $(jq -r '.quorum // false' <<<"$ETCD_DATA") == true ]] || fail "quorum did not form"
	[[ $(jq -r '.membersReady // 0' <<<"$ETCD_DATA") -ge 3 ]] ||
		fail "members did not become ready"
	FORMATION_SECS=$(( SECONDS - MINT_CLUSTER_START ))
	MEMBER0="$CLUSTER-etcd-m0"
	MEMBER0_MC=$(etcd_member_mc "$ETCD_DATA" "$MEMBER0")
	echo "    quorum ready in ${FORMATION_SECS}s; $MEMBER0 is on $MEMBER0_MC"
}

demo_wait_for_cp_placement() {
	echo "==> Waiting for the control plane pod next to member 0"
	HCP_DATA='{}'
	HCP_MC=""
	for _ in $(seq 1 120); do
		HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
		HCP_MC=$(jq -r '.mc // ""' <<<"$HCP_DATA")
		[[ $HCP_MC == "$MEMBER0_MC" ]] && break
		sleep 2
	done
	[[ $HCP_MC == "$MEMBER0_MC" ]] ||
		fail "hcp adapter placed the control plane on ${HCP_MC:-nothing}, wanted $MEMBER0_MC"
	echo "    control plane scheduled on $HCP_MC, co-located with $MEMBER0"
}

demo_wait_for_cp_ready() {
	echo "==> Booting the hosted control plane (payload pull happens here)"
	for _ in $(seq 1 $(( CP_TIMEOUT / 5 ))); do
		HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
		[[ $(jq -r '.ready // false' <<<"$HCP_DATA") == true ]] && break
		sleep 5
	done
	[[ $(jq -r '.ready // false' <<<"$HCP_DATA") == true ]] ||
		fail "control plane did not become ready within ${CP_TIMEOUT}s"
	STATUS_READY_SECS=$(( SECONDS - MINT_CLUSTER_START ))
	echo "    kubeconfig available in ${STATUS_READY_SECS}s; waiting for the API to answer"
}

demo_attach_control_plane() {
	echo "==> Attaching to the control plane and proving data"
	SERVED=0
	local attempt
	for attempt in $(seq 1 $(( CP_TIMEOUT / 5 ))); do
		HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
		HCP_MC=$(jq -r '.mc // ""' <<<"$HCP_DATA")
		HCP_POD=$(jq -r '.pod // ""' <<<"$HCP_DATA")
		HCP_KUBECONFIG_DATA=$(jq -r '.kubeconfigRef // ""' <<<"$HCP_DATA")
		if [[ -n $HCP_MC && -n $HCP_POD && -n $HCP_KUBECONFIG_DATA &&
			$(jq -r '.ready // false' <<<"$HCP_DATA") == true ]]; then
			printf '%s\n' "$HCP_KUBECONFIG_DATA" > "$ADMIN_KUBECONFIG"
			chmod 600 "$ADMIN_KUBECONFIG"
			if start_forward "$HCP_MC" "$HCP_POD" &&
				oc --kubeconfig "$ADMIN_KUBECONFIG" get ns >/dev/null 2>&1; then
				SERVED=1
				break
			fi
		fi
		stop_forward
		sleep 5
	done
	[[ $SERVED == 1 ]] || fail "control plane API never answered within ${CP_TIMEOUT}s"
	CP_FIRST_SERVE_SECS=$(( SECONDS - MINT_CLUSTER_START ))
	echo "    API answered in ${CP_FIRST_SERVE_SECS}s (pod $HCP_POD); waiting for the boot to settle"
}

demo_wait_for_boot() {
	local count
	LAST_COUNT=""
	STEADY=0
	for _ in $(seq 1 $(( CP_TIMEOUT / 5 ))); do
		if ! forward_alive; then
			HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
			HCP_MC=$(jq -r '.mc // ""' <<<"$HCP_DATA")
			HCP_POD=$(jq -r '.pod // ""' <<<"$HCP_DATA")
			HCP_KUBECONFIG_DATA=$(jq -r '.kubeconfigRef // ""' <<<"$HCP_DATA")
			printf '%s\n' "$HCP_KUBECONFIG_DATA" > "$ADMIN_KUBECONFIG"
			chmod 600 "$ADMIN_KUBECONFIG"
			start_forward "$HCP_MC" "$HCP_POD" >/dev/null 2>&1 || true
		fi
		count=$(namespace_count 2>/dev/null || true)
		if [[ -n $count ]]; then
			if [[ $count == "$LAST_COUNT" ]]; then
				STEADY=$(( STEADY + 1 ))
			else
				LAST_COUNT=$count
				STEADY=1
			fi
			(( STEADY >= 6 )) && break
		else
			stop_forward
		fi
		sleep 5
	done
	(( STEADY >= 6 )) || fail "control plane namespace set never settled"
	CP_BOOT_SECS=$(( SECONDS - MINT_CLUSTER_START ))
	echo "    control plane booted in ${CP_BOOT_SECS}s with $LAST_COUNT namespaces"
}

demo_wait_ready() {
	demo_wait_for_quorum
	demo_wait_for_cp_placement
	demo_wait_for_cp_ready
	demo_attach_control_plane
	demo_wait_for_boot
}
