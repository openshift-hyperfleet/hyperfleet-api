#!/usr/bin/env bash
# Test 06 — Stale report: ObsGen(new) < ObsGen(existing) → silently discarded

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 06: Stale report — ObsGen(new) < ObsGen(existing) → DISCARD (204)${NC}"
echo ""
echo "  If an adapter sends a report for an observed generation that is older"
echo "  than what is already stored, the server must silently discard it. The"
echo "  report carries no new information and must not overwrite newer state."
echo "  All cluster conditions must remain completely unchanged."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  ${ADAPTER2} stored at gen2; cluster at gen2"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER2}@gen1=False (stale — gen1 < stored gen2)"
echo -e "  ${YELLOW}Expected:${NC}        HTTP 204, ALL condition fields byte-identical to pre-event"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc06-$(rand_hex4)"
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
log_step "Setup: POST ${ADAPTER2}@gen2=True  (${ADAPTER2} now stored at gen2)"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 2 "True" > /dev/null

log_step "GET cluster — pre-stale baseline"
PRE=$(get_cluster "$CLUSTER_ID")
show_state "pre-stale" "$PRE"
PRE_AVAIL_STATUS=$(condition_field "$PRE" Available status)
PRE_AVAIL_OBSGEN=$(condition_field "$PRE" Available observed_generation)
PRE_AVAIL_UPDATED=$(condition_field "$PRE" Available last_updated_time)
PRE_AVAIL_TRANSITION=$(condition_field "$PRE" Available last_transition_time)
PRE_READY_STATUS=$(condition_field "$PRE" Ready status)
PRE_READY_OBSGEN=$(condition_field "$PRE" Ready observed_generation)
PRE_READY_UPDATED=$(condition_field "$PRE" Ready last_updated_time)
PRE_READY_TRANSITION=$(condition_field "$PRE" Ready last_transition_time)
log_received "Available=$PRE_AVAIL_STATUS@obsgen$PRE_AVAIL_OBSGEN  Ready=$PRE_READY_STATUS@obsgen$PRE_READY_OBSGEN"

sleep 1

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER2}@gen1=False  (STALE — gen1 < stored gen2)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "False")

echo ""
echo "  API response (stale detection must return 204 without touching state):"
assert_http "stale report discarded" "204" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — all fields must be byte-identical to pre-event snapshot"
POST=$(get_cluster "$CLUSTER_ID")
show_state "after stale" "$POST"

echo ""
echo "  Available condition (must be identical to pre-stale snapshot):"
assert_eq "Available.status"               "$PRE_AVAIL_STATUS"     "$(condition_field "$POST" Available status)"
assert_eq "Available.observed_generation"  "$PRE_AVAIL_OBSGEN"     "$(condition_field "$POST" Available observed_generation)"
assert_eq "Available.last_updated_time"    "$PRE_AVAIL_UPDATED"    "$(condition_field "$POST" Available last_updated_time)"
assert_eq "Available.last_transition_time" "$PRE_AVAIL_TRANSITION" "$(condition_field "$POST" Available last_transition_time)"

echo ""
echo "  Ready condition (must be identical to pre-stale snapshot):"
assert_eq "Ready.status"               "$PRE_READY_STATUS"     "$(condition_field "$POST" Ready status)"
assert_eq "Ready.observed_generation"  "$PRE_READY_OBSGEN"     "$(condition_field "$POST" Ready observed_generation)"
assert_eq "Ready.last_updated_time"    "$PRE_READY_UPDATED"    "$(condition_field "$POST" Ready last_updated_time)"
assert_eq "Ready.last_transition_time" "$PRE_READY_TRANSITION" "$(condition_field "$POST" Ready last_transition_time)"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  The stale-detection rule (observed_generation < stored_generation) fired:"
echo "  • The server returned 204 without persisting the report or running aggregation"
echo "  • Every Available and Ready field is byte-identical to the pre-event snapshot"
echo "  This prevents adapters from accidentally 'going back in time' — a late-"
echo "  arriving network packet from a previous reconciliation cycle cannot corrupt"
echo "  the current state."
echo ""
test_summary
