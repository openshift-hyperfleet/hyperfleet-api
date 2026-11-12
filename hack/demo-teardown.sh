#!/usr/bin/env bash
# Tear down what demo-stage started: delete the cluster record through the
# API and wait for finalize, stop the detached port-forward, and remove the
# demo server and database containers. Safe to run again if pieces are gone.
set -euo pipefail

source "$(dirname "$0")/demo-lib.sh"
demo_init
trap demo_cleanup_temp EXIT
require_commands podman curl jq

echo "==> Deleting the $CLUSTER cluster record"
code=$(http GET "/clusters?size=100" 2>/dev/null || true)
if [[ $code == 200 ]]; then
	id=$(jq -r --arg name "$CLUSTER" '.items[]? | select(.name == $name) | .id' "$TMP/body" |
		head -n 1)
	if [[ -n $id ]]; then
		code=$(http DELETE "/clusters/$id")
		case "$code" in
		202)
			wait_for_deletion "$id" ||
				fail "cluster did not finish deleting within ${DELETE_TIMEOUT}s"
			echo "    finalized $id"
			;;
		404)
			echo "    already gone"
			;;
		*)
			fail "delete returned HTTP $code: $(<"$TMP/body")"
			;;
		esac
	else
		echo "    no cluster record"
	fi
else
	echo "    API not reachable (HTTP $code); skipping cluster delete"
fi

echo "==> Stopping the API port-forward"
stop_detached_forward
rm -f "$HCP_KUBECONFIG" "$STATE_DIR/$CLUSTER.json"

echo "==> Removing demo containers"
for name in "$SERVER" "$PSQL"; do
	if podman container exists "$name" >/dev/null 2>&1; then
		podman rm -f "$name" >/dev/null 2>&1 || true
		echo "    removed $name"
	else
		echo "    $name already gone"
	fi
done

cat <<CARD

Teardown complete.
For full substrate teardown (the four kind management clusters):
  ./hack/fleet.sh down
CARD
