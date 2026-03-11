#!/usr/bin/env bash
# run_all.sh — Execute all 12 status-aggregation e2e tests and report a summary.
#
# Usage:
#   ./run_all.sh              # runs all tests
#   ./run_all.sh 03 07        # runs only tests 03 and 07
#
# Prerequisites:
#   • Server running:  HYPERFLEET_CLUSTER_ADAPTERS='["dns","validation"]' make run-no-auth
#   • Tools:           curl, jq

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/common.sh"

echo -e "${BOLD}HyperFleet Status Aggregation — E2E Tests${NC}"
echo -e "Server: $BASE_URL"
echo -e "Required adapters: dns, validation"
echo ""

check_server

# ── Discover tests ────────────────────────────────────────────────────────────
TESTS=(
  "01-initial-state"
  "02-partial-adapters"
  "03-all-adapters-ready"
  "04-generation-bump"
  "05-mixed-generations"
  "06-stale-report"
  "07-all-adapters-new-gen"
  "08-adapter-goes-false"
  "09-stable-true"
  "10-stable-false"
  "11-unknown-subsequent"
  "12-unknown-first"
)

# Filter by argument prefixes if provided (e.g. "03" "07")
SELECTED=()
if [ $# -gt 0 ]; then
  for filter in "$@"; do
    for t in "${TESTS[@]}"; do
      [[ "$t" == "${filter}"* ]] && SELECTED+=("$t")
    done
  done
else
  SELECTED=("${TESTS[@]}")
fi

# ── Run each test ─────────────────────────────────────────────────────────────
TOTAL_PASS=0
TOTAL_FAIL=0
SUITE_FAIL=()

for test_name in "${SELECTED[@]}"; do
  test_file="$SCRIPT_DIR/$test_name/test.sh"
  if [ ! -f "$test_file" ]; then
    echo -e "${YELLOW}SKIP${NC} $test_name (no test.sh found)"
    continue
  fi

  echo -e "${BOLD}── $test_name${NC}"
  set +e
  bash "$test_file"
  rc=$?
  set -e
  sleep 1   # let the server drain before the next test

  # Read PASS_COUNT and FAIL_COUNT set by the child process via exported env.
  # Since child is a subshell we cannot inherit counters directly.
  # Instead each test.sh writes its result to stdout with the summary line.
  if [ $rc -eq 0 ]; then
    TOTAL_PASS=$((TOTAL_PASS + 1))
    echo ""
  else
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    SUITE_FAIL+=("$test_name")
    echo ""
  fi
done

# ── Suite summary ─────────────────────────────────────────────────────────────
echo "══════════════════════════════════════════════"
TOTAL=$((TOTAL_PASS + TOTAL_FAIL))
if [ "$TOTAL_FAIL" -eq 0 ]; then
  echo -e "${GREEN}${BOLD}ALL TESTS PASSED${NC} ($TOTAL_PASS/$TOTAL)"
  exit 0
else
  echo -e "${RED}${BOLD}$TOTAL_FAIL TEST(S) FAILED${NC} ($TOTAL_PASS/$TOTAL passed)"
  for f in "${SUITE_FAIL[@]}"; do
    echo -e "  ${RED}✗${NC} $f"
  done
  exit 1
fi
