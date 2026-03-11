#!/usr/bin/env bash
# Test 02 — Partial adapters: one required adapter reports True, other is still missing

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 02: Partial adapters — one required adapter True, one missing${NC}"
echo ""
echo "  When only some required adapters have reported, the snapshot is"
echo "  inconsistent (adapters not all at the same generation). Available must"
echo "  stay unchanged. Ready stays False and refreshes its last_updated_time"
echo "  so Sentinel knows the cluster is actively being reconciled."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=False@gen1, Ready=False@gen1 (initial)"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER2}@gen1=True (${ADAPTER1} still missing)"
echo -e "  ${YELLOW}Expected:${NC}        Available=False unchanged (guard fires), Ready=False,"
echo -e "           ${YELLOW}         ${NC} Available timestamps frozen, Ready.LUT refreshed"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc02-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "GET cluster — baseline (initial state)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
BASELINE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_READY_UPDATED=$(condition_field "$BASELINE" Ready last_updated_time)
log_received "Available.LUT=$BASELINE_AVAIL_UPDATED"
log_received "Ready.LUT=$BASELINE_READY_UPDATED"

# Brief pause so a timestamp change would be detectable
sleep 1

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER2}@gen1=True  (${ADAPTER1} is still missing)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True")
assert_http "${ADAPTER2} report accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post-event"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after event" "$CLUSTER"

echo ""
echo "  Available condition (${ADAPTER1} missing → snapshot inconsistent → guard preserves):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)

assert_eq "Available.status"              "False" "$AVAIL_STATUS"
assert_eq "Available.observed_generation" "1"     "$AVAIL_OBSGEN"
assert_eq "Available.last_updated_time preserved" "$BASELINE_AVAIL_UPDATED"    "$AVAIL_UPDATED"
assert_eq "Available.last_transition_time preserved" "$BASELINE_AVAIL_TRANSITION" "$AVAIL_TRANSITION"

echo ""
echo "  Ready condition (always refreshes last_updated_time when False):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_UPDATED=$(condition_field "$CLUSTER" Ready last_updated_time)

assert_eq "Ready.status"              "False" "$READY_STATUS"
assert_eq "Ready.observed_generation" "1"     "$READY_OBSGEN"
assert_changed "Ready.last_updated_time refreshed" "$BASELINE_READY_UPDATED" "$READY_UPDATED"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  With ${ADAPTER1} still missing the snapshot is inconsistent — not all"
echo "  required adapters have reported at the same generation — so the Available"
echo "  guard fires and all Available timestamps are frozen unchanged."
echo "  Ready=False always sets last_updated_time=now, which keeps Sentinel's"
echo "  activity clock ticking even while the cluster is still converging."
echo ""
test_summary
