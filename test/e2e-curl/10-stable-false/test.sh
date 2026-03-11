#!/usr/bin/env bash
# Test 10 — Stable False: re-report when both conditions are already False

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 10: Stable False re-evaluation — Available.LUT preserved, Ready.LUT refreshed${NC}"
echo ""
echo "  When a required adapter is missing the cluster stays Available=False and"
echo "  Ready=False. Re-reports from the present adapter must not churn Available"
echo "  timestamps (nothing changed). Ready.last_updated_time uses min(adapter LRTs);"
echo "  because ${ADAPTER1} has never reported, the fallback is 'now', so it refreshes."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=False@gen2, Ready=False@gen2"
echo -e "           ${YELLOW}         ${NC} (${ADAPTER1} missing, ${ADAPTER2}@gen2=True, cluster at gen2)"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER2}@gen2=True (${ADAPTER1} still missing)"
echo -e "  ${YELLOW}Expected:${NC}        Available=False unchanged (guard fires), Ready=False,"
echo -e "           ${YELLOW}         ${NC} Available.LUT preserved, Ready.LUT refreshed (now, ${ADAPTER1} absent)"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc10-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "Setup: POST ${ADAPTER2}@gen1=True  (${ADAPTER1} missing → False@rG1)"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True" > /dev/null
log_step "Setup: PATCH cluster → gen2"
patch_cluster "$CLUSTER_ID" '{"v":2}' > /dev/null
log_step "Setup: POST ${ADAPTER2}@gen2=True  (${ADAPTER1} still missing → False@rG2)"
post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 2 "True" > /dev/null

log_step "GET cluster — pre-event baseline (Available=False@gen2, Ready=False@gen2)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
BASELINE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_READY_UPDATED=$(condition_field "$BASELINE" Ready last_updated_time)
BASELINE_READY_TRANSITION=$(condition_field "$BASELINE" Ready last_transition_time)

assert_eq "baseline Available.status" "False" "$(condition_field "$BASELINE" Available status)"
assert_eq "baseline Available.obsgen" "2"     "$(condition_field "$BASELINE" Available observed_generation)"
assert_eq "baseline Ready.status"    "False" "$(condition_field "$BASELINE" Ready status)"
log_received "Available.LUT=$BASELINE_AVAIL_UPDATED"
log_received "Ready.LUT=$BASELINE_READY_UPDATED"

sleep 2

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER2}@gen2=True  (${ADAPTER1} still missing — re-evaluation)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 2 "True")
assert_http "${ADAPTER2} re-report accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post re-evaluation"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after re-evaluation" "$CLUSTER"

echo ""
echo "  Available condition (guard matches False@gen2→False@gen2 — timestamps frozen):"
assert_eq "Available.status"                         "False" "$(condition_field "$CLUSTER" Available status)"
assert_eq "Available.observed_generation"            "2"     "$(condition_field "$CLUSTER" Available observed_generation)"
assert_eq "Available.last_updated_time preserved"    "$BASELINE_AVAIL_UPDATED"    "$(condition_field "$CLUSTER" Available last_updated_time)"
assert_eq "Available.last_transition_time preserved" "$BASELINE_AVAIL_TRANSITION" "$(condition_field "$CLUSTER" Available last_transition_time)"

echo ""
echo "  Ready condition (last_updated_time = now when False — Sentinel 10s clock):"
assert_eq "Ready.status"              "False" "$(condition_field "$CLUSTER" Ready status)"
assert_eq "Ready.observed_generation" "2"     "$(condition_field "$CLUSTER" Ready observed_generation)"
assert_changed "Ready.last_updated_time refreshed (= min(LRTs); fallback=now since ${ADAPTER1} absent)" \
               "$BASELINE_READY_UPDATED" "$(condition_field "$CLUSTER" Ready last_updated_time)"
assert_eq "Ready.last_transition_time preserved (no flip)" \
          "$BASELINE_READY_TRANSITION" "$(condition_field "$CLUSTER" Ready last_transition_time)"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  Stable False re-evaluation is handled correctly:"
echo "  • Available: guard fires (same status False AND same ObsGen=2) — all"
echo "    Available timestamps are frozen. No spurious churn while waiting for"
echo "    ${ADAPTER1} to appear."
echo "  • Ready: last_updated_time = min(adapter LRTs). Because ${ADAPTER1} has"
echo "    never reported, the fallback applies and LUT is set to 'now'. When all"
echo "    required adapters have reported, LUT will reflect their earliest LRT."
echo "  • Neither LastTransitionTime changes — the status did not flip."
echo ""
test_summary
