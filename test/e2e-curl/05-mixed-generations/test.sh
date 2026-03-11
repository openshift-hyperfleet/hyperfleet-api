#!/usr/bin/env bash
# Test 05 — Mixed generations: one adapter upgrades to gen2, the other stays at gen1

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 05: Mixed generations — one adapter at gen2, one at gen1, both True${NC}"
echo ""
echo "  When adapters are at different observed generations the snapshot is"
echo "  inconsistent. Available must stay unchanged (guard fires). Ready stays"
echo "  False because not all adapters are at the current resource generation."
echo "  This is the normal in-flight state during a rolling adapter upgrade."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=True@gen1, Ready=False@gen2"
echo -e "           ${YELLOW}         ${NC} (${ADAPTER1}@gen1=True, ${ADAPTER2}@gen1=True, cluster at gen2)"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER2}@gen2=True (${ADAPTER1} still at gen1)"
echo -e "  ${YELLOW}Expected:${NC}        Available=True@gen1 unchanged (inconsistent snapshot),"
echo -e "           ${YELLOW}         ${NC} Ready=False@gen2 (${ADAPTER1} not at gen2), no timestamp changes"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc05-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "Setup: POST ${ADAPTER1}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True" > /dev/null
log_step "Setup: POST ${ADAPTER2}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True" > /dev/null
log_step "Setup: PATCH cluster → gen2"
PATCHED=$(patch_cluster "$CLUSTER_ID" '{"v":2}')
assert_eq "generation=2" "2" "$(echo "$PATCHED" | jq -r '.generation')"

log_step "GET cluster — pre-event baseline (Available=True@gen1, Ready=False@gen2)"
AFTER_PATCH=$(get_cluster "$CLUSTER_ID")
show_state "pre-event" "$AFTER_PATCH"
AVAIL_UPDATED_AFTER_PATCH=$(condition_field "$AFTER_PATCH" Available last_updated_time)
AVAIL_TRANSITION_AFTER_PATCH=$(condition_field "$AFTER_PATCH" Available last_transition_time)
READY_TRANSITION_AFTER_PATCH=$(condition_field "$AFTER_PATCH" Ready last_transition_time)

assert_eq "pre-event Available.status" "True"  "$(condition_field "$AFTER_PATCH" Available status)"
assert_eq "pre-event Available.obsgen" "1"     "$(condition_field "$AFTER_PATCH" Available observed_generation)"
assert_eq "pre-event Ready.status"    "False" "$(condition_field "$AFTER_PATCH" Ready    status)"

sleep 1

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER2}@gen2=True  (${ADAPTER1} still at gen1 — snapshot inconsistent)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 2 "True")
assert_http "${ADAPTER2}@gen2 accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post-event"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after event" "$CLUSTER"

echo ""
echo "  Available condition (snapshot inconsistent — ${ADAPTER1}@gen1, ${ADAPTER2}@gen2 → guard fires):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)

assert_eq "Available.status"                         "True" "$AVAIL_STATUS"
assert_eq "Available.observed_generation (min=gen1)" "1"    "$AVAIL_OBSGEN"
assert_eq "Available.last_updated_time preserved"    "$AVAIL_UPDATED_AFTER_PATCH"    "$AVAIL_UPDATED"
assert_eq "Available.last_transition_time preserved" "$AVAIL_TRANSITION_AFTER_PATCH" "$AVAIL_TRANSITION"

echo ""
echo "  Ready condition (${ADAPTER1} at gen1 ≠ resource gen2 → stays False, no flip):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)

assert_eq "Ready.status"              "False" "$READY_STATUS"
assert_eq "Ready.observed_generation" "2"     "$READY_OBSGEN"
assert_eq "Ready.last_transition_time preserved (no flip)" \
          "$READY_TRANSITION_AFTER_PATCH" "$READY_TRANSITION"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  With adapters at different generations the snapshot is inconsistent:"
echo "  • Available guard fires → all Available timestamps are frozen. The"
echo "    last-known-good True@gen1 is preserved until adapters converge."
echo "  • Ready stays False because ${ADAPTER1} is still at gen1, not the current gen2."
echo "    No status flip → LastTransitionTime is also preserved."
echo "  This is the expected in-flight state while a rolling reconciliation is"
echo "  in progress — adapters update one at a time."
echo ""
test_summary
