# Claude Code Guidelines for Helm Charts

## Verification

Always validate changes with: `make test-helm`

This runs lint + template rendering across multiple value combinations (default, external DB, autoscaling, PDB, ServiceMonitor, auth disabled, custom image, full adapter config).

## Entity Configuration

Cluster and NodePool (and all other entity types) are configured as entity descriptors in `config.entities`.
Default values include Cluster, NodePool, Channel, Version, and WifConfig. Each entity's required adapters
are declared inline via `required_adapters`.

## Entity Registration

Entity descriptors are configured in `config.entities`. Default values include Channel, Version, and WifConfig.
Override with `--set-json 'config.entities=[...]'` for custom entity sets. An empty list disables all
generic entity routes.

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
helm template test-release charts/ [additional overrides]
```
