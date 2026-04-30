# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- JWT authentication handler using `golang-jwt/jwt/v5` and `MicahParks/keyfunc/v3` with RS256 validation, configurable issuer and audience, and JWKS key rotation support ([#120](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/120))
- Hard deletion for Clusters and NodePools: resources and their adapter statuses are permanently removed from the database once all required adapters report `Finalized=True` and no child resources remain ([#119](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/119))
- `Finalized` condition aggregation with `WaitingForChildResources` intermediate state when all adapters are finalized but child node pools still exist ([#119](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/119))
- Soft deletion for Clusters and NodePools with `deleted_time` and `deleted_by` fields for tracking deletion requests ([#106](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/106))
- Aggregation logic for resource data ([#91](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/91))
- Version subcommand to CLI ([#84](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/84))
- Condition subfield queries for selective Sentinel polling ([#71](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/71))
- Slice field validation in SliceFilter ([#78](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/78), [#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- Connection retry logic for database sidecar startup coordination ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- pgbouncer sidecar to Helm chart for connection pooling ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- Health check ping timeout configuration ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- Request-level context timeout to transaction middleware ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- Connection pool timeout configuration ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- PostgreSQL advisory locks for migration coordination in multi-replica deployments ([#72](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/72))
- Search and filtering documentation ([#63](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/63))
- Connection pool and pgbouncer documentation ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- HyperFleet API operator guide ([#76](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/76))

### Changed

- Replaced OCM SDK authentication handler with standalone JWT middleware, removing `ocm-sdk-go` dependency and its transitive dependencies (`glog`, `bluemonday`, `json-iterator`) ([#120](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/120))
- Upgraded JWT library from `golang-jwt/jwt/v4` to `golang-jwt/jwt/v5` ([#120](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/120))
- Refactored `AdapterStatusDao.Upsert()` to accept a pre-fetched existing record, moving lookup and `LastTransitionTime` preservation logic to the service layer ([#119](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/119))
- Refactored DAO methods to remove Unscoped calls for fetching Clusters and NodePools ([#106](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/106))
- Bumped oapi-codegen version to fix missing `omitempty` on generated response objects ([#106](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/106))
- Updated OpenAPI spec with examples for Cluster and NodePool schemas ([#106](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/106))
- Standardized appVersion and image.tag handling in Helm chart ([#90](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/90))
- Aligned Helm chart with conventions standard ([#87](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/87))
- Streamlined configuration system with Viper, removed getters and _FILE suffix pattern ([#75](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/75))
- Used CHANGE_ME placeholder for image registry ([#83](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/83))

### Removed

- OCM SDK dependency (`ocm-sdk-go`), OCM client (`pkg/client/ocm/`), OCM configuration (`pkg/config/ocm.go`), OCM logger bridge (`pkg/logger/ocm_bridge.go`), and OCM authorization mocks ([#120](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/120))

### Fixed

- Validated adapter status conditions in handler layer ([#88](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/88))
- Removed org prefix from image.repository default ([#86](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/86))
- Addressed revive linter violations from enabled linting standard ([#85](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/85))
- Truncated migrations table in CleanDB to ensure migrations re-run ([#72](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/72))
- Added fallback default for AdvisoryLockTimeout ([#72](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/72))
- Rejected not operator for condition queries ([#80](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/80))
- SliceFilter star propagation in query processing ([#79](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/79))
- Used 0.0.0-dev version for dev image builds ([#77](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/77))
- Aligned health ping timeout with K8s probe timeout ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- Hardened pgbouncer config and health check responses ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))
- pgbouncer secret handling, connection leak, and lint ([#69](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/69))

## [0.1.1](https://github.com/openshift-hyperfleet/hyperfleet-api/compare/v0.1.0...v0.1.1) - 2026-03-09

### Added

- Test suite for presenter package ([#64](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/64))
- DatabaseConfig test coverage and improved advisory lock tests ([#72](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/72))
- Prometheus metrics with `hyperfleet_db_` prefix to database layer ([#58](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/58))

### Changed

- Updated copyright year to 2026 ([#58](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/58))
- Renamed metrics to use `hyperfleet_api_` prefix for consistency ([#57](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/57))
- Standardized Dockerfiles and Makefile for building images ([#59](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/59))

### Fixed

- CA certificates missing in ubi9-micro runtime image ([#74](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/74))
- VERSION collision with go-toolset base image ([#70](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/70))
- Config file resolution broken by -trimpath build flag ([#66](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/66))
- Enforced mandatory conditions in adapter status ([#60](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/60))
- SliceFilter usage in handlers and time field handling ([#64](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/64))
- Helm chart testing and default image registry ([#62](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/62))
- Reset and re-seed buildInfoMetric in ResetMetricCollectors ([#57](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/57))
- Rejected creation requests with missing spec field ([#56](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/56))

## [0.1.0](https://github.com/openshift-hyperfleet/hyperfleet-api/compare/c33867f...v0.1.0) - 2026-02-16

### Added

- PodDisruptionBudget to Helm chart ([#44](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/44))
- ServiceMonitor to Helm chart for Prometheus Operator integration ([#43](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/43))
- YAML table format for adapter requirements ([#41](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/41))
- Configurable adapter requirements ([#40](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/40))
- Condition-based search with GIN index for improved query performance ([#39](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/39))
- Health endpoints (`/healthz`, `/readyz`) and graceful shutdown ([#34](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/34))
- User-friendly search syntax with lowercase Base32 ID encoding ([#16](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/16))
- Schema validation for cluster and nodepool specifications ([#12](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/12))
- Generation field for NodePool management ([#22](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/22))
- OpenAPI schema embedded in Docker image for runtime validation ([#14](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/14))
- Helm chart for Kubernetes deployment ([#16](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/16))
- gomock/mockgen infrastructure for service mocks ([#10](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/10))
- Bingo for Go tool dependency management ([#9](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/9))
- Linux/amd64 platform support for container builds ([#17](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/17))
- Integration tests for conditions ([#39](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/39))
- Dynamic table discovery for test cleanup ([#32](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/32))
- Operational runbook and metrics documentation ([#45](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/45))
- ServiceMonitor configuration documentation ([#43](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/43))
- Params constraints documentation ([#55](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/55))

### Changed

- BREAKING CHANGE: Updated OpenAPI spec for conditions-based status model ([#39](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/39))
- Aligned cluster and nodepool name validation with CS rules ([#48](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/48))
- Implemented RFC 9457 Problem Details error model for standardized error responses ([#37](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/37))
- Migrated to oapi-codegen for OpenAPI code generation ([#33](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/33))
- Aligned logging with HyperFleet structured logging specification ([#31](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/31))
- Integrated database logging with LOG_LEVEL ([#35](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/35))
- Renamed Makefile binary target to build with output to bin/ ([#30](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/30))
- Consolidated and streamlined documentation structure ([#21](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/21))
- Configured rh-hooks-ai for AI-readiness and security compliance ([#18](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/18))
- Migrated to HyperFleet v2 architecture ([#3](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/3))

### Removed

- Phase validation from status types in favor of conditions-based model ([#39](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/39))
- Generated mock files from git tracking ([#10](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/10))
- Generated OpenAPI code from git tracking ([#3](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/3))
- .claude directory from git tracking ([#45](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/45))

### Fixed

- Prevented duplicate nodepool names within a cluster ([#53](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/53))
- Returned 404 for non-existent cluster statuses ([#54](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/54))
- First adapter status report now correctly initializes with Available=Unknown ([#52](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/52))
- Integration tests updated to match new first-report behavior ([#52](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/52))
- Added timeout to testcontainer teardown to prevent Prow hang ([#52](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/52))
- Centralized adapter config to avoid duplicate logs ([#46](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/46))
- Avoided exposing secret values in runbook ([#45](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/45))
- Made adapter configuration mandatory ([#46](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/46))
- Used explicit nil checks for PDB values ([#44](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/44))
- Fixed goconst, gocritic, gosec, unparam and lll lint issues ([#42](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/42))
- Prevented fmt.Sprintf panic when reason contains % without values ([#37](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/37))
- Avoided leaking database error details to API clients ([#37](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/37))
- Omitted empty Instance and TraceId from Problem Details JSON ([#37](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/37))
- Added missing error codes to errorDefinitions map ([#37](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/37))
- MVP phase logic to only return Ready or NotReady ([#9](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/9))
- Cluster and nodepool name validation ([#16](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/16))
- Silent error suppression ([#26](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/26))
- Propagated JSON unmarshal errors ([#26](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/26))
- Lint failures in presubmit jobs ([#12](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/12), [#6](https://github.com/openshift-hyperfleet/hyperfleet-api/pull/6))

### Security

[Unreleased]: https://github.com/openshift-hyperfleet/hyperfleet-api/compare/v0.1.1...HEAD
