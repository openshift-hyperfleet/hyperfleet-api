.DEFAULT_GOAL := help

# Include bingo-managed tool variables
include .bingo/Variables.mk

# CGO_ENABLED=0 is not FIPS compliant. large commercial vendors and FedRAMP require FIPS compliant crypto
# Use ?= to allow Dockerfile to override (CGO_ENABLED=0 for Alpine-based dev images)
CGO_ENABLED ?= 1

GO ?= go

# Auto-detect container tool (podman preferred when available)
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

.PHONY: check-container-tool
check-container-tool:
ifndef CONTAINER_TOOL
	@echo "Error: No container tool found (podman or docker)"
	@echo ""
	@echo "Please install one of:"
	@echo "  brew install podman   # macOS"
	@echo "  brew install docker   # macOS"
	@echo "  dnf install podman    # Fedora/RHEL"
	@exit 1
endif

# Version information
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY ?= $(shell [ -z "$$(git status --porcelain 2>/dev/null)" ] || echo "-modified")
APP_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build flags
GOFLAGS ?= -trimpath
LDFLAGS := -s -w \
           -X github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.Version=$(APP_VERSION) \
           -X github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.Commit=$(GIT_SHA) \
           -X 'github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.BuildTime=$(BUILD_DATE)'

# =============================================================================
# Image Configuration
# =============================================================================
IMAGE_REGISTRY ?= quay.io/openshift-hyperfleet
IMAGE_NAME ?= hyperfleet-api
IMAGE_TAG ?= $(APP_VERSION)

PLATFORM ?= linux/amd64

# Dev image configuration - set QUAY_USER to push to personal registry
# Usage: QUAY_USER=myuser make image-dev
QUAY_USER ?=
DEV_TAG ?= dev-$(GIT_SHA)
DEV_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi-minimal:latest

# Encourage consistent tool versions
OPENAPI_GENERATOR_VERSION := 5.4.0
GO_VERSION := go1.25.

# Database connection details
db_name := hyperfleet
db_port := 5432
db_user := hyperfleet
db_password := foobar-bizz-buzz
db_sslmode := disable
db_image ?= docker.io/library/postgres:14.23

# Test output files
unit_test_json_output ?= ${PWD}/unit-test-results.json
integration_test_json_output ?= ${PWD}/integration-test-results.json

### Environment-sourced variables with defaults
ifndef HYPERFLEET_ENV
	HYPERFLEET_ENV := development
endif

ifndef TEST_SUMMARY_FORMAT
	TEST_SUMMARY_FORMAT = short-verbose
endif


.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_\/-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Code Quality

.PHONY: install-hooks
install-hooks: ## Install pre-commit hooks
	pre-commit install

.PHONY: verify
verify: ## Verify source passes standard checks
	${GO} vet \
		./cmd/... \
		./pkg/...
	! gofmt -l cmd pkg test |\
		sed 's/^/Unformatted file: /' |\
		grep .
	@ ${GO} version | grep -q "$(GO_VERSION)" || \
		( \
			printf '\033[41m\033[97m\n'; \
			echo "* Your go version is not the expected $(GO_VERSION) *" | sed 's/./*/g'; \
			echo "* Your go version is not the expected $(GO_VERSION) *"; \
			echo "* Your go version is not the expected $(GO_VERSION) *" | sed 's/./*/g'; \
			printf '\033[0m'; \
		)

.PHONY: gofmt
gofmt: ## Format Go code
	! gofmt -l cmd pkg test |\
		sed 's/^/Unformatted file: /' |\
		grep .

.PHONY: go-vet
go-vet: ## Run go vet
	${GO} vet ./cmd/... ./pkg/...

.PHONY: lint
lint: generate-all $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run ./cmd/... ./pkg/... ./test/...

.PHONY: verify-migrations
verify-migrations: ## Verify migration files follow project conventions
	@hack/verify-migrations.sh

##@ Code Generation

.PHONY: generate
generate: $(OAPI_CODEGEN) ## Generate OpenAPI types using oapi-codegen
	$(GO) mod download
	rm -rf pkg/api/openapi
	mkdir -p pkg/api/openapi openapi
	@rm -f openapi/openapi.yaml
	@cp "$$($(GO) list -m -f '{{.Dir}}' github.com/openshift-hyperfleet/hyperfleet-api-spec)/schemas/core/openapi.yaml" openapi/openapi.yaml
	$(OAPI_CODEGEN) --config openapi/oapi-codegen.yaml openapi/openapi.yaml

.PHONY: generate-mocks
generate-mocks: $(MOCKGEN) ## Generate mock implementations for services
	${GO} generate ./pkg/services/...

.PHONY: generate-all
generate-all: generate generate-mocks ## Generate all code (openapi + mocks)

.PHONY: generate-vendor
generate-vendor: generate

##@ Development

.PHONY: build
build: generate-all ## Build the hyperfleet-api binary
	@mkdir -p bin
	@echo "Building version: ${APP_VERSION}"
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/hyperfleet-api ./cmd/hyperfleet-api

.PHONY: install
install: generate-all ## Build and install binary to GOPATH/bin
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} install $(GOFLAGS) -ldflags="$(LDFLAGS)" ./cmd/hyperfleet-api

# Common CLI flags for local database access
DB_FLAGS = --db-host localhost --db-port $(db_port) --db-name $(db_name) \
           --db-username $(db_user) --db-password $(db_password)

.PHONY: run
run: db/migrate ## Run the application
	./bin/hyperfleet-api serve $(DB_FLAGS)

.PHONY: run-no-auth
run-no-auth: db/migrate ## Run the application without auth
	./bin/hyperfleet-api serve $(DB_FLAGS) --server-jwt-enabled=false

.PHONY: run/docs
run/docs: check-container-tool ## Run swagger and host the api spec
	@echo "Please open http://localhost:8081/"
	# Port 8081 instead of 80: ports <1024 are privileged and fail with rootless Podman.
	# Port 8080 is avoided since it's used by the health endpoint server.
	$(CONTAINER_TOOL) run -d -p 8081:8080 -e SWAGGER_JSON=/openapi.yaml -v $(PWD)/openapi/openapi.yaml:/openapi.yaml swaggerapi/swagger-ui

.PHONY: cmds
cmds: ## Build all binaries under cmd/
	@mkdir -p bin
	for cmd in $$(ls cmd); do \
		CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} build \
			$(GOFLAGS) \
			-ldflags="$(LDFLAGS)" \
			-o "bin/$${cmd}" \
			"./cmd/$${cmd}" \
			|| exit 1; \
	done

.PHONY: clean
clean: ## Delete temporary generated files
	rm -rf \
		bin \
		pkg/api/openapi \
		data/generated/openapi/*.json \
		openapi/openapi.yaml \

##@ Testing

.PHONY: test
test: install $(GOTESTSUM) ## Run unit tests
	HYPERFLEET_ENV=unit_testing $(GOTESTSUM) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...

.PHONY: ci-test-unit
ci-test-unit: install $(GOTESTSUM) ## Run unit tests with JSON output
	HYPERFLEET_ENV=unit_testing $(GOTESTSUM) --jsonfile-timing-events=$(unit_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...

.PHONY: test-integration
test-integration: install $(GOTESTSUM) ## Run integration tests
	TESTCONTAINERS_RYUK_DISABLED=true HYPERFLEET_ENV=integration_testing $(GOTESTSUM) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration

.PHONY: ci-test-integration
ci-test-integration: install $(GOTESTSUM) ## Run integration tests with JSON output
	TESTCONTAINERS_RYUK_DISABLED=true HYPERFLEET_ENV=integration_testing $(GOTESTSUM) --jsonfile-timing-events=$(integration_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration

.PHONY: test-all
test-all: lint test test-integration test-helm ## Run all checks (lint, unit, integration, helm)

.PHONY: test-coverage
test-coverage: ## Run unit tests with coverage (excludes generated code)
	@echo "Running unit tests with coverage..."
	@$(MAKE) test TESTFLAGS="-coverprofile=coverage.out -covermode=atomic -count=1"
	@if [ -f coverage.out ]; then \
		echo "Filtering out generated code (mocks, openapi) from coverage..."; \
		grep -v -E '(_mock\.go|/mocks/|/openapi/)' coverage.out > coverage-filtered.out || true; \
		mv coverage-filtered.out coverage.out; \
	fi
	@echo ""
	@if [ -f coverage.out ]; then \
		echo ""; \
		echo "Coverage summary (excluding generated code):"; \
		echo "Total Coverage: $$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}')"; \
		echo ""; \
		echo "To view detailed HTML coverage report, run: make coverage-html"; \
	else \
		echo "No coverage file generated."; \
	fi

.PHONY: test-coverage-integration
test-coverage-integration: ## Run integration tests with coverage (excludes generated code)
	@echo "Running integration tests with coverage..."
	@$(MAKE) test-integration TESTFLAGS="-coverprofile=coverage-integration.out -covermode=atomic -coverpkg=./pkg/...,./cmd/... -count=1"
	@if [ -f coverage-integration.out ]; then \
		echo "Filtering out generated code (mocks, openapi) from coverage..."; \
		grep -v -E '(_mock\.go|/mocks/|/openapi/)' coverage-integration.out > coverage-integration-filtered.out || true; \
		mv coverage-integration-filtered.out coverage-integration.out; \
	fi
	@echo ""
	@if [ -f coverage-integration.out ]; then \
		echo ""; \
		echo "Coverage summary (excluding generated code):"; \
		echo "Total Coverage: $$(go tool cover -func=coverage-integration.out | tail -1 | awk '{print $$3}')"; \
		echo ""; \
		echo "To view detailed HTML coverage report, run: make coverage-integration-html"; \
	fi

.PHONY: coverage-html
coverage-html: ## Open HTML coverage report for unit tests
	@if [ ! -f coverage.out ]; then \
		echo "No coverage.out file found. Run 'make test-coverage' first."; \
		exit 1; \
	fi
	@echo "Opening coverage report in browser..."
	@go tool cover -html=coverage.out

.PHONY: coverage-integration-html
coverage-integration-html: ## Open HTML coverage report for integration tests
	@if [ ! -f coverage-integration.out ]; then \
		echo "No coverage-integration.out file found. Run 'make test-coverage-integration' first."; \
		exit 1; \
	fi
	@echo "Opening coverage report in browser..."
	@go tool cover -html=coverage-integration.out

.PHONY: coverage-clean
coverage-clean: ## Remove all coverage files
	@echo "Cleaning coverage files..."
	@rm -f coverage.out coverage-integration.out coverage-unfiltered.out
	@echo "Coverage files removed."

##@ Agent Verification

.PHONY: verify-all
verify-all: verify lint verify-migrations test ## Run all static checks + unit tests (no database required)
	@echo "All static checks and unit tests passed."
	@echo "Run 'make test-integration' separately for integration tests (requires database)."

##@ Database

.PHONY: db/setup
db/setup: check-container-tool ## Start local PostgreSQL container
	$(CONTAINER_TOOL) run --name psql-hyperfleet -e POSTGRES_DB=$(db_name) -e POSTGRES_USER=$(db_user) -e POSTGRES_PASSWORD=$(db_password) -p $(db_port):5432 -d $(db_image)

.PHONY: db/migrate
db/migrate: build ## Apply database migrations to local PostgreSQL
	HYPERFLEET_SERVER_JWT_ENABLED=false ./bin/hyperfleet-api migrate $(DB_FLAGS)

.PHONY: db/login
db/login: check-container-tool ## Login to local PostgreSQL
	$(CONTAINER_TOOL) exec -it psql-hyperfleet bash -c "psql -h localhost -U $(db_user) $(db_name)"

.PHONY: db/teardown
db/teardown: check-container-tool ## Stop and remove local PostgreSQL container
	$(CONTAINER_TOOL) stop psql-hyperfleet
	$(CONTAINER_TOOL) rm psql-hyperfleet

##@ Helm Charts

.PHONY: helm-docs
helm-docs: $(HELM_DOCS) ## Generate Helm chart README from values.yaml annotations
	$(HELM_DOCS) --chart-search-root=charts --sort-values-order=file

.PHONY: verify-helm-docs
verify-helm-docs: $(HELM_DOCS) ## Verify chart README is up to date
	$(HELM_DOCS) --chart-search-root=charts --sort-values-order=file
	@git diff --exit-code charts/README.md > /dev/null 2>&1 || \
		(echo "ERROR: charts/README.md is out of date. Run 'make helm-docs' and commit the result." && exit 1)

# kubeconform flags for validating rendered Helm templates against Kubernetes
# and CRD schemas. Uses the datreeio/CRDs-catalog for ServiceMonitor and
# PrometheusRule schemas.
KUBECONFORM_FLAGS := \
	-strict \
	-kubernetes-version 1.30.0 \
	-schema-location default \
	-schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

.PHONY: test-helm
test-helm: $(KUBECONFORM) verify-helm-docs ## Test Helm charts (lint, template, validate, kubeconform)
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Testing Helm charts..."
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@if ! command -v helm > /dev/null; then \
		echo "Error: helm not found. Please install Helm:"; \
		echo "  brew install helm  # macOS"; \
		echo "  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash  # Linux"; \
		exit 1; \
	fi
	@echo "Linting Helm chart..."
	helm lint charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]'
	@echo ""
	@echo "Testing template rendering with default values..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Default values template OK"
	@echo ""
	@echo "Testing template with external database..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set database.postgresql.enabled=false \
		--set database.external.enabled=true \
		--set database.external.secretName=my-db-secret | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "External database config template OK"
	@echo ""
	@echo "Testing template with autoscaling..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set autoscaling.enabled=true \
		--set autoscaling.minReplicas=2 \
		--set autoscaling.maxReplicas=5 | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Autoscaling config template OK"
	@echo ""
	@echo "Testing template with PDB enabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set podDisruptionBudget.enabled=true \
		--set podDisruptionBudget.minAvailable=1 | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "PDB config template OK"
	@echo ""
	@echo "Testing template with ServiceMonitor enabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set serviceMonitor.enabled=true \
		--set serviceMonitor.interval=15s | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "ServiceMonitor config template OK"
	@echo ""
	@echo "Testing template with PodMonitoring enabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set monitoring.podMonitoring.enabled=true \
		--set monitoring.podMonitoring.interval=15s | $(KUBECONFORM) $(KUBECONFORM_FLAGS) -ignore-missing-schemas
	@echo "PodMonitoring config template OK"
	@echo ""
	@echo "Testing template with PodMonitoring and TLS enabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set monitoring.podMonitoring.enabled=true \
		--set config.metrics.tls.enabled=true \
		--set monitoring.podMonitoring.tlsConfig.insecureSkipVerify=true | $(KUBECONFORM) $(KUBECONFORM_FLAGS) -ignore-missing-schemas
	@echo "PodMonitoring with TLS config template OK"
	@echo ""
	@echo "Testing template with auth disabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set config.server.jwt.enabled=false | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Auth disabled config template OK"
	@echo ""
	@echo "Testing template with custom image..."
	helm template test-release charts/ \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set image.registry=quay.io \
		--set image.repository=myorg/hyperfleet-api \
		--set image.tag=v1.0.0 | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Custom image config template OK"
	@echo ""
	@echo "Testing template with sidecar injection..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set-json 'sidecars=[{"name":"test-sidecar","image":"busybox:1.36","command":["sleep","infinity"]}]' | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Sidecar injection config template OK"
	@echo ""
	@echo "Testing template with native sidecar injection..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set-json 'nativeSidecars=[{"name":"cloud-sql-proxy","restartPolicy":"Always","image":"gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.14.3","args":["--structured-logs","--port=5432","project:region:instance"]}]'); \
		echo "$$OUTPUT" | grep -q 'name: cloud-sql-proxy' || { echo "FAIL: cloud-sql-proxy not found in rendered output"; exit 1; }; \
		PROXY_LINE=$$(echo "$$OUTPUT" | grep -n 'name: cloud-sql-proxy' | head -1 | cut -d: -f1); \
		MIGRATE_LINE=$$(echo "$$OUTPUT" | grep -n 'name: db-migrate' | head -1 | cut -d: -f1); \
		if [ "$$PROXY_LINE" -ge "$$MIGRATE_LINE" ]; then echo "FAIL: cloud-sql-proxy must appear before db-migrate"; exit 1; fi; \
		echo "$$OUTPUT" | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Native sidecar injection config template OK"
	@echo ""
	@echo "Testing template with native sidecars and no database..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set database.postgresql.enabled=false \
		--set-json 'nativeSidecars=[{"name":"test-proxy","restartPolicy":"Always","image":"busybox:1.36","command":["sleep","infinity"]}]'); \
		echo "$$OUTPUT" | grep -q 'name: test-proxy' || { echo "FAIL: test-proxy not found in rendered output"; exit 1; }; \
		if echo "$$OUTPUT" | grep -q 'name: wait-for-db'; then echo "FAIL: wait-for-db should not appear when no database is configured"; exit 1; fi; \
		if echo "$$OUTPUT" | grep -q 'name: db-migrate'; then echo "FAIL: db-migrate should not appear when no database is configured"; exit 1; fi; \
		echo "$$OUTPUT" | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Native sidecar without database config template OK"
	@echo ""
	@echo "Testing template with full adapter config..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set-json 'adapters.cluster=["validation","dns","pullsecret","hypershift"]' \
		--set-json 'adapters.nodepool=["validation","hypershift"]' | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Full adapter config template OK"
	@echo ""
	@echo "Testing template with validation schema enabled..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set validationSchema.enabled=true \
		--set-string 'validationSchema.content=openapi: 3.0.0'); \
		echo "$$OUTPUT" | grep -q 'app.kubernetes.io/component: validation-schema' || { echo "FAIL: validation-schema ConfigMap not found"; exit 1; }; \
		echo "$$OUTPUT" | grep -q '/etc/hyperfleet/validation-schema' || { echo "FAIL: validation schema volume mount not found"; exit 1; }; \
		echo "$$OUTPUT" | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Validation schema enabled config template OK"
	@echo ""
	@echo "Testing template with validation schema disabled (default)..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]'); \
		if echo "$$OUTPUT" | grep -q 'validation-schema'; then echo "FAIL: validation-schema should not appear when disabled"; exit 1; fi; \
		echo "$$OUTPUT" | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Validation schema disabled config template OK"
	@echo ""
	@echo "Testing template with validation schema existingConfigMap..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set validationSchema.enabled=true \
		--set validationSchema.existingConfigMap=my-validation-schema); \
		echo "$$OUTPUT" | grep -q 'my-validation-schema' || { echo "FAIL: existingConfigMap name not found"; exit 1; }; \
		if echo "$$OUTPUT" | grep -q 'app.kubernetes.io/component: validation-schema'; then echo "FAIL: generated ConfigMap should not appear with existingConfigMap"; exit 1; fi; \
		echo "$$OUTPUT" | $(KUBECONFORM) $(KUBECONFORM_FLAGS)
	@echo "Validation schema existingConfigMap config template OK"
	@echo ""
	@echo "Testing validation schema fails without content or existingConfigMap..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set validationSchema.enabled=true 2>&1); \
		if [ $$? -eq 0 ]; then \
			echo "FAIL: should fail when validationSchema.enabled=true without content or existingConfigMap"; exit 1; \
		fi; \
		echo "$$OUTPUT" | grep -q 'validationSchema.content is required' || { \
			echo "FAIL: expected validationSchema validation error message"; echo "$$OUTPUT"; exit 1; \
		}
	@echo "Validation schema validation (no content) OK"
	@echo ""
	@echo "Testing validation schema fails with whitespace-only content..."
	@OUTPUT=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-api \
		--set image.tag=test \
		--set 'adapters.cluster=["validation"]' \
		--set 'adapters.nodepool=["validation"]' \
		--set validationSchema.enabled=true \
		--set-string 'validationSchema.content=   ' 2>&1); \
		if [ $$? -eq 0 ]; then \
			echo "FAIL: should fail when validationSchema.content is whitespace-only"; exit 1; \
		fi; \
		echo "$$OUTPUT" | grep -q 'validationSchema.content is required' || { \
			echo "FAIL: expected validationSchema validation error message"; echo "$$OUTPUT"; exit 1; \
		}
	@echo "Validation schema validation (whitespace-only content) OK"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "All Helm chart tests passed!"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

##@ Container Images

# Build container image (multi-stage build, no local binary needed)
.PHONY: image
image: check-container-tool ## Build container image with configurable registry/tag
	@echo "Building container image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)..."
	$(CONTAINER_TOOL) build \
		--platform $(PLATFORM) \
		--build-arg GIT_SHA=$(GIT_SHA) \
		--build-arg GIT_DIRTY=$(GIT_DIRTY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		-t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "Image built: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-push
image-push: image ## Build and push container image
	@echo "Pushing image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)..."
	$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	@echo "Image pushed: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-dev
image-dev: check-container-tool ## Build and push to personal Quay registry (requires QUAY_USER)
ifeq ($(strip $(QUAY_USER)),)
	@echo "Error: QUAY_USER is not set"
	@echo ""
	@echo "Usage: QUAY_USER=myuser make image-dev"
	@echo ""
	@echo "This will build and push to: quay.io/$$QUAY_USER/$(IMAGE_NAME):$(DEV_TAG)"
	@exit 1
endif
	@echo "Building dev image quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)..."
	$(CONTAINER_TOOL) build \
		--platform $(PLATFORM) \
		--build-arg BASE_IMAGE=$(DEV_BASE_IMAGE) \
		--build-arg GIT_SHA=$(GIT_SHA) \
		--build-arg GIT_DIRTY=$(GIT_DIRTY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg APP_VERSION=0.0.0-dev \
		-t quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG) .
	@echo "Pushing dev image..."
	$(CONTAINER_TOOL) push quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)
	@echo ""
	@echo "Dev image pushed: quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)"
