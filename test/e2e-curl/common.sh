#!/usr/bin/env bash
# common.sh — Shared helpers for HyperFleet status-aggregation e2e tests.
#
# Requires: curl, jq
# Server must be running with --enable-authz=false --enable-jwt=false
# and HYPERFLEET_CLUSTER_ADAPTERS='["dns","validation"]'

BASE_URL="${HYPERFLEET_URL:-http://localhost:8000}"
API="$BASE_URL/api/hyperfleet/v1"

# Required adapter names — must match HYPERFLEET_CLUSTER_ADAPTERS on the server.
# Override via env vars: ADAPTER1=dns ADAPTER2=validation ./run_all.sh
ADAPTER1="${ADAPTER1:-adapter1}"
ADAPTER2="${ADAPTER2:-adapter2}"

# ── Counters ────────────────────────────────────────────────────────────────
PASS_COUNT=0
FAIL_COUNT=0

# ── Colours (disabled when not a terminal) ──────────────────────────────────
if [ -t 1 ]; then
  RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
  BOLD='\033[1m'; NC='\033[0m'
else
  RED=''; GREEN=''; YELLOW=''; BOLD=''; NC=''
fi

# ── Time helpers ─────────────────────────────────────────────────────────────
now_rfc3339() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

# Unique 4-char hex suffix — cheap, no external deps
rand_hex4() {
  printf '%04x' $((RANDOM * RANDOM % 65536))
}

# ── Logging ──────────────────────────────────────────────────────────────────
log_step() { echo -e "  ${YELLOW}>${NC} $*"; }

# Print a feedback value received from a step (HTTP code, extracted field, etc.)
log_received() { echo "    → $*"; }

# Print a one-line Available+Ready state summary after a GET.
# Usage: show_state "label" "$CLUSTER_JSON"
show_state() {
  local label="$1" json="$2"
  local av_s av_g re_s re_g
  av_s=$(condition_field "$json" Available status)
  av_g=$(condition_field "$json" Available observed_generation)
  re_s=$(condition_field "$json" Ready status)
  re_g=$(condition_field "$json" Ready observed_generation)
  echo "    → ${label}: Available=${av_s}@gen${av_g}  Ready=${re_s}@gen${re_g}"
}

# ── Assertions ───────────────────────────────────────────────────────────────
assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    echo -e "    ${GREEN}PASS${NC} [$label]: '$actual'"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo -e "    ${RED}FAIL${NC} [$label]: expected='$expected' got='$actual'"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

assert_http() {
  local label="$1" expected="$2" actual="$3"
  assert_eq "HTTP $label" "$expected" "$actual"
}

# Asserts two values are NOT equal (e.g. timestamp was refreshed)
assert_changed() {
  local label="$1" before="$2" after="$3"
  if [ "$before" != "$after" ]; then
    echo -e "    ${GREEN}PASS${NC} [$label]: changed from '$before' to '$after'"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo -e "    ${RED}FAIL${NC} [$label]: expected change but value stayed '$after'"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

# Asserts a value is non-empty
assert_nonempty() {
  local label="$1" actual="$2"
  if [ -n "$actual" ] && [ "$actual" != "null" ]; then
    echo -e "    ${GREEN}PASS${NC} [$label]: '$actual'"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo -e "    ${RED}FAIL${NC} [$label]: value is empty or null"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

# ── API helpers ───────────────────────────────────────────────────────────────

# Create a cluster. Prints JSON on stdout, exits non-zero on failure.
create_cluster() {
  local name="$1"
  local spec="${2:-{\"v\":1}}"
  curl -sf -X POST "$API/clusters" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"Cluster\",\"name\":\"$name\",\"spec\":$spec}"
}

# Patch a cluster spec — bumps generation when spec content changes.
patch_cluster() {
  local id="$1"
  local spec="${2:-{\"v\":2}}"
  curl -sf -X PATCH "$API/clusters/$id" \
    -H "Content-Type: application/json" \
    -d "{\"spec\":$spec}"
}

# Get a single cluster. Prints JSON on stdout.
get_cluster() {
  local id="$1"
  curl -sf "$API/clusters/$id"
}

# Post adapter status. Returns the HTTP status code as a string ("201", "204", ...).
# Usage: code=$(post_adapter_status CLUSTER_ID ADAPTER GEN STATUS)
#   STATUS: True | False | Unknown
post_adapter_status() {
  local cluster_id="$1" adapter="$2" gen="$3" available="$4"
  curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$API/clusters/$cluster_id/statuses" \
    -H "Content-Type: application/json" \
    -d "{
      \"adapter\": \"$adapter\",
      \"observed_generation\": $gen,
      \"observed_time\": \"$(now_rfc3339)\",
      \"conditions\": [
        {\"type\": \"Available\", \"status\": \"$available\", \"reason\": \"Testing\"},
        {\"type\": \"Applied\",   \"status\": \"True\",       \"reason\": \"Testing\"},
        {\"type\": \"Health\",    \"status\": \"True\",       \"reason\": \"Testing\"}
      ]
    }"
}

# List adapter statuses for a cluster. Prints JSON on stdout.
get_statuses() {
  local cluster_id="$1"
  curl -sf "$API/clusters/$cluster_id/statuses"
}

# ── JSON field extraction ─────────────────────────────────────────────────────

# Extract a field from a specific condition type in cluster.status.conditions
# Usage: val=$(condition_field "$CLUSTER_JSON" Available status)
condition_field() {
  local json="$1" ctype="$2" field="$3"
  echo "$json" | jq -r ".status.conditions[] | select(.type == \"$ctype\") | .$field"
}

# Count adapter statuses stored for a cluster (by adapter name)
count_adapter_status() {
  local statuses_json="$1" adapter="$2"
  echo "$statuses_json" | jq "[.items[] | select(.adapter == \"$adapter\")] | length"
}

# ── Test summary ──────────────────────────────────────────────────────────────
test_summary() {
  local total=$((PASS_COUNT + FAIL_COUNT))
  echo ""
  if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}ALL PASSED${NC} ($PASS_COUNT/$total)"
    return 0
  else
    echo -e "${RED}${BOLD}FAILED${NC} ($FAIL_COUNT/$total failed)"
    return 1
  fi
}

# ── Preflight check ───────────────────────────────────────────────────────────
check_server() {
  if ! curl -sf "$BASE_URL/api/hyperfleet/v1/clusters?pageSize=1" > /dev/null 2>&1; then
    echo -e "${RED}ERROR${NC}: Cannot reach $BASE_URL"
    echo "Start the server: HYPERFLEET_CLUSTER_ADAPTERS='[\"dns\",\"validation\"]' make run-no-auth"
    exit 1
  fi
}
