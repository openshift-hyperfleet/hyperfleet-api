# Claude Code Guidelines for OpenAPI

See [README.md](README.md) for the full reference on how schemas are imported, generated, and used for validation.

## Key Rules

- **Never edit** `openapi.yaml` or `pkg/api/openapi/openapi.gen.go` — both are overwritten by `make generate`.
- **Never copy** the spec file manually — `make generate` extracts it from the Go module cache automatically.
- **To change the schema**, update TypeSpec in `hyperfleet-api-spec`, publish a new release, bump the version in `go.mod`, then run `make generate-all`.

## Quick Commands

```shell
make generate        # Extract schema from spec module, then run oapi-codegen
make generate-mocks  # Regenerate mock implementations
make generate-all    # Both of the above
```
