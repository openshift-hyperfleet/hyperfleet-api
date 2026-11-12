#!/usr/bin/env bash
# Live PoC demo: create a cluster, wait for coreshift, check `oc get ns`,
# then delete it and wait for the control plane to disappear.
# Re-runnable: finalizes earlier poc-demo cluster records before creating a
# new one, so a mid-run failure is recovered by running it again.
set -euo pipefail

API=${CLM_POC_API_URL:-http://localhost:8000/api/hyperfleet/v1}
READY_TIMEOUT=${CLM_POC_READY_TIMEOUT:-1800}
DELETE_TIMEOUT=${CLM_POC_DELETE_TIMEOUT:-600}
NAME=${CLM_POC_CLUSTER_NAME:-poc-demo-$(date +%Y%m%d%H%M%S)}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

if [[ ${CLM_POC_E2E:-} != 1 ]]; then
	echo "SKIP: this live demo only runs on demand." >&2
	echo "      export CLM_POC_E2E=1 (plus podman and a pull secret) to run it." >&2
	exit 2
fi

for cmd in curl jq oc podman; do
	command -v "$cmd" >/dev/null || fail "required command not found: $cmd"
done
[[ -n ${HYPERFLEET_CORESHIFT_PULL_SECRET_FILE:-} ]] ||
	fail "HYPERFLEET_CORESHIFT_PULL_SECRET_FILE is not set; export the pull-secret path"
[[ -r ${HYPERFLEET_CORESHIFT_PULL_SECRET_FILE:-} ]] ||
	fail "pull secret is not readable at HYPERFLEET_CORESHIFT_PULL_SECRET_FILE"

# http METHOD PATH [JSON] -> prints the HTTP status code, body in $TMP/body
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

code=$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "$API/clusters?size=1") ||
	fail "server is not reachable at $API; start it with 'make run-no-auth'"
[[ $code == 200 ]] ||
	fail "server at $API returned HTTP $code; the demo needs the no-auth local server"

echo "==> Finalizing earlier poc-demo clusters"
code=$(http GET "/clusters?size=100")
[[ $code == 200 ]] || fail "list clusters returned HTTP $code: $(<"$TMP/body")"
mapfile -t old_ids < <(jq -r --arg name "$NAME" '.items[]? |
	select(.name == $name or ((.name // "") | startswith("poc-demo-"))) | .id' "$TMP/body")
for old in "${old_ids[@]}"; do
	echo "    finalizing $old"
	code=$(http DELETE "/clusters/$old")
	[[ $code == 202 || $code == 404 ]] ||
		fail "delete cluster $old returned HTTP $code: $(<"$TMP/body")"
	wait_for_deletion "$old" ||
		fail "cluster $old did not finish deleting within ${DELETE_TIMEOUT}s"
done

echo "Demo cluster: $NAME"
echo

start=$SECONDS
code=$(http POST /clusters "{\"kind\":\"Cluster\",\"name\":\"$NAME\",\"spec\":{\"topology\":\"hosted\",\"platform\":\"coreshift\"}}")
[[ $code == 201 ]] || fail "create returned HTTP $code: $(<"$TMP/body")"
id=$(jq -r '.id // empty' "$TMP/body")
[[ -n $id ]] || fail "create response has no id: $(<"$TMP/body")"
echo "[  0s] created cluster $id (HTTP 201)"

echo "Waiting for Reconciled (budget ${READY_TIMEOUT}s)..."
while (( SECONDS - start < READY_TIMEOUT )); do
	code=$(http GET "/clusters/$id")
	if [[ $code != 200 ]]; then
		printf '[%4ds] GET cluster -> HTTP %s\n' "$((SECONDS - start))" "$code"
		sleep 5
		continue
	fi

	read -r status reason generation < <(jq -r '
		[.status.conditions[]? | select(.type == "Reconciled")][0] as $c |
		"\($c.status // "Unknown") \($c.reason // "-") \(.generation)"' "$TMP/body")
	printf '[%4ds] Reconciled=%s (%s), generation %s\n' \
		"$((SECONDS - start))" "$status" "$reason" "$generation"
	[[ $status == True ]] && break
	sleep 5
done
[[ ${status:-} == True ]] || fail "cluster did not reach Reconciled within ${READY_TIMEOUT}s"
ready_secs=$((SECONDS - start))

code=$(http GET "/clusters/$id/statuses")
[[ $code == 200 ]] || fail "statuses returned HTTP $code: $(<"$TMP/body")"
kubeconfig=$(jq -r '[.items[]? | select(.adapter == "coreshift")][0].data.kubeconfig // empty' "$TMP/body")
[[ -n $kubeconfig && -f $kubeconfig ]] || fail "coreshift status did not provide a kubeconfig file"

echo
echo "Checking cluster access with the minted kubeconfig..."
timeout 60 oc --kubeconfig "$kubeconfig" get ns || fail "oc get ns failed"

echo
delete_start=$SECONDS
code=$(http DELETE "/clusters/$id")
[[ $code == 202 ]] || fail "delete returned HTTP $code: $(<"$TMP/body")"
echo "[  0s] DELETE cluster -> HTTP 202"

echo "Waiting for deletion (budget ${DELETE_TIMEOUT}s)..."
while (( SECONDS - delete_start < DELETE_TIMEOUT )); do
	code=$(http GET "/clusters/$id")
	if [[ $code == 404 ]]; then
		printf '[%4ds] cluster is gone (HTTP 404)\n' "$((SECONDS - delete_start))"
		break
	fi
	if [[ $code == 200 ]]; then
		read -r phase reason < <(jq -r '
			(if .deleted_time then "Finalizing" else "Present" end) + " " +
			([.status.conditions[]? | select(.type == "Reconciled")][0].reason // "-")' "$TMP/body")
		printf '[%4ds] %s, Reconciled reason %s\n' \
			"$((SECONDS - delete_start))" "$phase" "$reason"
	else
		printf '[%4ds] GET cluster -> HTTP %s\n' "$((SECONDS - delete_start))" "$code"
	fi
	sleep 5
done
[[ $code == 404 ]] || fail "cluster was not deleted within ${DELETE_TIMEOUT}s"
gone_secs=$((SECONDS - delete_start))

echo
echo "PASS: create -> Reconciled in ${ready_secs}s; delete -> gone in ${gone_secs}s"
