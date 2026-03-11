#!/usr/bin/env bash
# Test 04 — Generation bump: spec updated, adapters still at old generation

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 04: Generation bump — adapters at gen1, cluster bumps to gen2${NC}"
echo ""
echo "  When a cluster's spec is updated its generation increments. Adapters"
echo "  are still reporting for the old generation. Ready must immediately drop"
echo "  to False (adapters not caught up). Available must preserve its last-known-"
echo "  good value — the spec says the cluster was fine before the change."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=True@gen1, Ready=True@gen1"
echo -e "  ${YELLOW}Event:${NC}           PATCH cluster spec (generation bumps gen1→gen2)"
echo -e "  ${YELLOW}Expected:${NC}        Available=True@gen1 preserved (any-gen semantics),"
echo -e "           ${YELLOW}         ${NC} Ready=False@gen2, Ready.LTT advances (True→False)"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc04-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "Setup: POST ${ADAPTER1}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True" > /dev/null

log_step "Setup: POST ${ADAPTER2}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True" > /dev/null

log_step "GET cluster — baseline (Ready=True@gen1)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
BASELINE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_READY_TRANSITION=$(condition_field "$BASELINE" Ready last_transition_time)

assert_eq "baseline Available.status" "True" "$(condition_field "$BASELINE" Available status)"
assert_eq "baseline Ready.status"     "True" "$(condition_field "$BASELINE" Ready    status)"

sleep 2

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "PATCH cluster spec → generation bumps to 2 (adapters still at gen1)"
PATCHED=$(patch_cluster "$CLUSTER_ID" '{"v":2,"bumped":true}')
NEW_GEN=$(echo "$PATCHED" | jq -r '.generation')
log_received "new generation=$NEW_GEN"
assert_eq "generation bumped to 2" "2" "$NEW_GEN"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post-patch"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after generation bump" "$CLUSTER"

echo ""
echo "  Available condition (adapters still True@gen1 — last-known-good preserved):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)

assert_eq "Available.status"                         "True" "$AVAIL_STATUS"
assert_eq "Available.observed_generation"            "1"    "$AVAIL_OBSGEN"
assert_eq "Available.last_updated_time preserved"    "$BASELINE_AVAIL_UPDATED"    "$AVAIL_UPDATED"
assert_eq "Available.last_transition_time preserved" "$BASELINE_AVAIL_TRANSITION" "$AVAIL_TRANSITION"

echo ""
echo "  Ready condition (adapters haven't caught up to gen2 → drops to False):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)

assert_eq "Ready.status"              "False" "$READY_STATUS"
assert_eq "Ready.observed_generation" "2"     "$READY_OBSGEN"
assert_changed "Ready.last_transition_time advanced (True→False)" \
               "$BASELINE_READY_TRANSITION" "$READY_TRANSITION"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  A spec update immediately makes the cluster 'not ready' — adapters must"
echo "  reconcile the new spec before Ready can return to True."
echo "  • Available=True@gen1 preserved: any-gen semantics allow the last-known-"
echo "    good state to persist. The cluster was available before the change."
echo "  • Ready=False@gen2: adapters are still at gen1, not caught up to gen2."
echo "    LastTransitionTime advances because the status flipped True→False."
echo "  This is the key safety property: spec changes put the cluster into a"
echo "  'pending reconciliation' state immediately, without waiting for adapters."
echo ""
test_summary
