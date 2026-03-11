#!/usr/bin/env bash
# Test 09 — Stable True: keep-alive re-reports when cluster is already Ready

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 09: Stable True re-evaluation — both Available and Ready.LUT refreshed${NC}"
echo ""
echo "  Adapters periodically re-report True to keep the cluster 'alive' in"
echo "  Sentinel's view. Per spec §5 (Available=True, Available=True row):"
echo "  lut = min(statuses[].lut) — both Available and Ready refresh their"
echo "  last_updated_time to the new min adapter LRT on every heartbeat."
echo "  last_transition_time is preserved for both (no status flip)."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=True@gen1, Ready=True@gen1"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER1}@gen1=True, POST ${ADAPTER2}@gen1=True (re-reports)"
echo -e "  ${YELLOW}Expected:${NC}        Available=True@gen1, Available.LUT refreshed (= new min LRT),"
echo -e "           ${YELLOW}         ${NC} Ready=True@gen1, Ready.LUT refreshed (= new min LRT),"
echo -e "           ${YELLOW}         ${NC} both LTTs preserved (no True↔False flip)"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc09-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "Setup: POST ${ADAPTER1}@gen1=True (initial report)"
post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True" > /dev/null
log_step "Setup: POST ${ADAPTER2}@gen1=True (initial report)"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True" > /dev/null

log_step "GET cluster — baseline (Ready=True@gen1)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
BASELINE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_READY_UPDATED=$(condition_field "$BASELINE" Ready last_updated_time)
BASELINE_READY_TRANSITION=$(condition_field "$BASELINE" Ready last_transition_time)
log_received "Available.LUT=$BASELINE_AVAIL_UPDATED"
log_received "Ready.LUT=$BASELINE_READY_UPDATED"

assert_eq "baseline Available.status" "True" "$(condition_field "$BASELINE" Available status)"
assert_eq "baseline Ready.status"     "True" "$(condition_field "$BASELINE" Ready    status)"

# Ensure new reports will have newer timestamps (2s for second-precision timestamps)
sleep 2

# ── Events ────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER1}@gen1=True (heartbeat re-report)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True")
assert_http "${ADAPTER1} re-report accepted" "201" "$CODE"

log_step "POST ${ADAPTER2}@gen1=True (heartbeat re-report)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True")
assert_http "${ADAPTER2} re-report accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post re-reports"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after re-reports" "$CLUSTER"

echo ""
echo "  Available condition (status and ObsGen unchanged, LUT advances to new min LRT):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)

assert_eq      "Available.status"                         "True" "$AVAIL_STATUS"
assert_eq      "Available.observed_generation"            "1"    "$AVAIL_OBSGEN"
assert_changed "Available.last_updated_time refreshed (= new min adapter LRT)" \
               "$BASELINE_AVAIL_UPDATED" "$AVAIL_UPDATED"
assert_eq      "Available.last_transition_time preserved" "$BASELINE_AVAIL_TRANSITION" "$AVAIL_TRANSITION"

echo ""
echo "  Ready condition (last_updated_time = min(LRT) — freshened by new reports):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_UPDATED=$(condition_field "$CLUSTER" Ready last_updated_time)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)

assert_eq "Ready.status"              "True" "$READY_STATUS"
assert_eq "Ready.observed_generation" "1"    "$READY_OBSGEN"
assert_changed "Ready.last_updated_time refreshed (= new min adapter LRT)" \
               "$BASELINE_READY_UPDATED" "$READY_UPDATED"
assert_eq "Ready.last_transition_time preserved (status stayed True, no flip)" \
          "$BASELINE_READY_TRANSITION" "$READY_TRANSITION"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  Heartbeat re-reports (same status, same generation) are handled correctly:"
echo "  • Available: per spec lut=min(statuses[].lut), so last_updated_time advances"
echo "    to the new min(adapter LRTs) on every heartbeat. last_transition_time is"
echo "    preserved because the status stayed True (no True↔False flip)."
echo "  • Ready: same rule — last_updated_time advances to the new min(adapter LRTs)."
echo "    This freshens Sentinel's staleness clock — if adapters stop reporting,"
echo "    Ready.LUT will eventually age past the 30-minute threshold and Sentinel"
echo "    will trigger a re-reconciliation."
echo "  • LastTransitionTime for both stays frozen — no status flip occurred."
echo ""
test_summary
