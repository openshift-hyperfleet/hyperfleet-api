# Condition Mapping Configuration Guide

Condition mapping is a CEL-based engine that exposes adapter-specific conditions in the public `status.conditions` array on Cluster and NodePool resources. Operators configure declarative rules in YAML — no code changes required.

---

## Table of Contents

- [Overview](#overview)
- [Configuration Schema](#configuration-schema)
  - [Rule Structure](#rule-structure)
  - [Reserved Condition Types](#reserved-condition-types)
- [CEL Evaluation Context](#cel-evaluation-context)
  - [Variables](#variables)
  - [The `statuses` Variable](#the-statuses-variable)
  - [The `resource` Variable](#the-resource-variable)
  - [Custom CEL Functions](#custom-cel-functions)
- [Safe Navigation Pattern](#safe-navigation-pattern)
- [Usage Examples](#usage-examples)
  - [Example 1: One-to-One Transformation](#example-1-one-to-one-transformation)
  - [Example 2: Cross-Adapter Aggregation](#example-2-cross-adapter-aggregation)
  - [Example 3: Data Field Extraction with Safe Navigation](#example-3-data-field-extraction-with-safe-navigation)
- [Security](#security)
  - [Automatic Sensitive Data Masking](#automatic-sensitive-data-masking)
  - [Data Field Exposure Risks](#data-field-exposure-risks)
  - [Operator Responsibilities](#operator-responsibilities)
- [Migration Guide](#migration-guide)
  - [Before: Hardcoded Conditions](#before-hardcoded-conditions)
  - [After: Config-Driven Conditions](#after-config-driven-conditions)
  - [Migration Steps](#migration-steps)
- [DSL Keyword Consistency](#dsl-keyword-consistency)
- [CI Validation Guidance](#ci-validation-guidance)
  - [Startup Validation (Fail-Fast)](#startup-validation-fail-fast)
  - [Testing Mapped Conditions](#testing-mapped-conditions)
  - [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Field Length Constraints](#field-length-constraints)
- [Error Handling](#error-handling)
- [Additional Resources](#additional-resources)

---

## Overview

When an adapter reports status via `PUT /statuses`, the API runs condition mapping **within the existing status aggregation flow**:

1. Store the adapter status row
2. Fetch all adapter statuses for the resource
3. Compute aggregated conditions (`Reconciled`, `LastKnownReconciled`)
4. **Apply condition mapping rules** (if configured)
5. Persist all conditions to the resource

Mapping produces **resource conditions** (True/False only) from **adapter conditions** (True/False/Unknown). Individual adapter conditions with `Unknown` status are automatically filtered out before CEL evaluation — only `True`/`False` conditions are available in the `statuses` variable.

Each rule executes once per `PUT /statuses` request and produces at most one resource condition. Rules are compiled at API startup — invalid CEL expressions prevent the server from starting (fail-fast).

---

## Configuration Schema

Condition mapping rules are defined per entity type in the `entities` section of `config.yaml`:

```yaml
entities:
  - kind: Cluster
    plural: clusters
    required_adapters: [validation, dns, pullsecret, hypershift]

    conditions:
      - type: LandingZoneReady
        when:
          expression: |
            statuses.exists(s, s.adapter == "landing-zone-adapter"
              && s.conditions.exists(c, c.type == "NamespaceReady"))
        output:
          status:
            expression: |
              statuses.filter(s, s.adapter == "landing-zone-adapter")[0]
                .conditions.filter(c, c.type == "NamespaceReady")[0].status
          reason:
            expression: |
              statuses.filter(s, s.adapter == "landing-zone-adapter")[0]
                .conditions.filter(c, c.type == "NamespaceReady")[0].reason
          message:
            expression: |
              "Landing zone: " + statuses.filter(s, s.adapter == "landing-zone-adapter")[0]
                .conditions.filter(c, c.type == "NamespaceReady")[0].message
```

### Rule Structure

Each rule in the `conditions` array has the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Output condition type name (e.g., `ROSAControlPlaneReady`). Must be unique per entity and not reserved. Max 100 characters. |
| `when.expression` | string (CEL) | Yes | Boolean expression. If `true`, the rule fires and produces a condition. If `false`, the rule is skipped. |
| `output.status.expression` | string (CEL) | Yes | Must evaluate to `"True"` or `"False"` (case-insensitive). |
| `output.reason.expression` | string (CEL) | Yes | Machine-readable CamelCase reason string. Max 256 characters (condition skipped if exceeded). |
| `output.message.expression` | string (CEL) | Yes | Human-readable description. Max 2048 characters (truncated if exceeded). |

Four additional fields are **automatically generated** by the API:

- `observed_generation` — set to the current resource generation
- `created_time` — set when the condition is first created; preserved on subsequent updates
- `last_updated_time` — updated on every evaluation that produces this condition, regardless of whether `status` changed
- `last_transition_time` — updated only when `status` changes (matches Kubernetes condition semantics)

### Reserved Condition Types

The following condition types cannot be used in mapping rules:

| Type | Source |
|------|--------|
| `Reconciled` | Computed by status aggregation |
| `LastKnownReconciled` | Computed by status aggregation |
| Per-adapter synthesized types | Auto-generated from `required_adapters` (e.g., `validation` adapter produces `ValidationSuccessful`) |

Attempting to use a reserved type causes a startup validation error.

---

## CEL Evaluation Context

### Variables

All CEL expressions in condition mapping rules have access to the same context:

| Variable | Type | Description |
|----------|------|-------------|
| `statuses` | `list(dyn)` | Array of adapter statuses for the resource. Individual conditions with `Unknown` status are filtered out. |
| `resource` | `dyn` | Full resource object (Cluster/NodePool) as a map. Sensitive fields are masked. |

> **Note**: Unlike the Adapter Framework and Sentinel, condition mapping does not currently expose an `env` variable. Environment-specific logic should use `resource.metadata` (labels, annotations) or adapter-reported `data` fields instead. The `env` variable may be added in a future release if use cases require it.

### The `statuses` Variable

Each entry in `statuses` represents one adapter's status report and has the following structure:

| Field | Type | Description |
|-------|------|-------------|
| `adapter` | `string` | Adapter name (e.g., `"rosa-adapter"`, `"validation"`) |
| `observed_generation` | `number` | Resource generation the adapter observed |
| `conditions` | `list(map)` | Array of adapter conditions (see below) |
| `data` | `map` | Adapter-specific JSONB data (sensitive fields masked) |

Each condition in `conditions` has:

| Field | Type | Description |
|-------|------|-------------|
| `type` | `string` | Condition type (e.g., `"Available"`, `"ControlPlaneReady"`) |
| `status` | `string` | `"True"` or `"False"` (`"Unknown"` entries are filtered before evaluation) |
| `reason` | `string` | Machine-readable reason |
| `message` | `string` | Human-readable message |

**Accessing conditions**: Use nested filtering to reach a specific adapter's condition:

```cel
statuses.filter(s, s.adapter == "rosa-adapter")[0]
  .conditions.filter(c, c.type == "ControlPlaneReady")[0].status
```

### The `resource` Variable

The `resource` variable provides access to the full resource object as a map. Commonly used fields:

| Path | Type | Description |
|------|------|-------------|
| `resource.generation` | `number` | Current resource generation |
| `resource.id` | `string` | Resource ID |
| `resource.name` | `string` | Resource name |
| `resource.kind` | `string` | Resource kind (e.g., `"Cluster"`) |
| `resource.spec` | `map` | Resource spec (desired state) |
| `resource.metadata` | `map` | Resource metadata (labels, annotations) |

### Custom CEL Functions

Two custom functions are available in addition to the standard CEL library:

| Function | Signature | Description |
|----------|-----------|-------------|
| `toJson(value)` | `dyn → string` | Marshal any value to a JSON string. Output capped at 1 MB. |
| `dig(target, "dot.path")` | `(dyn, string) → dyn` | Safe nested navigation through maps and arrays. Returns `null` on missing keys. Supports array indices (e.g., `"items.0.name"`). |

**Example using `dig`**:

```cel
dig(statuses.filter(s, s.adapter == "gcp-adapter")[0].data, "quota.remaining")
```

**Example using `toJson`**:

```cel
"Adapter data: " + toJson(statuses.filter(s, s.adapter == "gcp-adapter")[0].data)
```

---

## Safe Navigation Pattern

When accessing optional or nested fields (like `data`), use the **safe navigation operator (`?`)** to prevent runtime errors:

| Pattern | Example | Behavior |
|---------|---------|----------|
| Unsafe | `has(c.data.quotaRemaining)` | Fails with error if `data` field does not exist |
| Safe (check) | `c.?data.?quotaRemaining.hasValue()` | Returns `false` if `data` or `quotaRemaining` is missing |
| Safe (access) | `c.?data.?quotaRemaining.orValue(0)` | Returns `0` if missing; actual value otherwise |

**Why**: The `?` operator safely accesses maps without errors on missing keys. Combined with:

- `hasValue()` — existence check (returns `true` if the field exists and is not null)
- `orValue(default)` — safe access with a default value

This pattern handles all states: key missing, key with nil value, key with value.

**Example in a `when` expression**:

```cel
statuses.exists(s, s.adapter == "gcp-adapter"
  && s.?data.?quotaRemaining.hasValue())
```

**Example in an `output` expression**:

```cel
statuses.filter(s, s.adapter == "gcp-adapter")[0]
  .?data.?quotaRemaining.orValue(0) > 10 ? "True" : "False"
```

**Alternative**: Use the `dig()` custom function for deeply nested paths:

```cel
dig(statuses.filter(s, s.adapter == "gcp-adapter")[0].data, "quota.remaining") != null
```

---

## Usage Examples

### Example 1: One-to-One Transformation

Copy a single adapter condition to the public API with a transformed message prefix.

**Config**:

```yaml
conditions:
  - type: ROSAControlPlaneReady
    when:
      expression: |
        statuses.exists(s, s.adapter == "rosa-adapter"
          && s.conditions.exists(c, c.type == "ControlPlaneReady"))
    output:
      status:
        expression: |
          statuses.filter(s, s.adapter == "rosa-adapter")[0]
            .conditions.filter(c, c.type == "ControlPlaneReady")[0].status
      reason:
        expression: |
          statuses.filter(s, s.adapter == "rosa-adapter")[0]
            .conditions.filter(c, c.type == "ControlPlaneReady")[0].reason
      message:
        expression: |
          "ROSA: " + statuses.filter(s, s.adapter == "rosa-adapter")[0]
            .conditions.filter(c, c.type == "ControlPlaneReady")[0].message
```

**Adapter reports** (`PUT /clusters/{id}/statuses`):

```json
{
  "adapter": "rosa-adapter",
  "conditions": [
    {
      "type": "Available", "status": "True",
      "reason": "Ready", "message": "Ready"
    },
    {
      "type": "ControlPlaneReady", "status": "True",
      "reason": "Operational", "message": "Control plane is operational"
    }
  ]
}
```

**Result in `GET /clusters/{id}` → `status.conditions`**:

```json
{
  "type": "ROSAControlPlaneReady",
  "status": "True",
  "reason": "Operational",
  "message": "ROSA: Control plane is operational",
  "observed_generation": 5,
  "last_transition_time": "2026-05-19T10:32:00Z"
}
```

### Example 2: Cross-Adapter Aggregation

Aggregate signals from multiple adapters into a single health condition.

**Config**:

```yaml
conditions:
  - type: ClusterHealthy
    when:
      expression: |
        statuses.exists(s, s.adapter == "rosa-adapter"
          && s.conditions.exists(c, c.type == "ControlPlaneReady"))
        && statuses.exists(s, s.adapter == "dns-adapter"
          && s.conditions.exists(c, c.type == "Available"))
    output:
      status:
        expression: |
          statuses.filter(s, s.adapter == "rosa-adapter")[0]
            .conditions.filter(c, c.type == "ControlPlaneReady")[0].status == "True"
          && statuses.filter(s, s.adapter == "dns-adapter")[0]
            .conditions.filter(c, c.type == "Available")[0].status == "True"
          ? "True" : "False"
      reason:
        expression: |
          statuses.filter(s, s.adapter == "rosa-adapter")[0]
            .conditions.filter(c, c.type == "ControlPlaneReady")[0].status == "True"
          && statuses.filter(s, s.adapter == "dns-adapter")[0]
            .conditions.filter(c, c.type == "Available")[0].status == "True"
          ? "AllHealthy" : "Degraded"
      message:
        expression: |
          "Cluster health based on ROSA control plane and DNS availability"
```

### Example 3: Data Field Extraction with Safe Navigation

Extract structured data from the adapter's `data` field using safe navigation.

**Config**:

```yaml
conditions:
  - type: GCPQuotaStatus
    when:
      expression: |
        statuses.exists(s, s.adapter == "gcp-adapter"
          && s.conditions.exists(c, c.type == "QuotaAvailable")
          && s.?data.?quotaRemaining.hasValue())
    output:
      status:
        expression: |
          statuses.filter(s, s.adapter == "gcp-adapter")[0]
            .?data.?quotaRemaining.orValue(0) > 10 ? "True" : "False"
      reason:
        expression: |
          statuses.filter(s, s.adapter == "gcp-adapter")[0]
            .?data.?quotaRemaining.orValue(0) > 10 ? "SufficientQuota" : "LowQuota"
      message:
        expression: |
          "GCP quota remaining: " + string(statuses.filter(s, s.adapter == "gcp-adapter")[0]
            .?data.?quotaRemaining.orValue(0))
```

**Adapter reports** (with `data` field):

```json
{
  "adapter": "gcp-adapter",
  "conditions": [
    {
      "type": "QuotaAvailable", "status": "True",
      "reason": "Available", "message": "Quota is available"
    }
  ],
  "data": {
    "quotaRemaining": 25,
    "internalProjectId": "secret-12345"
  }
}
```

**Result**: The CEL expression extracts only `quotaRemaining` (customer-visible). The `internalProjectId` field is never referenced in the mapping rule and does not appear in the output. Additionally, fields matching sensitive patterns (like keys containing `secret`) are automatically masked before CEL evaluation.

```json
{
  "type": "GCPQuotaStatus",
  "status": "True",
  "reason": "SufficientQuota",
  "message": "GCP quota remaining: 25"
}
```

---

## Security

### Automatic Sensitive Data Masking

Before CEL expressions evaluate, the API automatically masks adapter `data` fields whose keys match sensitive patterns. Matched values are replaced with `***REDACTED***`.

Masked patterns (case-insensitive substring match):

| Pattern | Catches |
|---------|---------|
| `password` | `adminPassword`, `dbPassword` |
| `secret` | `pullSecret`, `clientSecret` |
| `token` | `accessToken`, `refreshToken`, `bearerToken` |
| `credential` | `serviceCredential` |
| `apikey`, `api_key` | `gcpApiKey`, `api_key` |
| `privatekey` | `sshPrivateKey`, `tlsPrivateKey` |
| `secretkey` | `awsSecretKey` |
| `accesskey` | `awsAccessKey` |
| `sshkey` | `sshKeyData`, `sshKeyPair` |
| `encryptionkey` | `dataEncryptionKey`, `diskEncryptionKey` |
| `accountkey` | `serviceAccountKey` |
| `servicekey` | `gcpServiceKey`, `azureServiceKey` |
| `registrykey` | `containerRegistryKey` |
| `signingkey` | `codeSigningKey`, `imageSigningKey` |
| `cert` | `tlsCert`, `caCertificate` |
| `kubeconfig` | `kubeconfigData` |
| `auth` | `authToken`, `authorization` |
| `private` | `privateEndpoint`, `privateData` |
| `connection` | `connectionString`, `dbConnection` |
| `bearer` | `bearerAuth` |
| `passphrase` | `keyPassphrase` |

Masking is recursive (nested maps and arrays are traversed up to 20 levels deep).

The `resource` variable is also masked using the same patterns before being exposed to CEL.

> **Note**: Some patterns are intentionally broad (e.g., `private`, `auth`, `connection`) and may mask non-sensitive fields like `privateEndpoint` or `connectionTimeout`. This is a deliberate trade-off — over-masking is preferred over under-masking to prevent credential leakage.

### Data Field Exposure Risks

The adapter `data` field (JSONB) may contain sensitive information: API tokens, internal IPs, credentials, or internal resource IDs. While automatic masking catches common patterns, it is **not exhaustive**.

Risks:

- Custom field names that do not match masking patterns will be exposed
- Mapping rules that reference data fields can surface internal details in the public API
- Mapped condition `reason` and `message` fields are visible to external consumers

### Operator Responsibilities

Operators configuring mapping rules are responsible for:

1. **Reviewing adapter `data` schemas** — understand what each adapter stores in its `data` field before writing rules that access it
2. **Testing in non-production environments** — validate that mapping rules do not leak sensitive data before deploying to production
3. **Using selective extraction** — use CEL expressions to extract only specific, non-sensitive fields from `data` (e.g., `s.?data.?publicField.orValue("")` instead of `toJson(s.data)`)
4. **Auditing mapped conditions** — verify that mapped condition `reason` and `message` fields only expose customer-visible status, not internal implementation details

---

## Migration Guide

### Before: Hardcoded Conditions

Without condition mapping, the API generates per-adapter conditions using a hardcoded naming convention:

- Adapter name → PascalCase + `Successful` suffix
- Example: `validation` → `ValidationSuccessful`, `my-adapter` → `MyAdapterSuccessful`

These synthesized conditions reflect the adapter's `Available` condition status. Custom adapter conditions (e.g., `QuotaSufficient`, `ControlPlaneReady`) are **not** surfaced in the public API — they are only accessible via the internal `GET /statuses` endpoint.

**Example `status.conditions` without mapping**:

```json
[
  {"type": "Reconciled", "status": "True", "reason": "AllAdaptersAvailable"},
  {"type": "LastKnownReconciled", "status": "True", "reason": "AllAdaptersAvailable"},
  {"type": "ValidationSuccessful", "status": "True", "reason": "Available"},
  {"type": "HypershiftSuccessful", "status": "True", "reason": "Available"}
]
```

### After: Config-Driven Conditions

With condition mapping, operators can expose any adapter condition in the public API:

```yaml
entities:
  - kind: Cluster
    conditions:
      - type: QuotaValid
        when:
          expression: |
            statuses.exists(s, s.adapter == "validation"
              && s.conditions.exists(c, c.type == "QuotaSufficient"))
        output:
          status:
            expression: |
              statuses.filter(s, s.adapter == "validation")[0]
                .conditions.filter(c, c.type == "QuotaSufficient")[0].status
          reason:
            expression: |
              statuses.filter(s, s.adapter == "validation")[0]
                .conditions.filter(c, c.type == "QuotaSufficient")[0].reason
          message:
            expression: |
              "Quota: " + statuses.filter(s, s.adapter == "validation")[0]
                .conditions.filter(c, c.type == "QuotaSufficient")[0].message
```

**Example `status.conditions` with mapping**:

```json
[
  {"type": "Reconciled", "status": "True", "reason": "AllAdaptersAvailable"},
  {"type": "LastKnownReconciled", "status": "True", "reason": "AllAdaptersAvailable"},
  {"type": "ValidationSuccessful", "status": "True", "reason": "Available"},
  {"type": "HypershiftSuccessful", "status": "True", "reason": "Available"},
  {"type": "QuotaValid", "status": "True", "reason": "QuotaOK", "message": "Quota: Cluster quota is sufficient"}
]
```

### Migration Steps

1. **Identify which adapter conditions to expose** — review `GET /clusters/{id}/statuses` to see available adapter conditions and their `type` names

2. **Write mapping rules** — add `conditions` entries to the entity descriptor in `config.yaml` (see [Configuration Schema](#configuration-schema))

3. **Test locally** — start the API with the new config. Invalid CEL expressions will cause a startup error with a descriptive message indicating the rule and expression that failed

4. **Verify mapped conditions** — create a test resource, report adapter status, and check `GET /clusters/{id}` to confirm mapped conditions appear in `status.conditions`

5. **Update consumers** — notify CLI, UI, and integration teams about new condition types they can now read from the public API

> **Note**: Existing aggregated conditions (`Reconciled`, `LastKnownReconciled`) and per-adapter synthesized conditions (`*Successful`) are unaffected. Mapped conditions are **appended** alongside them.

---

## DSL Keyword Consistency

The condition mapping configuration uses `when` and `output` keywords that are consistent with the CEL DSL patterns across the HyperFleet platform:

| Component | `when` keyword | Output keywords | CEL engine |
|-----------|---------------|-----------------|------------|
| **API condition mapping** | `when.expression` — boolean gate | `output.status.expression`, `output.reason.expression`, `output.message.expression` | `google/cel-go` |
| **Adapter framework** | `when.expression` — task precondition | `expression` in params, payload builders | `google/cel-go` |
| **Sentinel decision engine** | `when.expression` — reconciliation trigger | `params[].expression` for parameter extraction | `google/cel-go` |

All three components:

- Compile CEL expressions at startup (fail-fast)
- Use `google/cel-go` as the expression engine
- Support optional chaining (`?.`, `hasValue()`, `orValue()`)
- Provide `env.*` access for environment-specific logic (adapter and Sentinel; API condition mapping currently exposes `resource` instead)

Cross-references:

- [Adapter Framework Design — CEL evaluation](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/adapter/framework/adapter-frame-design.md)
- [Sentinel Decision Engine Reference](https://github.com/openshift-hyperfleet/hyperfleet-sentinel/blob/main/docs/decision-engine.md)
- [ADR-0006 — CEL as the Shared Expression Evaluation Engine](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/adrs/0006-cel-expression-engine.md)

---

## CI Validation Guidance

### Startup Validation (Fail-Fast)

All CEL expressions are **compiled and validated at API server startup**. If any expression has invalid syntax, references undefined variables, or uses incorrect function signatures, the server will fail to start with a descriptive error:

```text
Cluster.ROSAControlPlaneReady.when: invalid CEL expression: parse error: ...
```

This fail-fast behavior catches most configuration issues before any traffic is served.

**What startup validates**:

- CEL syntax (parse errors)
- Function arity and existence (e.g., calling `toJson()` with wrong number of arguments)
- Expression cost limits (rejects overly complex expressions)
- Reserved condition type conflicts
- Duplicate condition types within the same entity
- Condition type length (max 100 characters)

**What startup does NOT validate**:

- Runtime data shape (e.g., an adapter not reporting the expected condition type)
- Field existence in `data` maps (use safe navigation to handle this)
- Semantic correctness (e.g., a status expression that always returns `"True"`)

### Testing Mapped Conditions

Integration tests for condition mapping are available in `test/integration/condition_mapping_test.go`. To run them:

```bash
# Run BEFORE tests (no mapping configured — verify custom conditions are NOT exposed)
make test-integration

# Run AFTER tests (with mapping configured — verify custom conditions ARE exposed)
HYPERFLEET_TEST_CONDITION_MAPPING=1 make test-integration
```

The BEFORE/AFTER test pattern validates the mapping behavior by comparing API output with and without CEL mapping rules configured.

### Pre-Deployment Checklist

Before deploying condition mapping rules to production:

- [ ] **Start the API locally** with the new config — confirm it starts without CEL compilation errors
- [ ] **Report test adapter statuses** — verify mapped conditions appear in `GET /clusters/{id}`
- [ ] **Test with missing conditions** — verify the `when` expression returns `false` gracefully when the expected adapter condition is not present
- [ ] **Test with missing `data` fields** — verify safe navigation (`?.`, `orValue()`) handles absent fields without errors
- [ ] **Review mapped condition output** — confirm `reason` and `message` fields do not expose sensitive or internal information
- [ ] **Verify reserved types** — confirm your condition type names do not conflict with `Reconciled`, `LastKnownReconciled`, or `*Successful` types
- [ ] **Check field lengths** — confirm `reason` values are under 256 characters and `message` values are under 2048 characters

---

## Field Length Constraints

| Field | Max Length | Behavior on Violation |
|-------|-----------|----------------------|
| `type` | 100 characters | Startup validation error (server does not start) |
| `reason` | 256 characters | Condition skipped entirely, warning logged |
| `message` | 2048 characters | Truncated to valid UTF-8 boundary, info logged |

---

## Error Handling

**CEL evaluation errors** (type mismatch, null reference, cost limit exceeded) cause the **entire mapping operation to fail** and the database transaction to **roll back**. The adapter receives an error response, and the status update is retried on the next reconciliation cycle (typically 10 seconds).

This ensures the system remains consistent: either all adapter status and mapped conditions are committed together, or none are committed.

**Why not skip the failed rule?** If the last required adapter reports `Available: True` but mapping fails, committing the adapter status without mapped conditions would set `Reconciled: True` and delay the next reconcile attempt from 10 seconds to 30 minutes.

**Malformed adapter data**: If adapter conditions JSONB is corrupted or unparseable, those conditions are excluded from the `statuses` array. All mapping rules still evaluate with the valid subset.

---

## Additional Resources

- [Condition Mapping Design Document](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/api-service/condition-mapping-design.md) — architecture design and rationale
- [Status Guide](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/status-guide.md) — adapter status reporting contract
- [ADR-0008 — Dynamic Status Aggregation](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/adrs/0008-dynamic-status-aggregation.md) — compute-on-write aggregation model
- [ADR-0007 — Conditions-Based Status Model](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/adrs/0007-conditions-based-status-model.md) — ResourceCondition and AdapterCondition contracts
- [ADR-0006 — CEL Expression Engine](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/adrs/0006-cel-expression-engine.md) — CEL as the shared expression engine
- [Configuration Guide](config.md) — general API configuration reference
