#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="charts"
RELEASE_NAME="test-release"

KUBECONFORM="${KUBECONFORM:-kubeconform}"
YQ="${YQ:-yq}"
KUBECONFORM_FLAGS=(
  -strict
  -kubernetes-version 1.30.0
  -schema-location default
  -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
)

PASSED=0
FAILED=0
SCENARIO_FAILED=0

DEFAULT_SETS=(
  --set image.registry=quay.io
  --set image.repository=openshift-hyperfleet/hyperfleet-api
  --set image.tag=test
  --set 'adapters.cluster=["validation"]'
  --set 'adapters.nodepool=["validation"]'
)

render() {
  helm template "$RELEASE_NAME" "$CHART_DIR" "${DEFAULT_SETS[@]}" "$@"
}

kubeconform_validate() {
  "$KUBECONFORM" "${KUBECONFORM_FLAGS[@]}" "$@"
}

extract_config_yaml() {
  sed -n '/config.yaml: |/,/^[^ ]/{ /config.yaml: |/d; /^[^ ]/d; s/^    //; p; }'
}

validate_yaml() {
  "$YQ" eval '.' - > /dev/null
}

pass() {
  if [ "$SCENARIO_FAILED" -eq 0 ]; then
    echo "  ✓ $1"
    PASSED=$((PASSED + 1))
  fi
}

fail() {
  echo "  ✗ FAIL: $1"
  FAILED=$((FAILED + 1))
  SCENARIO_FAILED=1
}

run_test() {
  SCENARIO_FAILED=0
  echo ""
  echo "Testing $1..."
}

assert_contains() {
  local input="$1" pattern="$2" msg="$3"
  if ! echo "$input" | grep -q "$pattern"; then
    fail "$msg"
  fi
}

assert_not_contains() {
  local input="$1" pattern="$2" msg="$3"
  if echo "$input" | grep -q "$pattern"; then
    fail "$msg"
  fi
}

# ─── Pre-flight checks ───────────────────────────────────────────────

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Testing Helm charts..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if ! command -v helm > /dev/null; then
  echo "Error: helm not found. Please install Helm:"
  echo "  brew install helm  # macOS"
  echo "  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash  # Linux"
  exit 1
fi

# ─── Lint ─────────────────────────────────────────────────────────────

echo ""
echo "Linting Helm chart..."
helm lint "$CHART_DIR" "${DEFAULT_SETS[@]}"

# ─── Template rendering tests ────────────────────────────────────────

run_test "template rendering with default values"
render | kubeconform_validate
pass "Default values template"

run_test "template with external database"
render \
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true \
  --set database.external.secretName=my-db-secret | kubeconform_validate
pass "External database config template"

run_test "template with autoscaling"
render \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=5 | kubeconform_validate
pass "Autoscaling config template"

run_test "template with PDB enabled"
render \
  --set podDisruptionBudget.enabled=true \
  --set podDisruptionBudget.minAvailable=1 | kubeconform_validate
pass "PDB config template"

run_test "template with ServiceMonitor enabled"
render \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.interval=15s | kubeconform_validate
pass "ServiceMonitor config template"

run_test "template with PodMonitoring enabled"
render \
  --set monitoring.podMonitoring.enabled=true \
  --set monitoring.podMonitoring.interval=15s | kubeconform_validate -ignore-missing-schemas
pass "PodMonitoring config template"

run_test "template with PodMonitoring and TLS enabled"
render \
  --set monitoring.podMonitoring.enabled=true \
  --set config.metrics.tls.enabled=true \
  --set monitoring.podMonitoring.tlsConfig.insecureSkipVerify=true | kubeconform_validate -ignore-missing-schemas
pass "PodMonitoring with TLS config template"

run_test "template with auth disabled"
render --set config.server.jwt.enabled=false | kubeconform_validate
pass "Auth disabled config template"

run_test "template with custom image"
helm template "$RELEASE_NAME" "$CHART_DIR" \
  --set 'adapters.cluster=["validation"]' \
  --set 'adapters.nodepool=["validation"]' \
  --set image.registry=quay.io \
  --set image.repository=myorg/hyperfleet-api \
  --set image.tag=v1.0.0 | kubeconform_validate
pass "Custom image config template"

run_test "template with sidecar injection"
render \
  --set-json 'sidecars=[{"name":"test-sidecar","image":"busybox:1.36","command":["sleep","infinity"]}]' | kubeconform_validate
pass "Sidecar injection config template"

run_test "template with native sidecar injection"
OUTPUT=$(render \
  --set-json 'nativeSidecars=[{"name":"cloud-sql-proxy","restartPolicy":"Always","image":"gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.14.3","args":["--structured-logs","--port=5432","project:region:instance"]}]')
assert_contains "$OUTPUT" 'name: cloud-sql-proxy' "cloud-sql-proxy not found in rendered output"
PROXY_LINE=$(echo "$OUTPUT" | grep -n 'name: cloud-sql-proxy' | head -1 | cut -d: -f1)
MIGRATE_LINE=$(echo "$OUTPUT" | grep -n 'name: db-migrate' | head -1 | cut -d: -f1)
if [ "$PROXY_LINE" -ge "$MIGRATE_LINE" ]; then
  fail "cloud-sql-proxy must appear before db-migrate"
else
  echo "$OUTPUT" | kubeconform_validate
  pass "Native sidecar injection config template"
fi

run_test "template with native sidecars and no database"
OUTPUT=$(render \
  --set database.postgresql.enabled=false \
  --set-json 'nativeSidecars=[{"name":"test-proxy","restartPolicy":"Always","image":"busybox:1.36","command":["sleep","infinity"]}]')
assert_contains "$OUTPUT" 'name: test-proxy' "test-proxy not found in rendered output"
assert_not_contains "$OUTPUT" 'name: wait-for-db' "wait-for-db should not appear when no database is configured"
assert_not_contains "$OUTPUT" 'name: db-migrate' "db-migrate should not appear when no database is configured"
echo "$OUTPUT" | kubeconform_validate
pass "Native sidecar without database config template"

run_test "template with full adapter config"
helm template "$RELEASE_NAME" "$CHART_DIR" \
  --set image.registry=quay.io \
  --set image.repository=openshift-hyperfleet/hyperfleet-api \
  --set image.tag=test \
  --set-json 'adapters.cluster=["validation","dns","pullsecret","hypershift"]' \
  --set-json 'adapters.nodepool=["validation","hypershift"]' | kubeconform_validate
pass "Full adapter config template"

# ─── Validation schema tests ─────────────────────────────────────────

run_test "template with validation schema enabled"
OUTPUT=$(render \
  --set validationSchema.enabled=true \
  --set-string 'validationSchema.content=openapi: 3.0.0')
assert_contains "$OUTPUT" 'app.kubernetes.io/component: validation-schema' "validation-schema ConfigMap not found"
assert_contains "$OUTPUT" '/etc/hyperfleet/validation-schema' "validation schema volume mount not found"
echo "$OUTPUT" | kubeconform_validate
pass "Validation schema enabled config template"

run_test "template with validation schema disabled (default)"
OUTPUT=$(render)
assert_not_contains "$OUTPUT" 'validation-schema' "validation-schema should not appear when disabled"
echo "$OUTPUT" | kubeconform_validate
pass "Validation schema disabled config template"

run_test "template with validation schema existingConfigMap"
OUTPUT=$(render \
  --set validationSchema.enabled=true \
  --set validationSchema.existingConfigMap=my-validation-schema)
assert_contains "$OUTPUT" 'my-validation-schema' "existingConfigMap name not found"
assert_not_contains "$OUTPUT" 'app.kubernetes.io/component: validation-schema' "generated ConfigMap should not appear with existingConfigMap"
echo "$OUTPUT" | kubeconform_validate
pass "Validation schema existingConfigMap config template"

run_test "validation schema fails without content or existingConfigMap"
OUTPUT=$(render --set validationSchema.enabled=true 2>&1 || true)
assert_contains "$OUTPUT" 'validationSchema.content is required' "expected validationSchema validation error message"
pass "Validation schema validation (no content)"

run_test "validation schema fails with whitespace-only content"
OUTPUT=$(render --set validationSchema.enabled=true --set-string 'validationSchema.content=   ' 2>&1 || true)
assert_contains "$OUTPUT" 'validationSchema.content is required' "expected validationSchema validation error message"
pass "Validation schema validation (whitespace-only content)"

# ─── Entity rendering tests ──────────────────────────────────────────

run_test "template with default entities (Channel, Version, WifConfig)"
OUTPUT=$(render)
CONFIG_YAML=$(echo "$OUTPUT" | extract_config_yaml)
echo "$CONFIG_YAML" | validate_yaml || fail "rendered config.yaml is not valid YAML"
assert_contains "$CONFIG_YAML" 'kind: Channel' "Channel entity not found in config.yaml"
assert_contains "$CONFIG_YAML" 'kind: Version' "Version entity not found in config.yaml"
assert_contains "$CONFIG_YAML" 'kind: WifConfig' "WifConfig entity not found in config.yaml"
echo "$OUTPUT" | kubeconform_validate
pass "Default entities config template"

run_test "template with entities using all optional fields"
OUTPUT=$(helm template "$RELEASE_NAME" "$CHART_DIR" -f charts/ci/entities-all-fields-values.yaml)
CONFIG_YAML=$(echo "$OUTPUT" | extract_config_yaml)
echo "$CONFIG_YAML" | validate_yaml || fail "rendered config.yaml is not valid YAML with all entity fields"
assert_contains "$CONFIG_YAML" 'parent_kind: Channel' "parent_kind not found in config.yaml"
assert_contains "$CONFIG_YAML" 'on_parent_delete: restrict' "on_parent_delete not found in config.yaml"
assert_contains "$CONFIG_YAML" 'ref_type: channel' "references not found in config.yaml"
assert_contains "$CONFIG_YAML" 'required_adapters:' "required_adapters not found in config.yaml"
echo "$OUTPUT" | kubeconform_validate
pass "Entities with all optional fields config template"

run_test "template with empty entities list"
OUTPUT=$(render --set-json 'config.entities=[]')
CONFIG_YAML=$(echo "$OUTPUT" | extract_config_yaml)
echo "$CONFIG_YAML" | validate_yaml || fail "rendered config.yaml is not valid YAML with empty entities"
assert_contains "$CONFIG_YAML" 'entities: \[\]' "expected entities: [] in config.yaml"
echo "$OUTPUT" | kubeconform_validate
pass "Empty entities list config template"

# ─── Summary ─────────────────────────────────────────────────────────

echo ""
if [ "$FAILED" -gt 0 ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "FAILED: $FAILED test(s) failed, $PASSED passed"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "All Helm chart tests passed! ($PASSED tests)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
