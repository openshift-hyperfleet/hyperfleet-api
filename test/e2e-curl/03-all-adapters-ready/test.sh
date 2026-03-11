#!/usr/bin/env bash
# Test 03 — All required adapters True at gen=1 → both conditions flip to True

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common.sh"

# ── Purpose ───────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}Test 03: All required adapters True at gen=1 → Available=True, Ready=True${NC}"
echo ""
echo "  When all required adapters report Available=True at the same generation,"
echo "  both synthetic conditions must flip to True and their LastTransitionTime"
echo "  values must advance (False→True). Ready.last_updated_time is set to the"
echo "  earliest adapter LastReportTime (not the wall clock)."
echo ""
echo -e "  ${YELLOW}Starting state:${NC}  Available=False@gen1, Ready=False@gen1 (initial)"
echo -e "  ${YELLOW}Event:${NC}           POST ${ADAPTER1}@gen1=True, POST ${ADAPTER2}@gen1=True"
echo -e "  ${YELLOW}Expected:${NC}        Available=True@gen1, Ready=True@gen1,"
echo -e "           ${YELLOW}         ${NC} both LTT advance (False→True flip)"
echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────
CLUSTER_NAME="tc03-$(rand_hex4)"
log_step "Creating cluster '$CLUSTER_NAME'"
CLUSTER=$(create_cluster "$CLUSTER_NAME")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
log_received "cluster id=$CLUSTER_ID"

log_step "GET cluster — baseline (before any adapter reports)"
BASELINE=$(get_cluster "$CLUSTER_ID")
show_state "baseline" "$BASELINE"
BASELINE_AVAIL_TRANSITION=$(condition_field "$BASELINE" Available last_transition_time)
BASELINE_READY_TRANSITION=$(condition_field "$BASELINE" Ready    last_transition_time)

# ── Events ────────────────────────────────────────────────────────────────────
log_step "POST ${ADAPTER1}@gen1=True"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER1" 1 "True")
assert_http "${ADAPTER1} report accepted" "201" "$CODE"

log_step "POST ${ADAPTER2}@gen1=True"
CODE=$(post_adapter_status "$CLUSTER_ID" "$ADAPTER2" 1 "True")
assert_http "${ADAPTER2} report accepted" "201" "$CODE"

# ── Validate ──────────────────────────────────────────────────────────────────
log_step "GET cluster — post-event"
CLUSTER=$(get_cluster "$CLUSTER_ID")
show_state "after both adapters True" "$CLUSTER"

echo ""
echo "  Available condition (all required adapters True@gen1 — any-gen semantics):"
AVAIL_STATUS=$(condition_field "$CLUSTER" Available status)
AVAIL_OBSGEN=$(condition_field "$CLUSTER" Available observed_generation)
AVAIL_TRANSITION=$(condition_field "$CLUSTER" Available last_transition_time)
AVAIL_UPDATED=$(condition_field "$CLUSTER" Available last_updated_time)

assert_eq "Available.status"              "True" "$AVAIL_STATUS"
assert_eq "Available.observed_generation" "1"    "$AVAIL_OBSGEN"
assert_changed "Available.last_transition_time advanced (False→True)" \
               "$BASELINE_AVAIL_TRANSITION" "$AVAIL_TRANSITION"
assert_nonempty "Available.last_updated_time (= min LRT)"             "$AVAIL_UPDATED"

echo ""
echo "  Ready condition (all adapters True at current generation gen=1):"
READY_STATUS=$(condition_field "$CLUSTER" Ready status)
READY_OBSGEN=$(condition_field "$CLUSTER" Ready observed_generation)
READY_TRANSITION=$(condition_field "$CLUSTER" Ready last_transition_time)
READY_UPDATED=$(condition_field "$CLUSTER" Ready last_updated_time)

assert_eq "Ready.status"              "True" "$READY_STATUS"
assert_eq "Ready.observed_generation" "1"    "$READY_OBSGEN"
assert_changed "Ready.last_transition_time advanced (False→True)" \
               "$BASELINE_READY_TRANSITION" "$READY_TRANSITION"
assert_nonempty "Ready.last_updated_time (= min LRT)"             "$READY_UPDATED"

echo ""
echo "  Adapter statuses persisted in the database:"
STATUSES=$(get_statuses "$CLUSTER_ID")
assert_eq "${ADAPTER1} status stored" "1" "$(count_adapter_status "$STATUSES" "$ADAPTER1")"
assert_eq "${ADAPTER2} status stored" "1" "$(count_adapter_status "$STATUSES" "$ADAPTER2")"

# ── Conclusion ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Conclusion:${NC}"
echo "  With all required adapters True at a consistent generation (gen=1):"
echo "  • Available flips to True@gen1 — any-gen semantics: all adapters at the"
echo "    same gen and all True is sufficient, regardless of the resource generation"
echo "  • Ready flips to True@gen1 — current-gen semantics: all adapters at the"
echo "    current resource generation (also gen=1 here)"
echo "  • Both LastTransitionTime values advance — status changed False→True"
echo "  • Ready.last_updated_time = min(adapter LastReportTimes), not wall clock"
echo ""
test_summary
