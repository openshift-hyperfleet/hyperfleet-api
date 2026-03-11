#!/usr/bin/env bash
# Test 07 — All adapters converge at gen2: Available ObsGen advances, Ready=True

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 07: All adapters converge at gen2 → Available@gen2, Ready=True${NC}"
echo ""
echo "  When all required adapters finally report True at the current generation"
echo "  the cluster is fully reconciled. Available's observed_generation must"
echo "  advance from gen1 to gen2 (status stays True, LTT is preserved because"
echo "  there is no True↔False flip). Ready flips from False to True."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=True@gen1, Ready=False@gen2"
echo -e "           ${YELLOW}         ${NC} (${ADAPTER1}@gen1=True, ${ADAPTER2}@gen2=True, cluster at gen2)"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER1}@gen2=True (now both adapters at gen2)"
echo -e "  ${YELLOW}Expected:${NC}        Available=True@gen2 (ObsGen advances, LTT preserved),"
echo -e "           ${YELLOW}         ${NC} Ready=True@gen2, Ready.LTT advances (False→True)"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc07-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "Setup: POST ${ADAPTER1}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True" > /dev/null
log_step "Setup: POST ${ADAPTER2}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True" > /dev/null
log_step "Setup: PATCH cluster → gen2"
patch_cluster "$CLUSTER_ID" '{"v":2}' > /dev/null
log_step "Setup: POST ${ADAPTER2}@gen2=True  (${ADAPTER1} still at gen1)"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 2 "True" > /dev/null

log_step "GET cluster — pre-event baseline (Available=True@gen1, Ready=False@gen2)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "pre-event" "$BASELINE"
BASELINE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_READY_TRANSITION=$(condition_field "$BASELINE" Ready last_transition_time)

assert_eq "baseline Available.status" "True"  "$(condition_field "$BASELINE" Available status)"
assert_eq "baseline Available.obsgen" "1"     "$(condition_field "$BASELINE" Available observed_generation)"
assert_eq "baseline Ready.status"    "False" "$(condition_field "$BASELINE" Ready    status)"

sleep 1

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER1}@gen2=True  (all adapters now at gen2 — snapshot consistent)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 2 "True")
assert_http "${ADAPTER1}@gen2 accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post-event"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after convergence" "$CLUSTER"

echo ""
echo "  Available condition (ObsGen advances gen1→gen2, status stays True):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)

assert_eq "Available.status"              "True" "$AVAIL_STATUS"
assert_eq "Available.observed_generation" "2"    "$AVAIL_OBSGEN"
assert_changed "Available.last_updated_time refreshed (ObsGen changed gen1→gen2)" \
               "$BASELINE_AVAIL_UPDATED" "$AVAIL_UPDATED"
assert_eq "Available.last_transition_time preserved (status stayed True, no flip)" \
          "$BASELINE_AVAIL_TRANSITION" "$AVAIL_TRANSITION"

echo ""
echo "  Ready condition (all adapters True@gen2 = current generation → flips to True):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)
READY_UPDATED=$(condition_field "$CLUSTER" Ready last_updated_time)

assert_eq "Ready.status"              "True" "$READY_STATUS"
assert_eq "Ready.observed_generation" "2"    "$READY_OBSGEN"
assert_changed "Ready.last_transition_time advanced (False→True)" \
               "$BASELINE_READY_TRANSITION" "$READY_TRANSITION"
assert_nonempty "Ready.last_updated_time (= min adapter LRT)" "$READY_UPDATED"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  When all adapters converge at the current generation:"
echo "  • Available's observed_generation advances from gen1 to gen2. The status"
echo "    stays True (no True↔False flip) so LastTransitionTime is preserved."
echo "    LastUpdatedTime refreshes because the observed_generation changed."
echo "  • Ready flips to True (False→True) because all adapters are now at gen2."
echo "    LastTransitionTime advances. LastUpdatedTime = min(adapter LRTs) — the"
echo "    earliest of the two adapter report times, not the wall clock."
echo "  The cluster is now fully reconciled to gen2."
echo ""
test_summary
