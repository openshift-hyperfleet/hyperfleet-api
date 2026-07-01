# hyperfleet-api

![Version: 1.1.0](https://img.shields.io/badge/Version-1.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.0-dev](https://img.shields.io/badge/AppVersion-0.0.0--dev-informational?style=flat-square)

HyperFleet API - Cluster Lifecycle Management Service

**Homepage:** <https://github.com/openshift-hyperfleet/hyperfleet-api>

## Installation

```bash
helm install hyperfleet-api oci://REGISTRY/hyperfleet-api \
  --set image.registry=REGISTRY \
  --set image.repository=ORG/hyperfleet-api \
  --set image.tag=<version>
```

## Database Modes

| Mode | When to use | Configuration |
|------|-------------|---------------|
| **Built-in PostgreSQL** | Development / testing | `database.postgresql.enabled=true` (default) |
| **External database** | Production | `database.external.enabled=true`, `database.external.secretName=<secret>` |

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| HyperFleet Team | <hyperfleet-team@redhat.com> |  |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| replicaCount | int | `1` | Number of API server replicas |
| image.registry | string | `"CHANGE_ME"` | Container image registry (no default — must be set) |
| image.repository | string | `"CHANGE_ME"` | Container image repository (no default — must be set) |
| image.pullPolicy | string | `"Always"` | Image pull policy |
| image.tag | string | `""` | Image tag (no default — must be set via `--set image.tag=<version>`) |
| imagePullSecrets | list | `[]` | Secrets for pulling images from private registries |
| nameOverride | string | `""` | Override the chart name used in resource names |
| fullnameOverride | string | `""` | Override the full release name used in resource names |
| ports | object | `{"api":8000,"health":8080,"metrics":9090}` | Container ports exposed by the API server. These must match the corresponding application config values (`config.server.port`, `config.health.port`, `config.metrics.port`). |
| ports.api | int | `8000` | API server port |
| ports.health | int | `8080` | Health check endpoint port |
| ports.metrics | int | `9090` | Prometheus metrics endpoint port |
| config | object | `{"adapters":{"required":{"cluster":[],"nodepool":[]}},"database":{"debug":false,"dialect":"postgres","host":"","name":"hyperfleet","pool":{"conn_max_idle_time":"1m","conn_max_lifetime":"5m","conn_retry_attempts":10,"conn_retry_interval":"3s","max_connections":50,"max_idle_connections":10,"request_timeout":"30s"},"port":5432,"ssl":{"mode":"disable","root_cert_file":""}},"existingConfigMap":"","health":{"db_ping_timeout":"2s","host":"0.0.0.0","port":8080,"shutdown_timeout":"20s","tls":{"enabled":false}},"logging":{"format":"json","level":"info","masking":{"enabled":true,"fields":["password","secret","token","api_key","access_token","refresh_token","client_secret"],"headers":["Authorization","X-API-Key","Cookie","X-Auth-Token","X-Forwarded-Authorization","X-HyperFleet-Identity"]},"otel":{"enabled":false},"output":"stdout"},"metrics":{"host":"0.0.0.0","label_metrics_inclusion_duration":"168h","port":9090,"reconciliation_stuck_threshold":"10m","tls":{"enabled":false}},"server":{"host":"0.0.0.0","hostname":"","identity_header":"","jwk":{"cert_file":"","cert_url":""},"jwt":{"audience":"","enabled":false,"identity_claim":"email","issuer_url":""},"port":8000,"timeouts":{"read":"5s","write":"30s"},"tls":{"cert_file":"","enabled":false,"key_file":""}}}` | Application configuration. All settings in this section generate the ConfigMap consumed by the API server. Set `config.existingConfigMap` to use a pre-existing ConfigMap instead. |
| config.existingConfigMap | string | `""` | Use an existing ConfigMap instead of generating one. When set, all other `config.*` values are ignored. |
| config.server | object | `{"host":"0.0.0.0","hostname":"","identity_header":"","jwk":{"cert_file":"","cert_url":""},"jwt":{"audience":"","enabled":false,"identity_claim":"email","issuer_url":""},"port":8000,"timeouts":{"read":"5s","write":"30s"},"tls":{"cert_file":"","enabled":false,"key_file":""}}` | HTTP server settings |
| config.server.hostname | string | `""` | Public hostname advertised by the API (leave empty for auto-detect) |
| config.server.host | string | `"0.0.0.0"` | Listen address |
| config.server.port | int | `8000` | Listen port (must match `ports.api`) |
| config.server.timeouts | object | `{"read":"5s","write":"30s"}` | Request timeout settings |
| config.server.timeouts.read | string | `"5s"` | HTTP read timeout |
| config.server.timeouts.write | string | `"30s"` | HTTP write timeout |
| config.server.tls | object | `{"cert_file":"","enabled":false,"key_file":""}` | TLS configuration for the API server |
| config.server.tls.enabled | bool | `false` | Enable TLS on the API listener |
| config.server.tls.cert_file | string | `""` | Path to TLS certificate file |
| config.server.tls.key_file | string | `""` | Path to TLS key file |
| config.server.jwt | object | `{"audience":"","enabled":false,"identity_claim":"email","issuer_url":""}` | JWT authentication settings |
| config.server.jwt.enabled | bool | `false` | Enable JWT authentication |
| config.server.jwt.issuer_url | string | `""` | OIDC issuer URL for token validation |
| config.server.jwt.audience | string | `""` | Expected JWT audience claim |
| config.server.jwt.identity_claim | string | `"email"` | JWT claim used as the caller identity |
| config.server.identity_header | string | `""` | HTTP header used to pass caller identity (bypasses JWT when set) |
| config.server.jwk | object | `{"cert_file":"","cert_url":""}` | JWK settings for token verification |
| config.server.jwk.cert_file | string | `""` | Path to a local JWK certificate file |
| config.server.jwk.cert_url | string | `""` | URL to fetch JWK certificates from |
| config.database | object | `{"debug":false,"dialect":"postgres","host":"","name":"hyperfleet","pool":{"conn_max_idle_time":"1m","conn_max_lifetime":"5m","conn_retry_attempts":10,"conn_retry_interval":"3s","max_connections":50,"max_idle_connections":10,"request_timeout":"30s"},"port":5432,"ssl":{"mode":"disable","root_cert_file":""}}` | Database connection settings. Credentials must be provided via a Secret — see `database.external.secretName` or use the built-in PostgreSQL (`database.postgresql.enabled`). |
| config.database.dialect | string | `"postgres"` | SQL dialect |
| config.database.host | string | `""` | Database host (auto-set when using the built-in PostgreSQL) |
| config.database.port | int | `5432` | Database port |
| config.database.name | string | `"hyperfleet"` | Database name |
| config.database.debug | bool | `false` | Enable SQL debug logging |
| config.database.ssl | object | `{"mode":"disable","root_cert_file":""}` | SSL / TLS settings for the database connection |
| config.database.ssl.mode | string | `"disable"` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |
| config.database.ssl.root_cert_file | string | `""` | Path to the CA root certificate |
| config.database.pool | object | `{"conn_max_idle_time":"1m","conn_max_lifetime":"5m","conn_retry_attempts":10,"conn_retry_interval":"3s","max_connections":50,"max_idle_connections":10,"request_timeout":"30s"}` | Connection pool tuning |
| config.database.pool.max_connections | int | `50` | Maximum number of open connections |
| config.database.pool.max_idle_connections | int | `10` | Maximum number of idle connections |
| config.database.pool.conn_max_lifetime | string | `"5m"` | Maximum lifetime of a connection |
| config.database.pool.conn_max_idle_time | string | `"1m"` | Maximum idle time before a connection is closed |
| config.database.pool.request_timeout | string | `"30s"` | Timeout for acquiring a connection from the pool |
| config.database.pool.conn_retry_attempts | int | `10` | Number of connection retry attempts on startup |
| config.database.pool.conn_retry_interval | string | `"3s"` | Interval between connection retry attempts |
| config.logging | object | `{"format":"json","level":"info","masking":{"enabled":true,"fields":["password","secret","token","api_key","access_token","refresh_token","client_secret"],"headers":["Authorization","X-API-Key","Cookie","X-Auth-Token","X-Forwarded-Authorization","X-HyperFleet-Identity"]},"otel":{"enabled":false},"output":"stdout"}` | Logging configuration |
| config.logging.level | string | `"info"` | Log level (`debug`, `info`, `warn`, `error`) |
| config.logging.format | string | `"json"` | Log format (`json` or `text`) |
| config.logging.output | string | `"stdout"` | Log output destination |
| config.logging.otel | object | `{"enabled":false}` | OpenTelemetry tracing integration. See the [tracing standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/tracing.md#configuration). |
| config.logging.otel.enabled | bool | `false` | Enable OpenTelemetry log correlation |
| config.logging.masking | object | `{"enabled":true,"fields":["password","secret","token","api_key","access_token","refresh_token","client_secret"],"headers":["Authorization","X-API-Key","Cookie","X-Auth-Token","X-Forwarded-Authorization","X-HyperFleet-Identity"]}` | Sensitive-data masking for logs |
| config.logging.masking.enabled | bool | `true` | Enable log masking |
| config.logging.masking.headers | list | `["Authorization","X-API-Key","Cookie","X-Auth-Token","X-Forwarded-Authorization","X-HyperFleet-Identity"]` | HTTP headers whose values are redacted in logs |
| config.logging.masking.fields | list | `["password","secret","token","api_key","access_token","refresh_token","client_secret"]` | Field names whose values are redacted in logs |
| config.metrics | object | `{"host":"0.0.0.0","label_metrics_inclusion_duration":"168h","port":9090,"reconciliation_stuck_threshold":"10m","tls":{"enabled":false}}` | Prometheus metrics endpoint settings |
| config.metrics.host | string | `"0.0.0.0"` | Listen address (must be `0.0.0.0` for in-cluster access) |
| config.metrics.port | int | `9090` | Listen port (must match `ports.metrics`) |
| config.metrics.tls | object | `{"enabled":false}` | TLS configuration for the metrics endpoint |
| config.metrics.tls.enabled | bool | `false` | Enable TLS on the metrics endpoint |
| config.metrics.label_metrics_inclusion_duration | string | `"168h"` | Duration window for label-based metric inclusion |
| config.metrics.reconciliation_stuck_threshold | string | `"10m"` | Threshold after which a pending reconciliation is considered stuck |
| config.health | object | `{"db_ping_timeout":"2s","host":"0.0.0.0","port":8080,"shutdown_timeout":"20s","tls":{"enabled":false}}` | Health check endpoint settings |
| config.health.host | string | `"0.0.0.0"` | Listen address (must be `0.0.0.0` for probe access) |
| config.health.port | int | `8080` | Listen port (must match `ports.health`) |
| config.health.tls | object | `{"enabled":false}` | TLS configuration for the health endpoint |
| config.health.tls.enabled | bool | `false` | Enable TLS on the health endpoint |
| config.health.shutdown_timeout | string | `"20s"` | Graceful shutdown timeout |
| config.health.db_ping_timeout | string | `"2s"` | Timeout for the database liveness ping |
| config.adapters | object | `{"required":{"cluster":[],"nodepool":[]}}` | Adapters required for resources to reach "Ready" state. Production deployments should list all expected adapters. |
| config.adapters.required | object | `{"cluster":[],"nodepool":[]}` | Adapters required for cluster resources |
| config.adapters.required.cluster | list | `[]` | Required cluster adapters (e.g. `["validation", "dns", "pullsecret", "hypershift"]`) |
| config.adapters.required.nodepool | list | `[]` | Required nodepool adapters (e.g. `["validation", "hypershift"]`) |
| serviceAccount | object | `{"annotations":{},"create":true,"name":""}` | ServiceAccount configuration |
| serviceAccount.create | bool | `true` | Create a ServiceAccount for the API server |
| serviceAccount.annotations | object | `{}` | Annotations added to the ServiceAccount (e.g. for Workload Identity) |
| serviceAccount.name | string | `""` | Override the ServiceAccount name (defaults to the release fullname) |
| podAnnotations | object | `{}` | Additional annotations applied to all pods |
| podLabels | object | `{}` | Additional labels applied to all pods |
| podSecurityContext | object | `{"fsGroup":65532,"runAsNonRoot":true,"runAsUser":65532}` | Pod-level security context |
| podSecurityContext.fsGroup | int | `65532` | Filesystem group for volume mounts |
| podSecurityContext.runAsNonRoot | bool | `true` | Run all containers as non-root |
| podSecurityContext.runAsUser | int | `65532` | UID for all containers |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Container-level security context |
| securityContext.allowPrivilegeEscalation | bool | `false` | Disallow privilege escalation |
| securityContext.readOnlyRootFilesystem | bool | `true` | Mount root filesystem as read-only |
| service | object | `{"type":"ClusterIP"}` | Kubernetes Service configuration |
| service.type | string | `"ClusterIP"` | Service type (`ClusterIP`, `LoadBalancer`, `NodePort`) |
| resources | object | `{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | CPU and memory resource requests and limits |
| lifecycle | object | `{"preStop":{"exec":{"command":["/bin/sh","-c","sleep 5"]}}}` | Container lifecycle hooks. Use `preStop` to delay SIGTERM during rolling updates, giving the LoadBalancer time to drain the old pod. See HYPERFLEET-1306. |
| strategy | object | `{"rollingUpdate":{"maxSurge":1,"maxUnavailable":0},"type":"RollingUpdate"}` | Deployment rollout strategy. `maxUnavailable: 0` ensures zero-downtime during rolling updates — the old pod stays until the new one is Ready. |
| terminationGracePeriodSeconds | int | `70` | Seconds Kubernetes waits after SIGTERM before SIGKILL. Shutdown sequence is sequential: preStop sleep (5s) + health server (20s max) + API server (10s max) + metrics server (10s max) + OTel (20s max) + DB close (no timeout). Typical shutdown is ~8s total; worst case exceeds 60s. Set to 70s to cover worst case with buffer. If the DB close hangs beyond this, Kubernetes sends SIGKILL — acceptable as a last resort. |
| nodeSelector | object | `{}` | Node selector constraints for pod scheduling |
| tolerations | list | `[]` | Tolerations for pod scheduling |
| affinity | object | `{}` | Affinity rules for pod scheduling |
| autoscaling | object | `{"enabled":false,"maxReplicas":10,"minReplicas":1,"targetCPUUtilizationPercentage":80,"targetMemoryUtilizationPercentage":80}` | Horizontal Pod Autoscaler configuration |
| autoscaling.enabled | bool | `false` | Enable the HPA |
| autoscaling.minReplicas | int | `1` | Minimum number of replicas |
| autoscaling.maxReplicas | int | `10` | Maximum number of replicas |
| autoscaling.targetCPUUtilizationPercentage | int | `80` | Target CPU utilization percentage |
| autoscaling.targetMemoryUtilizationPercentage | int | `80` | Target memory utilization percentage |
| podDisruptionBudget | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget configuration |
| podDisruptionBudget.enabled | bool | `false` | Enable the PDB |
| podDisruptionBudget.minAvailable | int | `1` | Minimum number of available pods during disruption |
| database | object | `{"external":{"enabled":false,"secretName":""},"postgresql":{"database":"hyperfleet","enabled":true,"image":"docker.io/library/postgres:14.23","password":"hyperfleet-dev-password","persistence":{"enabled":false,"size":"1Gi","storageClass":""},"port":5432,"resources":{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"100m","memory":"128Mi"}},"user":"hyperfleet"}}` | Database infrastructure settings. For **production**, set `database.external.enabled=true` and supply a secret with connection details. For **development**, the built-in PostgreSQL pod is enabled by default. |
| database.external | object | `{"enabled":false,"secretName":""}` | External database configuration (production) |
| database.external.enabled | bool | `false` | Use an external database instead of the built-in PostgreSQL |
| database.external.secretName | string | `""` | Name of an existing Secret with keys: `db.host`, `db.port`, `db.name`, `db.user`, `db.password` |
| database.postgresql | object | `{"database":"hyperfleet","enabled":true,"image":"docker.io/library/postgres:14.23","password":"hyperfleet-dev-password","persistence":{"enabled":false,"size":"1Gi","storageClass":""},"port":5432,"resources":{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"100m","memory":"128Mi"}},"user":"hyperfleet"}` | Built-in PostgreSQL for development and testing |
| database.postgresql.enabled | bool | `true` | Deploy a single-pod PostgreSQL instance |
| database.postgresql.image | string | `"docker.io/library/postgres:14.23"` | PostgreSQL container image |
| database.postgresql.database | string | `"hyperfleet"` | Database name |
| database.postgresql.user | string | `"hyperfleet"` | Database user |
| database.postgresql.password | string | `"hyperfleet-dev-password"` | Database password (**development only** — use a Secret in production) |
| database.postgresql.port | int | `5432` | PostgreSQL listen port |
| database.postgresql.resources | object | `{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Resource requests and limits for the PostgreSQL pod |
| database.postgresql.persistence | object | `{"enabled":false,"size":"1Gi","storageClass":""}` | Persistent volume configuration for PostgreSQL data |
| database.postgresql.persistence.enabled | bool | `false` | Enable persistent storage (uses emptyDir when disabled) |
| database.postgresql.persistence.size | string | `"1Gi"` | Volume size |
| database.postgresql.persistence.storageClass | string | `""` | StorageClass name (empty for cluster default) |
| monitoring | object | `{"podMonitoring":{"additionalLabels":{},"enabled":false,"interval":"30s","metricRelabeling":[],"tlsConfig":{"insecureSkipVerify":false}},"prometheusRule":{"additionalLabels":{},"enabled":false,"namespace":"","rules":{"reconciliationStuck":{"for":"5m","runbookUrl":""},"reconciliationTimeout":{"durationSeconds":1800,"for":"5m","runbookUrl":""}}}}` | Monitoring and alerting configuration |
| monitoring.podMonitoring | object | `{"additionalLabels":{},"enabled":false,"interval":"30s","metricRelabeling":[],"tlsConfig":{"insecureSkipVerify":false}}` | PodMonitoring for Google Managed Prometheus (GMP) scraping |
| monitoring.podMonitoring.enabled | bool | `false` | Create a PodMonitoring resource |
| monitoring.podMonitoring.interval | string | `"30s"` | Scrape interval |
| monitoring.podMonitoring.additionalLabels | object | `{}` | Additional labels for the PodMonitoring resource |
| monitoring.podMonitoring.metricRelabeling | list | `[]` | Metric relabel configs to apply to samples before ingestion |
| monitoring.podMonitoring.tlsConfig | object | `{"insecureSkipVerify":false}` | TLS configuration when config.metrics.tls.enabled=true |
| monitoring.podMonitoring.tlsConfig.insecureSkipVerify | bool | `false` | Disable target certificate validation (e.g. for self-signed certs) |
| monitoring.prometheusRule | object | `{"additionalLabels":{},"enabled":false,"namespace":"","rules":{"reconciliationStuck":{"for":"5m","runbookUrl":""},"reconciliationTimeout":{"durationSeconds":1800,"for":"5m","runbookUrl":""}}}` | PrometheusRule for alerting |
| monitoring.prometheusRule.enabled | bool | `false` | Create PrometheusRule resources |
| monitoring.prometheusRule.additionalLabels | object | `{}` | Additional labels for PrometheusRule discovery |
| monitoring.prometheusRule.namespace | string | `""` | Namespace to create the PrometheusRule in (defaults to release namespace) |
| monitoring.prometheusRule.rules | object | `{"reconciliationStuck":{"for":"5m","runbookUrl":""},"reconciliationTimeout":{"durationSeconds":1800,"for":"5m","runbookUrl":""}}` | Alert rule configuration |
| monitoring.prometheusRule.rules.reconciliationStuck | object | `{"for":"5m","runbookUrl":""}` | Alert when reconciliation is stuck |
| monitoring.prometheusRule.rules.reconciliationStuck.for | string | `"5m"` | Duration before the alert fires |
| monitoring.prometheusRule.rules.reconciliationStuck.runbookUrl | string | `""` | Runbook URL included in the alert |
| monitoring.prometheusRule.rules.reconciliationTimeout | object | `{"durationSeconds":1800,"for":"5m","runbookUrl":""}` | Alert when reconciliation exceeds timeout (based on actual stuck duration, survives Prometheus restarts) |
| monitoring.prometheusRule.rules.reconciliationTimeout.durationSeconds | int | `1800` | Stuck duration in seconds that triggers the critical alert |
| monitoring.prometheusRule.rules.reconciliationTimeout.for | string | `"5m"` | Stabilization window before firing (short — the duration check is the real gate) |
| monitoring.prometheusRule.rules.reconciliationTimeout.runbookUrl | string | `""` | Runbook URL included in the alert |
| serviceMonitor | object | `{"enabled":false,"interval":"30s","labels":{},"namespace":"","scrapeTimeout":"10s"}` | ServiceMonitor for Prometheus Operator scrape configuration |
| serviceMonitor.enabled | bool | `false` | Create a ServiceMonitor resource |
| serviceMonitor.interval | string | `"30s"` | Scrape interval |
| serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout |
| serviceMonitor.labels | object | `{}` | Additional labels for ServiceMonitor discovery |
| serviceMonitor.namespace | string | `""` | Namespace to create the ServiceMonitor in (defaults to release namespace) |
| tracing | object | `{"enabled":false,"otlpEndpoint":"","otlpProtocol":"grpc","propagators":"tracecontext,baggage","sampler":"parentbased_traceidratio","samplerArg":"1.0","serviceName":"hyperfleet-api"}` | Distributed tracing configuration (OpenTelemetry) |
| tracing.enabled | bool | `false` | Enable trace export |
| tracing.serviceName | string | `"hyperfleet-api"` | Service name reported in traces |
| tracing.otlpEndpoint | string | `""` | OTLP exporter endpoint (traces go to stdout when empty) |
| tracing.otlpProtocol | string | `"grpc"` | OTLP protocol (`grpc` or `http/protobuf`) |
| tracing.sampler | string | `"parentbased_traceidratio"` | Sampler type |
| tracing.samplerArg | string | `"1.0"` | Sampling rate (`1.0` for dev, `0.01` for production) |
| tracing.propagators | string | `"tracecontext,baggage"` | Context propagation formats |
| nativeSidecars | list | `[]` | Native sidecar containers (Kubernetes 1.28+). Native sidecars are init containers with `restartPolicy: Always` — they start before other init containers and keep running throughout the pod lifecycle. Use this for database proxies that must be available during `db-migrate`. Each entry is a full Kubernetes container spec. |
| sidecars | list | `[]` | Regular sidecar containers. These start after init containers complete. Use `nativeSidecars` above for containers that must be available during init (e.g. database proxies). Each entry is a full Kubernetes container spec. |
| validationSchema | object | `{"content":"","enabled":false,"existingConfigMap":""}` | Validation schema configuration. Supply a custom OpenAPI schema for cluster/nodepool spec validation. When enabled, the schema is mounted into the container and every create/update request is validated against it. The API will fail to start if the schema is invalid. |
| validationSchema.enabled | bool | `false` | Enable spec validation |
| validationSchema.existingConfigMap | string | `""` | Use an existing ConfigMap (must contain an `openapi.yaml` key). When set, `validationSchema.content` is ignored. |
| validationSchema.content | string | `""` | Inline OpenAPI 3.0 schema content. Must define `ClusterSpec` and `NodePoolSpec` under `components.schemas`. |
| extraEnv | list | `[]` | Additional environment variables injected into the API container. Use sparingly — prefer `config.*` values above. |
| extraVolumeMounts | list | `[]` | Extra volume mounts added to the API container |
| extraVolumes | list | `[]` | Extra volumes added to the pod |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs](https://github.com/norwoodj/helm-docs)
