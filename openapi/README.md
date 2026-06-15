# OpenAPI Schema — Source of Truth

This directory contains the code-generation configuration for the HyperFleet API's OpenAPI layer.

## Overview

OpenAPI schemas are **not authored here**. They are defined in the [`hyperfleet-api-spec`](https://github.com/openshift-hyperfleet/hyperfleet-api-spec) repository (TypeSpec) and consumed by this repository as a Go module. The `openapi/openapi.yaml` file is extracted from the module cache at code-generation time and is **not tracked in git**.

## For Operators

### Validation Schema

#### Why this exists

HyperFleet API is intentionally schema-agnostic at its core: it stores clusters and nodepools as long as the `spec` field is present and non-null, without caring what is inside it. This is by design — the API serves multiple deployments with different provider-specific payloads.

Deployers, however, **do** care. A GCP deployment might require a `region` field inside `spec`; an AWS deployment might require an `instanceType`. Without validation, invalid or incomplete specs silently end up in the database and only fail later when a downstream component tries to use them.

The `--server-openapi-schema-path` flag solves this: at deploy time, the operator points the API at a deployment-specific OpenAPI schema file. The API then validates every `POST`/`PATCH` request's `spec` payload against that schema in HTTP middleware — before any service or database code runs.

#### What the schema file must contain

The schema file must be a valid OpenAPI 3.0 document. The API looks up two specific component schemas by name:

| Resource | Required component |
|----------|--------------------|
| `cluster` | `components.schemas.ClusterSpec` |
| `nodepool` | `components.schemas.NodePoolSpec` |

A minimal example for a GCP deployment:

```yaml
openapi: 3.0.0
info:
  title: HyperFleet GCP Validation Schema
  version: 1.0.0
paths: {}
components:
  schemas:
    ClusterSpec:
      type: object
      required:
        - region
      properties:
        region:
          type: string
          description: GCP region (e.g. us-central1)
        zone:
          type: string
    NodePoolSpec:
      type: object
      required:
        - machineType
        - replicas
      properties:
        machineType:
          type: string
        replicas:
          type: integer
          minimum: 1
          maximum: 100
```

If `ClusterSpec` or `NodePoolSpec` is absent from the file, the API **fails to start** with an error — this ensures invalid schemas are caught immediately rather than silently skipping validation.

#### How to configure it

Three equivalent ways to supply the path:

| Method | Example |
|--------|---------|
| CLI flag | `--server-openapi-schema-path=/etc/hyperfleet/schemas/openapi.yaml` |
| Environment variable | `HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH=/etc/hyperfleet/schemas/openapi.yaml` |
| Config file | `server.openapi_schema_path: /etc/hyperfleet/schemas/openapi.yaml` |

**Default:** `openapi/openapi.yaml` (the core schema extracted by `make generate` — provider-agnostic, accepts any non-null spec).

#### Runtime behaviour

- Validation runs in HTTP middleware on every `POST` and `PATCH` request, before the service or database layer.
- Invalid specs return `400 Bad Request` with field-level error details.
- If validationSchema is enabled and the schema file is missing or malformed, the API **fails to start** with an error — this ensures misconfigured deployments are caught immediately.

## For Developers

### Directory Contents

| File | Purpose |
|------|---------|
| `oapi-codegen.yaml` | Code-generation config for `oapi-codegen` |
| `openapi.yaml` | **Not in git** — extracted from the Go module by `make generate` |

### How Schemas Are Imported

1. The `github.com/openshift-hyperfleet/hyperfleet-api-spec` module is declared in `go.mod`.
2. `make generate` locates the module's on-disk path via `go list -m -f '{{.Dir}}'` and copies `schemas/core/openapi.yaml` to `openapi/openapi.yaml`. Code generation always uses the `core` variant.
3. `oapi-codegen` reads `openapi/openapi.yaml` and produces `pkg/api/openapi/openapi.gen.go` — Go model structs, an HTTP client, and an embedded resolved spec.

### Generated Artifacts

| Artifact | Location | Description |
|----------|----------|-------------|
| Extracted spec | `openapi/openapi.yaml` | Copied from Go module; input to oapi-codegen |
| Go models + client | `pkg/api/openapi/openapi.gen.go` | Never edit — regenerate with `make generate` |
| Embedded resolved spec | Inside `openapi.gen.go` | Fully resolved; served at `/api/hyperfleet/v1/openapi` |

**Never edit `openapi.yaml` or `openapi.gen.go` directly.** Both are overwritten by `make generate`.

### Updating the API Schema

1. Update TypeSpec definitions in the [`hyperfleet-api-spec`](https://github.com/openshift-hyperfleet/hyperfleet-api-spec) repository and publish a new release.

2. Bump the module version in `go.mod`:

   ```shell
   go get github.com/openshift-hyperfleet/hyperfleet-api-spec@vX.Y.Z
   ```

3. Regenerate:

   ```shell
   make generate-all
   ```

4. Update handlers, services, and DAOs for any new or changed fields.

For local development before a new spec version is published, add a `replace` directive in `go.mod`:

```go
replace github.com/openshift-hyperfleet/hyperfleet-api-spec => /path/to/local/hyperfleet-api-spec
```

### Code Generation Commands

```shell
make generate        # Extract schema from spec module, then run oapi-codegen
make generate-mocks  # Regenerate mock implementations (go generate)
make generate-all    # Both of the above
```

### oapi-codegen Configuration

From `oapi-codegen.yaml`:

- **Package**: `openapi`
- **Output**: `pkg/api/openapi/openapi.gen.go`
- **Generates**: models, HTTP client, embedded spec (chi-server disabled)
- **Compatibility flags**: `old-merge-schemas: true` (inlines `allOf`), `old-aliasing: true` (type definitions, not aliases)
