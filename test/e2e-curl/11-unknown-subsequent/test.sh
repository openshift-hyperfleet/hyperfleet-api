#!/usr/bin/env bash
# Test 11 — Unknown on subsequent report: always discarded (P3 rule)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 11: Unknown on subsequent report → discarded (204), state unchanged${NC}"
echo ""
echo "  The P3 rule discards any adapter report where Available=Unknown. This"
echo "  applies to ALL reports — first, subsequent, at any generation. When a"
echo "  prior adapter status already exists, the Unknown report must not overwrite"
echo "  it and must not trigger aggregation. The stored True value is preserved."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  ${ADAPTER2} stored with Available=True; cluster Ready=True"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER2}@gen1=Unknown (prior True status exists)"
echo -e "  ${YELLOW}Expected:${NC}        HTTP 204, all cluster conditions byte-identical to baseline,"
echo -e "           ${YELLOW}         ${NC} stored adapter status retains its prior True value"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc11-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "Setup: POST ${ADAPTER2}@gen1=True (establish prior status)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True")
assert_http "initial ${ADAPTER2} True accepted" "201" "$CODE"

log_step "Setup: POST ${ADAPTER1}@gen1=True"
post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True" > /dev/null

log_step "GET cluster — baseline (both adapters True → cluster Ready)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
PRE_AVAIL_STATUS=$(condition_field "$BASELINE" Available status)
PRE_AVAIL_OBSGEN=$(condition_field "$BASELINE" Available observed_generation)
PRE_AVAIL_UPDATED=$(condition_field "$BASELINE" Available last_updated_time)
PRE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
PRE_READY_STATUS=$(condition_field "$BASELINE" Ready status)
PRE_READY_OBSGEN=$(condition_field "$BASELINE" Ready observed_generation)
PRE_READY_UPDATED=$(condition_field "$BASELINE" Ready last_updated_time)
PRE_READY_TRANSITION=$(condition_field "$BASELINE" Ready last_transition_time)
log_received "Available=$PRE_AVAIL_STATUS  Ready=$PRE_READY_STATUS"

sleep 1

# ── Event ─────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER2}@gen1=Unknown  (P3 rule — should be discarded)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "Unknown")

echo ""
echo "  API response (P3 rule must discard Unknown and return 204):"
assert_http "Unknown report discarded" "204" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — all fields must be byte-identical to baseline"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after Unknown" "$CLUSTER"

echo ""
echo "  Available condition (must be identical to baseline — no aggregation ran):"
assert_eq "Available.status"               "$PRE_AVAIL_STATUS"     "$(condition_field "$CLUSTER" Available status)"
assert_eq "Available.observed_generation"  "$PRE_AVAIL_OBSGEN"     "$(condition_field "$CLUSTER" Available observed_generation)"
assert_eq "Available.last_updated_time"    "$PRE_AVAIL_UPDATED"    "$(condition_field "$CLUSTER" Available last_updated_time)"
assert_eq "Available.last_transition_time" "$PRE_AVAIL_TRANSITION" "$(condition_field "$CLUSTER" Available last_transition_time)"

echo ""
echo "  Ready condition (must be identical to baseline):"
assert_eq "Ready.status"               "$PRE_READY_STATUS"     "$(condition_field "$CLUSTER" Ready status)"
assert_eq "Ready.observed_generation"  "$PRE_READY_OBSGEN"     "$(condition_field "$CLUSTER" Ready observed_generation)"
assert_eq "Ready.last_updated_time"    "$PRE_READY_UPDATED"    "$(condition_field "$CLUSTER" Ready last_updated_time)"
assert_eq "Ready.last_transition_time" "$PRE_READY_TRANSITION" "$(condition_field "$CLUSTER" Ready last_transition_time)"

echo ""
echo "  Stored adapter status retains its prior True value (not overwritten):"
STATUSES=$(get_statuses "$CLUSTER_ID")
VAL_STATUS=$(echo "$STATUSES" | jq -r --arg a "$ADAPTER2" \
  '.items[] | select(.adapter==$a) | .conditions[] | select(.type=="Available") | .status')
assert_eq "${ADAPTER2} stored Available still True" "True" "$VAL_STATUS"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  The P3 rule discarded the Unknown report before any state was changed:"
echo "  • The server returned 204 — no adapter status was persisted"
echo "  • The stored adapter status still shows Available=True (unchanged)"
echo "  • All cluster condition fields are byte-identical to the pre-event snapshot"
echo "  • No aggregation was triggered — Ready.LUT was not refreshed"
echo "  Unknown means the adapter doesn't yet know its state. The cluster should"
echo "  not regress from a known-good state just because an adapter is uncertain."
echo ""
test_summary
