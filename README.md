# HyperFleet Adapter

Configuration-driven framework for cluster lifecycle management. An adapter instance listens for CloudEvents from a message broker, executes a four-phase pipeline (extract params, check preconditions, apply resources, report status), and reports results back to the HyperFleet API.

## Quick Start

### Try Locally

No cluster, broker, or API needed. Dry-run mode processes a CloudEvent from a JSON file using mock clients and prints a full execution trace:

```bash
go run ./cmd/adapter/main.go serve \
  --config test/testdata/dryrun/dryrun-kubernetes-adapter-config.yaml \
  --task-config test/testdata/dryrun/kubernetes/dryrun-kubernetes-task-config.yaml \
  --dry-run-event test/testdata/dryrun/event.json \
  --dry-run-api-responses test/testdata/dryrun/dryrun-api-responses.json \
  --dry-run-discovery test/testdata/dryrun/kubernetes/dryrun-kubernetes-discovery.json \
  --dry-run-verbose
```

See [Dry-Run Mode](./docs/development.md#dry-run-mode) for flags, mock file formats, and JSON output.

### Deploy the Full Stack

The adapter requires a running message broker and HyperFleet API. The [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) repository provides a one-command setup that deploys the complete HyperFleet stack including pre-configured adapters:

| Command | What it deploys |
|---------|-----------------|
| `make local-up-gcp` | GKE cluster + images + API + adapters + Maestro |
| `make install-hyperfleet` | Everything on an existing K8s cluster using RabbitMQ (no GCP needed) |
| `make install-adapters` | Install sample Hyperfleet Adapters |
| `make status` | Verify the deployment |

Make sure you define the following environment variables:
* `HELMFILE_ENV`: accepted values : `kind`, `gcp`
* `NAMESPACE`: namespace where HyperFleet components will be deployed
* `REGISTRY`: The registry namespace from which to pull the images. `quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet` for released images
* `API_IMAGE_TAG`: image tag for `hyperfleet-api` container image
* `SENTINEL_IMAGE_TAG`: image tag for `hyperfleet-sentinel` container image
* `ADAPTER_IMAGE_TAG`: image tag for `hyperfleet-adapter` container image

See [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) for required environment variables and full instructions.

## Documentation

### For Operators (deploying and running adapters)

- **[Deployment Guide](docs/deployment.md)** — configuration, Helm values, and deployment instructions
- **[Helm Values Reference](./charts/README.md)** — auto-generated chart values table
- **[Configuration Reference](docs/configuration.md)** — all deployment config fields, CLI flags, and env vars
- **[Metrics](docs/metrics.md)** — Prometheus metric definitions, labels, and PromQL queries
- **[Alerts](docs/alerts.md)** — recommended alert rules and monitoring queries
- **[Runbook](docs/runbook.md)** — failure modes, recovery procedures, and escalation paths

### For Adapter Authors (writing task configurations)

- **[Adapter Authoring Guide](docs/adapter-authoring-guide.md)** — params, preconditions, resources, CEL expressions, status reporting

### For Developers

- **[Development Guide](./docs/development.md)** - setup, build and test guidelines.
- **[CONTRIBUTING.md](./CONTRIBUTING.md)** - code style, testing requirements, PR process, and commit guidelines

### Architecture

- **[HyperFleet Architecture](https://github.com/openshift-hyperfleet/architecture)** — system architecture and API documentation
- **[HyperFleet API Spec](https://github.com/openshift-hyperfleet/hyperfleet-api-spec)** — OpenAPI specification
- **[Broker Library](https://github.com/openshift-hyperfleet/hyperfleet-broker)** — message broker abstraction

## CLI Reference

| Command | Description |
|---------|-------------|
| `adapter serve` | Start the adapter, subscribe to broker, and process events |
| `adapter config-dump` | Print the merged configuration and exit |
| `adapter version` | Print version, commit, and build date |

All `serve` flags have environment variable equivalents — run `adapter serve --help` for the full list.

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for code style, testing requirements, PR process, and commit guidelines.

All members of the **hyperfleet** team have write access. Code reviews and approvals are managed through the OWNERS file.

## License

This project is licensed under the Apache License 2.0 — see the [LICENSE](./LICENSE) file for details.
