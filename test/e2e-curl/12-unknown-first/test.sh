#!/usr/bin/env bash
# Test 12 — Unknown first report: P3 rule discards ALL Unknown reports (204)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 12: Unknown report (first or subsequent) → always discarded (204)${NC}"
echo ""
echo "  The P3 rule discards every report where Available=Unknown — including"
echo "  the very first report from an adapter that has never reported before."
echo "  No adapter status is persisted. No aggregation runs. Cluster conditions"
echo "  stay at their initial False@gen1 state. A second Unknown is also discarded."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Fresh cluster, gen=1, no adapter statuses at all"
echo -e "  ${YELLOW}Event 1:${NC}         POST ${ADAPTER2}@gen1=Unknown (first-ever report)"
echo -e "  ${YELLOW}Event 2:${NC}         POST ${ADAPTER2}@gen1=Unknown (second report)"
echo -e "  ${YELLOW}Expected:${NC}        Both return 204; no adapter status stored;"
echo -e "           ${YELLOW}         ${NC} cluster conditions unchanged from initial state"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc12-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "GET cluster — initial state (no adapter statuses)"
INITIAL=$(get_cluster "$CLUSTER_ID")
show_state "initial" "$INITIAL"
INIT_AVAIL_STATUS=$(condition_field "$INITIAL" Available status)
INIT_AVAIL_OBSGEN=$(condition_field "$INITIAL" Available observed_generation)
INIT_AVAIL_UPDATED=$(condition_field "$INITIAL" Available last_updated_time)
INIT_AVAIL_TRANSITION=$(condition_field "$INITIAL" Available last_transition_time)
INIT_READY_STATUS=$(condition_field "$INITIAL" Ready status)
INIT_READY_OBSGEN=$(condition_field "$INITIAL" Ready observed_generation)
INIT_READY_UPDATED=$(condition_field "$INITIAL" Ready last_updated_time)
INIT_READY_TRANSITION=$(condition_field "$INITIAL" Ready last_transition_time)

assert_eq "initial Available.status" "False" "$INIT_AVAIL_STATUS"
assert_eq "initial Ready.status"     "False" "$INIT_READY_STATUS"
log_received "Available.LUT=$INIT_AVAIL_UPDATED"

# ── Event 1: first-ever Unknown report ────────────────────────────────────────
log_step "POST ${ADAPTER2}@gen1=Unknown  (first-ever report — P3 rule applies)"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "Unknown")

echo ""
echo "  API response (P3 discards ALL Unknown, including the first-ever report):"
assert_http "first Unknown discarded (204)" "204" "$CODE"

# ── Validate: no adapter status stored ────────────────────────────────────────
log_step "GET /statuses — adapter record must NOT exist"
STATUSES=$(get_statuses "$CLUSTER_ID")
VAL_COUNT=$(count_adapter_status "$STATUSES" "$ADAPTER2")

echo ""
echo "  Adapter status persistence (report was discarded — nothing stored):"
assert_eq "${ADAPTER2} status record NOT stored" "0" "$VAL_COUNT"

# ── Validate: cluster conditions completely unchanged ─────────────────────────
log_step "GET cluster — conditions must be byte-identical to initial state"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after first Unknown" "$CLUSTER"

echo ""
echo "  Available condition (unchanged — P3 discarded before any write):"
assert_eq "Available.status"               "$INIT_AVAIL_STATUS"     "$(condition_field "$CLUSTER" Available status)"
assert_eq "Available.observed_generation"  "$INIT_AVAIL_OBSGEN"     "$(condition_field "$CLUSTER" Available observed_generation)"
assert_eq "Available.last_updated_time"    "$INIT_AVAIL_UPDATED"    "$(condition_field "$CLUSTER" Available last_updated_time)"
assert_eq "Available.last_transition_time" "$INIT_AVAIL_TRANSITION" "$(condition_field "$CLUSTER" Available last_transition_time)"

echo ""
echo "  Ready condition (unchanged):"
assert_eq "Ready.status"               "$INIT_READY_STATUS"     "$(condition_field "$CLUSTER" Ready status)"
assert_eq "Ready.observed_generation"  "$INIT_READY_OBSGEN"     "$(condition_field "$CLUSTER" Ready observed_generation)"
assert_eq "Ready.last_updated_time"    "$INIT_READY_UPDATED"    "$(condition_field "$CLUSTER" Ready last_updated_time)"
assert_eq "Ready.last_transition_time" "$INIT_READY_TRANSITION" "$(condition_field "$CLUSTER" Ready last_transition_time)"

# ── Event 2: second Unknown report ────────────────────────────────────────────
echo ""
log_step "POST ${ADAPTER2}@gen1=Unknown  (second report — also discarded)"
CODE2=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "Unknown")

echo ""
echo "  API response (second Unknown also discarded):"
assert_http "second Unknown discarded (204)" "204" "$CODE2"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  The P3 rule applies uniformly to ALL Available=Unknown reports:"
echo "  • Both the first and second Unknown reports returned 204"
echo "  • No adapter status was written to the database (count=0)"
echo "  • No aggregation ran — cluster conditions are byte-identical to initial state"
echo "  Unknown means the adapter hasn't determined its state yet. The cluster"
echo "  should not be affected at all — not even by a first-time adapter report."
echo ""
test_summary
