#!/usr/bin/env bash
# Test 08 — Adapter goes False: one required adapter reports False → both conditions drop

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 08: One required adapter goes False → Available=False, Ready=False${NC}"
echo ""
echo "  When a required adapter reports Available=False the cluster can no longer"
echo "  be considered available or ready. Both conditions must flip to False and"
echo "  their LastTransitionTime values must advance (True→False). The timestamps"
echo "  reflect the adapter's observed_time, not the server's wall clock."
echo "  Available.observed_generation must be the resource generation, NOT zero."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=True@gen1, Ready=True@gen1 (both adapters True)"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER1}@gen1=False"
echo -e "  ${YELLOW}Expected:${NC}        Available=False@rG1 (rG not zero), Ready=False@rG1,"
echo -e "           ${YELLOW}         ${NC} both LTT advance (True→False), both LUT refreshed"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc08-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
RESOURCE_GEN=$(echo "$CLUSTER" | jq -r '.generation')
log_received "cluster id=$CLUSTER_ID  generation=$RESOURCE_GEN"

log_step "Setup: POST ${ADAPTER1}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True" > /dev/null
log_step "Setup: POST ${ADAPTER2}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True" > /dev/null

log_step "GET cluster — baseline (both adapters True → cluster Ready)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
BASELINE_READY_TRANSITION=$(condition_field "$BASELINE" Ready last_transition_time)
BASELINE_READY_UPDATED=$(condition_field "$BASELINE" Ready last_updated_time)

assert_eq "baseline Available.status" "True" "$(condition_field "$BASELINE" Available status)"
assert_eq "baseline Ready.status"     "True" "$(condition_field "$BASELINE" Ready    status)"

sleep 2

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER1}@gen1=False  (one required adapter reports a problem)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "False")
assert_http "${ADAPTER1} False report accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post-event"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after ${ADAPTER1}=False" "$CLUSTER"

echo ""
echo "  Available condition (False; observed_generation=rG=$RESOURCE_GEN, NOT zero):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)

assert_eq "Available.status"                          "False"         "$AVAIL_STATUS"
assert_eq "Available.observed_generation (=rG not 0)" "$RESOURCE_GEN" "$AVAIL_OBSGEN"
assert_changed "Available.last_transition_time advanced (True→False)" \
               "$BASELINE_AVAIL_TRANSITION" "$AVAIL_TRANSITION"
assert_changed "Available.last_updated_time refreshed (= observed_time)" \
               "$BASELINE_AVAIL_UPDATED" "$AVAIL_UPDATED"

echo ""
echo "  Ready condition (one adapter False → also drops to False):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)
READY_UPDATED=$(condition_field "$CLUSTER" Ready last_updated_time)

assert_eq "Ready.status"               "False"         "$READY_STATUS"
assert_eq "Ready.observed_generation"  "$RESOURCE_GEN" "$READY_OBSGEN"
assert_changed "Ready.last_transition_time advanced (True→False)" \
               "$BASELINE_READY_TRANSITION" "$READY_TRANSITION"
assert_changed "Ready.last_updated_time refreshed" \
               "$BASELINE_READY_UPDATED" "$READY_UPDATED"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  A single False report from any required adapter immediately drops both"
echo "  Available and Ready to False:"
echo "  • Available=False@rG1: the observed_generation is set to the resource"
echo "    generation (gen=1), NOT zero. This is an explicit spec requirement."
echo "  • Both LastTransitionTime values advance — the status flipped True→False."
echo "  • Both LastUpdatedTime values are refreshed to the adapter's observed_time."
echo "    This is the timestamp when the problem was first detected, not when the"
echo "    server processed the report."
echo ""
test_summary
