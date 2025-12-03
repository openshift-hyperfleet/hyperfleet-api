# Prerequisites

hyperfleet-api requires the following tools to be pre-installed:

## Go

`Go` is an open-source programming language that makes it easy to build simple, reliable, and efficient software.

- **Purpose**: Required for building and running the `hyperfleet` binary
- **Version**: Go 1.24 or higher (FIPS-compliant crypto support)
- **Installation**: Install Go from the [official Go website](https://golang.org/dl/)
- **Verification**: Run `go version` to verify installation

## Podman

`Podman` is a daemonless container engine for developing, managing, and running OCI containers.

- **Purpose**: Used for running PostgreSQL database locally and for code generation (openapi-generator-cli)
- **Installation**:
  - Podman: [https://podman.io/getting-started/installation](https://podman.io/getting-started/installation)

## PostgreSQL Client Tools

PostgreSQL client tools provide the `psql` command-line interface for database interaction.

- **Purpose**: Required for `make db/login` to connect to the database and inspect schema
- **Installation**:
  - macOS: `brew install postgresql`
  - Ubuntu: `apt-get install postgresql-client`
  - Fedora: `dnf install postgresql`
- **Note**: The PostgreSQL server itself runs in a container via `make db/setup`

## jq

`jq` is a lightweight and flexible command-line JSON processor.

- **Purpose**: Useful for parsing JSON outputs from API calls and commands
- **Installation**: Follow the instructions on the [jq official website](https://jqlang.github.io/jq/)
- **Verification**: Run `jq --version`

## ocm CLI (Optional)

`ocm` stands for OpenShift Cluster Manager CLI and is used for authentication in production mode.

- **Purpose**: CLI tool for authenticating with OCM and making authenticated API requests
- **Installation**: Refer to the [OCM CLI documentation](https://github.com/openshift-online/ocm-cli)
- **Note**: Only required when running with authentication enabled (production mode)
- **Development**: For local development, use `make run-no-auth` which bypasses authentication

## Quick Verification

Run these commands to verify all prerequisites are installed:

```bash
# Required tools
go version              # Should show 1.24 or higher
podman --version
psql --version          # PostgreSQL client
jq --version            # JSON processor

# Optional tools
ocm version             # OCM CLI (production auth only)
```

## Getting Started

Once all prerequisites are installed, follow the development workflow in README.md:

```bash
# Generate OpenAPI code (required before go mod download)
make generate

# Install Go dependencies
go mod download

# Initialize configuration
make secrets

# Start database
make db/setup

# Build and run
make binary
./hyperfleet-api migrate
make run-no-auth
```
