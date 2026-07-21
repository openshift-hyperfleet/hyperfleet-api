.DEFAULT_GOAL := help

# CGO_ENABLED=0 is not FIPS compliant. large commercial vendors and FedRAMP require FIPS compliant crypto
# Use ?= to allow Dockerfile to override (CGO_ENABLED=0 for Alpine-based dev images)
CGO_ENABLED ?= 1

GO ?= go

# Invoke a pinned tool: $(call gotool,name)
# All tools share tools/go.mod with Go 1.24+ tool directives.
TOOL_MOD := tools/go.mod
gotool = "$(GO)" tool -modfile="$(TOOL_MOD)" $(1)

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
GO_VERSION := go1.26

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
lint: generate-all ## Run golangci-lint
	$(call gotool,golangci-lint) run ./cmd/... ./pkg/... ./test/...

.PHONY: verify-migrations
verify-migrations: ## Verify migration files follow project conventions
	@hack/verify-migrations.sh

.PHONY: tools
tools: ## Ensure tool dependencies are up to date
	cd tools && "$(GO)" mod tidy

.PHONY: verify-tools
verify-tools: tools ## Fail in CI if tool module drifted
	@git diff --exit-code HEAD -- tools/go.mod tools/go.sum || (echo "tool modules out of date; run 'make tools'" && exit 1)

##@ Code Generation

.PHONY: generate
generate: ## Generate OpenAPI types using oapi-codegen
	$(GO) mod download
	rm -rf pkg/api/openapi
	mkdir -p pkg/api/openapi openapi
	@rm -f openapi/openapi.yaml
	@cp "$$($(GO) list -m -f '{{.Dir}}' github.com/openshift-hyperfleet/hyperfleet-api-spec)/schemas/core/openapi.yaml" openapi/openapi.yaml
	$(call gotool,oapi-codegen) --config openapi/oapi-codegen.yaml openapi/openapi.yaml
	@printf 'package openapi\n\nimport _ "github.com/oapi-codegen/runtime"\n' > pkg/api/openapi/stub.go

.PHONY: generate-mocks
generate-mocks: ## Generate mock implementations for services
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

DEV_TOKEN_FILE := /tmp/hf-dev-token.txt

.PHONY: run
run: db/migrate ## Run the application with JWT auth
	@install -m 600 /dev/null $(DEV_TOKEN_FILE) && configs/gen-dev-token.sh --new-key > $(DEV_TOKEN_FILE)
	./bin/hyperfleet-api serve $(DB_FLAGS) --config configs/dev.yaml

.PHONY: dev-token
dev-token: ## Generate a fresh JWT using the existing dev key (no server restart needed)
	@install -m 600 /dev/null $(DEV_TOKEN_FILE) && configs/gen-dev-token.sh > $(DEV_TOKEN_FILE)
	@echo 'Usage: curl -H "Authorization: Bearer $$(cat $(DEV_TOKEN_FILE))" http://localhost:8000/api/hyperfleet/v1/clusters'

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
test: install ## Run unit tests
	HYPERFLEET_ENV=unit_testing $(call gotool,gotestsum) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...

.PHONY: ci-test-unit
ci-test-unit: install ## Run unit tests with JSON output
	HYPERFLEET_ENV=unit_testing $(call gotool,gotestsum) --jsonfile-timing-events=$(unit_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...

.PHONY: test-integration
test-integration: install ## Run integration tests
	TESTCONTAINERS_RYUK_DISABLED=true HYPERFLEET_ENV=integration_testing $(call gotool,gotestsum) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration

.PHONY: ci-test-integration
ci-test-integration: install ## Run integration tests with JSON output
	TESTCONTAINERS_RYUK_DISABLED=true HYPERFLEET_ENV=integration_testing $(call gotool,gotestsum) --jsonfile-timing-events=$(integration_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
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
verify-all: verify verify-tools lint verify-migrations test ## Run all static checks + unit tests (no database required)
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
helm-docs: ## Generate Helm chart README from values.yaml annotations
	$(call gotool,helm-docs) --chart-search-root=charts --sort-values-order=file

.PHONY: verify-helm-docs
verify-helm-docs: ## Verify chart README is up to date
	$(call gotool,helm-docs) --chart-search-root=charts --sort-values-order=file
	@git diff --exit-code charts/README.md > /dev/null 2>&1 || \
		(echo "ERROR: charts/README.md is out of date. Run 'make helm-docs' and commit the result." && exit 1)

.PHONY: test-helm
test-helm: verify-helm-docs ## Test Helm charts (lint, template, validate, kubeconform)
	@KUBECONFORM="$(call gotool,kubeconform)" YQ="$(call gotool,yq)" ./scripts/test-helm.sh

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
