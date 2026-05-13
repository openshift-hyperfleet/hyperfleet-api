# Claude Code Guidelines for OpenAPI

## Schema Source

The OpenAPI schemas are defined in the `hyperfleet-api-spec` repository (TypeSpec) and consumed here via the Go module `github.com/openshift-hyperfleet/hyperfleet-api-spec/schemas`.

**`openapi/openapi.yaml` is extracted from the Go module during `make generate` — it is NOT tracked in git.**

## Two openapi.yaml Files (both generated, not in git)

1. **Extracted**: `openapi/openapi.yaml` (61KB, uses `$ref`) — copied from the module cache via `go list -m -f '{{.Dir}}'`. Used as input to oapi-codegen.
2. **Generated**: `pkg/api/openapi/api/openapi.yaml` (44KB, fully resolved) — produced by oapi-codegen, embedded in binary via `//go:embed`

**Never edit either file directly.** Both are overwritten by `make generate`.

## Code Generation

Tool: **oapi-codegen** (not openapi-generator-cli)
Config: `oapi-codegen.yaml` in this directory

```
make generate        # Extracts schema from spec module, then runs oapi-codegen
make generate-all    # generate + generate-mocks
```

Generation produces:
- `pkg/api/openapi/openapi.gen.go` — Go model structs + client + embedded spec

## Config Details

From `oapi-codegen.yaml`:
- Package: `openapi`
- Output: `pkg/api/openapi/openapi.gen.go`
- Generates: models (yes), chi-server (no), client (yes), embedded-spec (yes)
- Compatibility: `old-merge-schemas: true` (inlines allOf), `old-aliasing: true` (type defs not aliases)

## Updating the API

1. Update TypeSpec definitions in `hyperfleet-api-spec` repo and release a new version
2. Update the module version in `go.mod`: `go get github.com/openshift-hyperfleet/hyperfleet-api-spec@vX.Y.Z`
3. Run `make generate-all`
4. Update handlers/services/DAOs for any new or changed fields

For local development (before the spec module is published), the `replace` directive in `go.mod` points to a local checkout:
```
replace github.com/openshift-hyperfleet/hyperfleet-api-spec => /path/to/hyperfleet-api-spec
```
