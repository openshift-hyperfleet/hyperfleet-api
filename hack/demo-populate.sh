#!/usr/bin/env bash
# Bring up the hosted-control-plane demo and leave it running for a live run.
# The cluster is minted on hosted/hcp, the map is held until etcd quorum is
# healthy and the control plane answers a real API call, then a detached
# port-forward is left behind so oc works immediately.
set -euo pipefail

DEMO_DETACHED_FORWARD=true
source "$(dirname "$0")/demo-lib.sh"
demo_init
cleanup() {
	if (( $? != 0 )); then
		stop_detached_forward
	fi
	demo_cleanup_temp
}
trap cleanup EXIT

demo_preflight
demo_ensure_fleet
demo_mint_image
demo_load_image
demo_start_postgres
demo_start_server
demo_clean_start
demo_mint_cluster
ADMIN_KUBECONFIG=$HCP_KUBECONFIG
demo_wait_ready

ETCD_DATA=$(status_data "$CLUSTER_ID" etcd)
HCP_DATA=$(status_data "$CLUSTER_ID" hcp)
HCP_MC=$(jq -r '.mc // ""' <<<"$HCP_DATA")
HCP_POD=$(jq -r '.pod // ""' <<<"$HCP_DATA")
MEMBER1_NAME="$CLUSTER-etcd-m1"
MEMBER1_MC=$(etcd_member_mc "$ETCD_DATA" "$MEMBER1_NAME")
MEMBER1_POD="$MEMBER1_NAME-0"
[[ -n $HCP_MC && -n $HCP_POD ]] || fail "control plane status is missing its MC or pod"
[[ -n $MEMBER1_MC ]] || fail "etcd status is missing $MEMBER1_NAME"

member_mcs=" $(jq -r '.members[].mc' <<<"$ETCD_DATA" | tr '\n' ' ') "
spare_mc=""
for mc in $FLEET_MCS; do
	if [[ $member_mcs != *" $mc "* ]]; then
		spare_mc=$mc
		break
	fi
done
[[ -n $spare_mc ]] || spare_mc=$MEMBER0_MC
forward_pid=$(<"$FORWARD_PID_FILE")

cat <<EOF

Demo is live.
  Dashboard:   $API/poc-dashboard
  oc access:   KUBECONFIG=$HCP_KUBECONFIG oc get ns   (port-forward running, pid $forward_pid)

Your controls:
  Kill an MC (the full story - member-0 + control plane are co-located there):
      ./hack/demo-kill-mc.sh $MEMBER0_MC
    (kill $MEMBER1_MC instead to see member-only recovery; the spare $spare_mc receives)
  Restart a member pod (controller-free healing, ~10s):
      kubectl --kubeconfig $FLEET_DIR/$MEMBER1_MC.kubeconfig -n $ETCD_NS delete pod $MEMBER1_POD
  Restart the control plane pod (in-place, warm from its PVC):
      kubectl --kubeconfig $FLEET_DIR/$HCP_MC.kubeconfig -n $NS delete pod $HCP_POD
  Stop CLM itself (healing continues without it):
      podman stop $SERVER   (podman start $SERVER to bring it back)

Teardown:  ./hack/demo-teardown.sh
EOF
