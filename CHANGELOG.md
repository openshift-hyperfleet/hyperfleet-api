# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Aggregation logic for resource data ([#91](https://github.com/rh-amarin/hyperfleet-api/pull/91))
- Version subcommand to CLI ([#84](https://github.com/rh-amarin/hyperfleet-api/pull/84))
- Condition subfield queries for selective Sentinel polling ([#71](https://github.com/rh-amarin/hyperfleet-api/pull/71))
- Slice field validation in SliceFilter ([#78](https://github.com/rh-amarin/hyperfleet-api/pull/78))
- Prometheus metrics with `hyperfleet_db_` prefix to database layer ([#58](https://github.com/rh-amarin/hyperfleet-api/pull/58))
- PodDisruptionBudget to Helm chart ([#44](https://github.com/rh-amarin/hyperfleet-api/pull/44))
- ServiceMonitor to Helm chart for Prometheus Operator integration ([#42](https://github.com/rh-amarin/hyperfleet-api/pull/42))
- Configurable adapter requirements ([#40](https://github.com/rh-amarin/hyperfleet-api/pull/40))
- Condition-based search with GIN index for improved query performance ([#39](https://github.com/rh-amarin/hyperfleet-api/pull/39))
- Health endpoints (`/healthz`, `/readyz`) and graceful shutdown ([#34](https://github.com/rh-amarin/hyperfleet-api/pull/34))
- User-friendly search syntax with lowercase Base32 ID encoding ([#16](https://github.com/rh-amarin/hyperfleet-api/pull/16))
- Schema validation for cluster and nodepool specifications ([#12](https://github.com/rh-amarin/hyperfleet-api/pull/12))
- Generation field for NodePool management ([#22](https://github.com/rh-amarin/hyperfleet-api/pull/22))
- Connection retry logic for database sidecar startup coordination ([#69](https://github.com/rh-amarin/hyperfleet-api/pull/69))
- pgbouncer sidecar to Helm chart for connection pooling ([#69](https://github.com/rh-amarin/hyperfleet-api/pull/69))
- Health check ping timeout configuration ([#69](https://github.com/rh-amarin/hyperfleet-api/pull/69))
- Request-level context timeout to transaction middleware ([#69](https://github.com/rh-amarin/hyperfleet-api/pull/69))
- Connection pool timeout configuration ([#69](https://github.com/rh-amarin/hyperfleet-api/pull/69))
- PostgreSQL advisory locks for migration coordination in multi-replica deployments ([#72](https://github.com/rh-amarin/hyperfleet-api/pull/72))
- OpenAPI schema embedded in Docker image for runtime validation ([#14](https://github.com/rh-amarin/hyperfleet-api/pull/14))
- Helm chart for Kubernetes deployment ([#16](https://github.com/rh-amarin/hyperfleet-api/pull/16))

### Changed

- BREAKING CHANGE: Updated OpenAPI spec for conditions-based status model ([#39](https://github.com/rh-amarin/hyperfleet-api/pull/39))
- Streamlined configuration system with Viper, removed getters and _FILE suffix pattern ([#75](https://github.com/rh-amarin/hyperfleet-api/pull/75))
- Renamed metrics to use `hyperfleet_api_` prefix for consistency ([#57](https://github.com/rh-amarin/hyperfleet-api/pull/57))
- Aligned cluster and nodepool name validation with CS rules ([#48](https://github.com/rh-amarin/hyperfleet-api/pull/48))
- Implemented RFC 9457 Problem Details error model for standardized error responses ([#37](https://github.com/rh-amarin/hyperfleet-api/pull/37))
- Migrated to oapi-codegen for OpenAPI code generation ([#33](https://github.com/rh-amarin/hyperfleet-api/pull/33))
- Aligned logging with HyperFleet structured logging specification ([#31](https://github.com/rh-amarin/hyperfleet-api/pull/31))
- Migrated to HyperFleet v2 architecture ([#3](https://github.com/rh-amarin/hyperfleet-api/pull/3))

### Deprecated

### Removed

- Removed phase validation from status types in favor of conditions-based model ([#39](https://github.com/rh-amarin/hyperfleet-api/pull/39))

### Fixed

- Validated adapter status conditions in handler layer ([#88](https://github.com/rh-amarin/hyperfleet-api/pull/88))
- Rejected not operator for condition queries ([#80](https://github.com/rh-amarin/hyperfleet-api/pull/80))
- SliceFilter star propagation in query processing ([#79](https://github.com/rh-amarin/hyperfleet-api/pull/79))
- CA certificates missing in ubi9-micro runtime image ([#74](https://github.com/rh-amarin/hyperfleet-api/pull/74))
- Config file resolution broken by -trimpath build flag ([#66](https://github.com/rh-amarin/hyperfleet-api/pull/66))
- Enforced mandatory conditions in adapter status ([#60](https://github.com/rh-amarin/hyperfleet-api/pull/60))
- SliceFilter usage in handlers and time field handling ([#64](https://github.com/rh-amarin/hyperfleet-api/pull/64))
- Rejected creation requests with missing spec field ([#56](https://github.com/rh-amarin/hyperfleet-api/pull/56))
- Prevented duplicate nodepool names within a cluster ([#53](https://github.com/rh-amarin/hyperfleet-api/pull/53))
- Returned 404 for non-existent cluster statuses ([#54](https://github.com/rh-amarin/hyperfleet-api/pull/54))
- First adapter status report now correctly initializes with Available=Unknown ([#52](https://github.com/rh-amarin/hyperfleet-api/pull/52))
- Made adapter configuration mandatory ([#46](https://github.com/rh-amarin/hyperfleet-api/pull/46))
- Prevented fmt.Sprintf panic when reason contains % without values ([#37](https://github.com/rh-amarin/hyperfleet-api/pull/37))
- Avoided leaking database error details to API clients ([#37](https://github.com/rh-amarin/hyperfleet-api/pull/37))
- Cluster and nodepool name validation ([#16](https://github.com/rh-amarin/hyperfleet-api/pull/16))

### Security

[Unreleased]: https://github.com/rh-amarin/hyperfleet-api/compare/c33867f...HEAD
