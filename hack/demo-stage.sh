#!/usr/bin/env bash
# Prepare the hosted-control-plane demo and leave the stack live. This script
# only does the machinery: fleet, image, postgres, server, clean start. The
# cluster creation, port-forward, kill, and cleanup are typed on stage from
# the card printed at the end.
set -euo pipefail

source "$(dirname "$0")/demo-lib.sh"
demo_init
trap demo_cleanup_temp EXIT

# Values rendered into the on-stage card. These match the manifests in
# pkg/adapters/hcp_manifests.go and the placement used by the demo server.
HCP_DEPLOYMENT=hcp
HCP_STATE_KUBECONFIG=/var/lib/coreshift/state/coreshift/clusters/coreshift/admin.kubeconfig
STAGE_CP_MC=mc-0
STAGE_RECOVERY_MC=mc-3
FLEET_DISPLAY=${FLEET_DIR#"$REPO_ROOT"/}
CONTAINER_FLEET_DISPLAY=${CONTAINER_FLEET#"$REPO_ROOT"/}

demo_preflight
demo_ensure_fleet
demo_mint_image
demo_load_image
demo_start_postgres
demo_start_server
demo_clean_start

cat <<EOF

Demo stage is ready. Your performance:

  0. (if you want kind commands direct — the scripts already export this)
     export KIND_EXPERIMENTAL_PROVIDER=podman

  1. CREATE the cluster — the moment:
     ID=\$(curl -s -X POST $API/clusters \\
       -H 'Content-Type: application/json' \\
       -d '{"kind":"Cluster","name":"$CLUSTER","spec":{"topology":"hosted","platform":"hcp","etcd":{"managed":"clm"}}}' | jq -r .id)
     echo "\$ID"

  2. WATCH it build (dashboard is best: $API/poc-dashboard; terminal alternative):
     watch -n2 "curl -s $API/clusters/\$ID | jq -c '.status.conditions[] | {type,status,reason}'"
     etcd members landing on the MCs:
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_CP_MC.kubeconfig -n $ETCD_NS get pods
     control plane booting (minutes — the payload pull):
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_CP_MC.kubeconfig -n $NS get pods

  3. WHEN READY — use the control plane (wait for Reconciled=True; if the API
     is still booting, wait and rerun the last line):
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_CP_MC.kubeconfig -n $NS port-forward deployment/$HCP_DEPLOYMENT 6443:6443 &
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_CP_MC.kubeconfig -n $NS exec deployment/$HCP_DEPLOYMENT -- cat $HCP_STATE_KUBECONFIG > /tmp/demo-admin.kubeconfig
     KUBECONFIG=/tmp/demo-admin.kubeconfig oc get ns

  4. THE KILL — typed, visible:
     go tool -modfile=tools/go.mod kind delete cluster --name $STAGE_CP_MC
     rm $FLEET_DISPLAY/$STAGE_CP_MC.kubeconfig $CONTAINER_FLEET_DISPLAY/$STAGE_CP_MC.kubeconfig
     (the kubeconfig removal is what the fleet view sees — don't skip it)

  5. WATCH the recovery (minutes): dashboard; or:
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_RECOVERY_MC.kubeconfig -n $ETCD_NS get pods
     then re-forward and prove the data survived:
     kill %1 2>/dev/null || true
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_RECOVERY_MC.kubeconfig -n $NS port-forward deployment/$HCP_DEPLOYMENT 6443:6443 &
     kubectl --kubeconfig $FLEET_DISPLAY/$STAGE_RECOVERY_MC.kubeconfig -n $NS exec deployment/$HCP_DEPLOYMENT -- cat $HCP_STATE_KUBECONFIG > /tmp/demo-admin.kubeconfig
     KUBECONFIG=/tmp/demo-admin.kubeconfig oc get ns
     (the re-homed control plane has fresh certs; same namespaces and anything you created is still there)

  6. CLEANUP:
     curl -X DELETE $API/clusters/\$ID
     ./hack/demo-teardown.sh

  Convenience wrapper if you'd rather not type the kill: ./hack/demo-kill-mc.sh $STAGE_CP_MC

EOF
