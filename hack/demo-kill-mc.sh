#!/usr/bin/env bash
# Delete one management cluster the way a live failure does: the cluster and
# its kubeconfig disappear from both fleet views, so the adapter chain sees it
# gone and repairs the workloads on the survivors.
# Safe to re-run: an already-dead cluster reports "already gone" and exits 0,
# and missing kubeconfigs are ignored.
set -euo pipefail

source "$(dirname "$0")/demo-lib.sh"

[[ $# -eq 1 ]] || fail "usage: $0 <mc>"
mc=$1
require_commands go podman kubectl

if podman container exists "$mc-control-plane" >/dev/null 2>&1; then
	echo "==> Killing management cluster $mc"
	demo_delete_mc "$mc" || fail "failed to delete management cluster $mc"
	podman container exists "$mc-control-plane" >/dev/null 2>&1 &&
		fail "management cluster $mc is still running"

cat <<EOF

Killed $mc. Watch the dashboard: $API/poc-dashboard
  - the $mc box grays out and its workloads turn red
  - the etcd quorum badge holds at 2/3
  - the failed member is replaced on the spare and the control plane re-lands there
  - the cold re-home takes a few minutes; oc resumes when the new pod is ready

Namespaces under repair: $ETCD_NS and $NS
EOF
else
	demo_forget_mc "$mc"
	echo "==> Management cluster $mc is already gone; removed any stale kubeconfigs"
fi
