# Claude Code Guidelines for Helm Charts

## Verification

Always validate changes with: `make test-helm`

This runs lint + template rendering across multiple value combinations (default, external DB, autoscaling, PDB, ServiceMonitor, auth disabled, custom image, full adapter config).

## Required Values

Adapter arrays are required — templates fail without them:
```
--set 'adapters.cluster=["validation"]'
--set 'adapters.nodepool=["validation"]'
```

## Database Modes

Two database configurations supported:
- **Embedded PostgreSQL** (default): `database.postgresql.enabled=true`
- **External database**: `database.postgresql.enabled=false`, `database.external.enabled=true`, `database.external.secretName=...`

## Chart Structure

- `Chart.yaml` — chart metadata (apiVersion v2)
- `values.yaml` — default values
- `templates/` — Kubernetes manifests

## Template Testing

When modifying templates, test with multiple configurations:
```
helm template test-release charts/ --set 'adapters.cluster=["validation"]' --set 'adapters.nodepool=["validation"]' [additional overrides]
```
