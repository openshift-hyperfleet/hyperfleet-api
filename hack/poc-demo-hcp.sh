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

source "$(dirname "$0")/demo-lib.sh"
demo_init
trap demo_cleanup EXIT

demo_preflight
demo_ensure_fleet
demo_mint_image
demo_load_image
demo_start_postgres
demo_start_server
demo_clean_start
demo_mint_cluster
demo_wait_ready

oc --kubeconfig "$ADMIN_KUBECONFIG" create ns "$DATA_NAMESPACE" >/dev/null 2>&1 || true
oc --kubeconfig "$ADMIN_KUBECONFIG" -n "$DATA_NAMESPACE" create configmap "$DATA_CONFIGMAP" \
	--from-literal=message="$DATA_MESSAGE" --dry-run=client -o yaml |
	oc --kubeconfig "$ADMIN_KUBECONFIG" apply -f - >/dev/null
pre_kill_ns=$(namespace_count)
echo "    created namespace $DATA_NAMESPACE and configmap $DATA_CONFIGMAP"
echo "    oc get ns -> $pre_kill_ns namespaces"
oc --kubeconfig "$ADMIN_KUBECONFIG" get ns | sed 's/^/    /'

echo "==> Killing mc-0 with both workloads"
stop_forward
kill_ns=$(date +%s%N)
demo_delete_mc mc-0

echo "==> Watching quorum hold, member replacement, and re-home"
quorum_hold_secs=""
repair_secs=""
rehome_seen_secs=""
rehome_ready_secs=""
rehome_mc=""
deadline=$(( SECONDS + REHOME_TIMEOUT ))
while (( SECONDS < deadline )); do
	ETCD_DATA=$(status_data "$CLUSTER_ID" etcd)
	HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
	quorum=$(jq -r 'if has("quorum") then .quorum else "unknown" end' <<<"$ETCD_DATA")
	members_ready=$(jq -r '.membersReady // 0' <<<"$ETCD_DATA")
	# A tick that raced the kill can publish an empty error report; wait for
	# the next good report instead of calling that a lost quorum.
	if [[ $quorum == false ]]; then
		fail "quorum was lost after the kill: $ETCD_DATA"
	fi
	if [[ $quorum == true && -z $quorum_hold_secs ]] && (( members_ready < 3 )); then
		quorum_hold_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		echo "    quorum held on $members_ready members at +${quorum_hold_secs}s"
	fi
	if [[ -z $repair_secs ]] &&
		[[ $(etcd_member_mc "$ETCD_DATA" "$MEMBER0") == mc-3 ]] &&
		[[ $(etcd_member_healthy "$ETCD_DATA" "$MEMBER0") == true ]]; then
		repair_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		echo "    $MEMBER0 replaced on mc-3 at +${repair_secs}s"
	fi
	hcp_mc=$(jq -r '.mc // ""' <<<"$HCP_DATA")
	if [[ -z $rehome_seen_secs && -n $hcp_mc && $hcp_mc != "$MEMBER0_MC" ]]; then
		rehome_seen_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		rehome_mc=$hcp_mc
		echo "    control plane re-homing to $rehome_mc at +${rehome_seen_secs}s"
	fi
	if [[ -n $repair_secs && -n $rehome_seen_secs &&
		$(jq -r '.ready // false' <<<"$HCP_DATA") == true ]]; then
		rehome_ready_secs=$(( ( $(date +%s%N) - kill_ns ) / 1000000000 ))
		break
	fi
	sleep 2
done
[[ -n $quorum_hold_secs ]] || echo "    WARN: repair outran the quorum dip sample"
[[ -n $repair_secs ]] || fail "$MEMBER0 was not replaced within ${REHOME_TIMEOUT}s"
[[ -n $rehome_seen_secs ]] || fail "control plane did not re-home within ${REHOME_TIMEOUT}s"
[[ -n $rehome_ready_secs ]] || fail "re-homed control plane did not become ready"

echo "==> Reattaching to the re-homed control plane"
post_ns=""
for attempt in $(seq 1 $(( REHOME_TIMEOUT / 5 ))); do
	HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
	hcp_mc=$(jq -r '.mc // ""' <<<"$HCP_DATA")
	hcp_pod=$(jq -r '.pod // ""' <<<"$HCP_DATA")
	hcp_kubeconfig=$(jq -r '.kubeconfigRef // ""' <<<"$HCP_DATA")
	if [[ $hcp_mc != "$MEMBER0_MC" && -n $hcp_mc && -n $hcp_pod &&
		-n $hcp_kubeconfig && $(jq -r '.ready // false' <<<"$HCP_DATA") == true ]]; then
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
code=$(http DELETE "/clusters/$CLUSTER_ID")
[[ $code == 202 || $code == 404 ]] ||
	fail "delete returned HTTP $code: $(<"$TMP/body")"
wait_for_deletion "$CLUSTER_ID" ||
	fail "cluster did not finish deleting within ${DELETE_TIMEOUT}s"
teardown_secs=$(( SECONDS - teardown_start ))
remaining=0
for _ in $(seq 1 60); do
	remaining=0
	for mc in $FLEET_MCS; do
		[[ $mc == "$MEMBER0_MC" ]] && continue
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
printf '%-36s %ss\n' 'etcd quorum formation:' "$FORMATION_SECS"
printf '%-36s %ss\n' 'first control-plane boot:' "$CP_BOOT_SECS"
printf '%-36s %ss\n' 'kill -> quorum held:' "$quorum_hold_secs"
printf '%-36s %ss\n' 'kill -> member replaced:' "$repair_secs"
printf '%-36s %ss\n' 'kill -> re-homed (kubeconfig):' "$rehome_ready_secs"
printf '%-36s %ss\n' 'kill -> control plane serving:' "$serving_secs"
printf '%-36s %ss\n' 'total recovery RTO:' "$serving_secs"
printf '%-36s %ss\n' 'teardown:' "$teardown_secs"
printf '%-36s %s\n' 're-home target:' "$rehome_mc (co-located with a surviving member)"
printf '%-36s %s\n' 'namespaces before/after kill:' "$pre_kill_ns/$post_ns"
