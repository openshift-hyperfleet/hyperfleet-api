#!/usr/bin/env bash
# Test 01 — Initial state: cluster created, no adapter reports

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 01: Initial state — no adapter statuses${NC}"
echo ""
echo "  A freshly created cluster must immediately expose Available=False and"
echo "  Ready=False at generation 1 with valid timestamps. No adapter reports"
echo "  have been posted — the API must bootstrap these conditions itself."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Cluster just created, gen=1, no adapter statuses"
echo -e "  ${YELLOW}Event:${NC}           GET cluster immediately after creation (no events)"
echo -e "  ${YELLOW}Expected:${NC}        Available=False@gen1, Ready=False@gen1, timestamps set"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc01-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
GENERATION=$(echo "$CLUSTER" | jq -r '.generation')
log_received "cluster id=$CLUSTER_ID  generation=$GENERATION"

# ── Event: GET immediately (no adapter statuses posted) ───────────────────────
log_step "GET cluster — no adapter statuses have been posted"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "received" "$CLUSTER"

# ── Validate ──────────────────────────────────────────────────────────────────
echo ""
echo "  Available condition:"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)

assert_eq "Available.status"              "False" "$AVAIL_STATUS"
assert_eq "Available.observed_generation" "1"     "$AVAIL_OBSGEN"
assert_nonempty "Available.last_updated_time"      "$AVAIL_UPDATED"
assert_nonempty "Available.last_transition_time"   "$AVAIL_TRANSITION"

echo ""
echo "  Ready condition:"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_UPDATED=$(condition_field "$CLUSTER" Ready last_updated_time)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)

assert_eq "Ready.status"              "False" "$READY_STATUS"
assert_eq "Ready.observed_generation" "1"     "$READY_OBSGEN"
assert_nonempty "Ready.last_updated_time"      "$READY_UPDATED"
assert_nonempty "Ready.last_transition_time"   "$READY_TRANSITION"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  The API correctly bootstraps both synthetic conditions at resource creation:"
echo "  • Available=False because no adapter has reported yet — cannot be available"
echo "  • Ready=False for the same reason — no adapter at current generation"
echo "  • observed_generation=1 matches the cluster's creation generation"
echo "  • Both timestamps are non-empty, establishing a baseline for future"
echo "    change-detection checks (assert_changed relies on these being set)"
echo ""
test_summary
