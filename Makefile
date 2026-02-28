.DEFAULT_GOAL := help

# Include bingo-managed tool variables
include .bingo/Variables.mk

# CGO_ENABLED=0 is not FIPS compliant. large commercial vendors and FedRAMP require FIPS compliant crypto
# Use ?= to allow Dockerfile to override (CGO_ENABLED=0 for Alpine-based dev images)
CGO_ENABLED ?= 1

GO ?= go

# Auto-detect container tool (podman preferred when available)
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# Version information
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY ?= $(shell [ -z "$$(git status --porcelain 2>/dev/null)" ] || echo "-modified")
VERSION ?= $(GIT_SHA)$(GIT_DIRTY)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build flags
GOFLAGS ?= -trimpath
LDFLAGS := -s -w \
           -X github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.Version=$(VERSION) \
           -X github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.Commit=$(GIT_SHA) \
           -X 'github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.BuildTime=$(BUILD_DATE)'

# =============================================================================
# Image Configuration
# =============================================================================
IMAGE_REGISTRY ?= quay.io/openshift-hyperfleet
IMAGE_NAME ?= hyperfleet-api
IMAGE_TAG ?= $(VERSION)

# Dev image configuration - set QUAY_USER to push to personal registry
# Usage: QUAY_USER=myuser make image-dev
QUAY_USER ?=
DEV_TAG ?= dev-$(GIT_SHA)

# Encourage consistent tool versions
OPENAPI_GENERATOR_VERSION := 5.4.0
GO_VERSION := go1.25.

# Database connection details
db_name := hyperfleet
db_port := 5432
db_user := hyperfleet
db_password := foobar-bizz-buzz
db_password_file := ${PWD}/secrets/db.password
db_sslmode := disable
db_image ?= docker.io/library/postgres:14.2

# Location of the JSON web key set used to verify tokens
jwks_url := https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs

# Test output files
unit_test_json_output ?= ${PWD}/unit-test-results.json
integration_test_json_output ?= ${PWD}/integration-test-results.json

### Environment-sourced variables with defaults
ifndef OCM_ENV
	OCM_ENV := development
endif

ifndef TEST_SUMMARY_FORMAT
	TEST_SUMMARY_FORMAT = short-verbose
endif

ifndef OCM_BASE_URL
	OCM_BASE_URL := "https://api.integration.openshift.com"
endif

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_\/-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Code Quality

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

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run ./cmd/... ./pkg/... ./test/...

##@ Code Generation

.PHONY: generate
generate: $(OAPI_CODEGEN) ## Generate OpenAPI types using oapi-codegen
	rm -rf pkg/api/openapi
	mkdir -p pkg/api/openapi
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
	@echo "Building version: ${VERSION}"
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/hyperfleet-api ./cmd/hyperfleet-api

.PHONY: install
install: generate-all ## Build and install binary to GOPATH/bin
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} install $(GOFLAGS) -ldflags="$(LDFLAGS)" ./cmd/hyperfleet-api

.PHONY: run
run: build ## Run the application
	./bin/hyperfleet-api migrate
	./bin/hyperfleet-api serve

.PHONY: run-no-auth
run-no-auth: build ## Run the application without auth
	./bin/hyperfleet-api migrate
	./bin/hyperfleet-api serve --enable-authz=false --enable-jwt=false

.PHONY: run/docs
run/docs: ## Run swagger and host the api spec
	@echo "Please open http://localhost:8081/"
	# Port 8081 instead of 80: ports <1024 are privileged and fail with rootless Podman.
	# Port 8080 is avoided since it's used by the health endpoint server.
	$(CONTAINER_TOOL) run -d -p 8081:8080 -e SWAGGER_JSON=/hyperfleet.yaml -v $(PWD)/openapi/hyperfleet.yaml:/hyperfleet.yaml swaggerapi/swagger-ui

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
		secrets \

.PHONY: secrets
secrets: ## Initialize secrets directory with default values
	@mkdir -p secrets
	@printf "localhost" > secrets/db.host
	@printf "$(db_name)" > secrets/db.name
	@printf "$(db_password)" > secrets/db.password
	@printf "$(db_port)" > secrets/db.port
	@printf "$(db_user)" > secrets/db.user
	@printf "ocm-hyperfleet-testing" > secrets/ocm-service.clientId
	@printf "your-client-secret-here" > secrets/ocm-service.clientSecret
	@printf "your-token-here" > secrets/ocm-service.token
	@echo "Secrets directory initialized with default values"

##@ Testing

.PHONY: test
test: install secrets $(GOTESTSUM) ## Run unit tests
	OCM_ENV=unit_testing $(GOTESTSUM) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...

.PHONY: ci-test-unit
ci-test-unit: install secrets $(GOTESTSUM) ## Run unit tests with JSON output
	OCM_ENV=unit_testing $(GOTESTSUM) --jsonfile-timing-events=$(unit_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...

.PHONY: test-integration
test-integration: install secrets $(GOTESTSUM) ## Run integration tests
	TESTCONTAINERS_RYUK_DISABLED=true OCM_ENV=integration_testing $(GOTESTSUM) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration

.PHONY: ci-test-integration
ci-test-integration: install secrets $(GOTESTSUM) ## Run integration tests with JSON output
	TESTCONTAINERS_RYUK_DISABLED=true OCM_ENV=integration_testing $(GOTESTSUM) --jsonfile-timing-events=$(integration_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration

##@ Database

.PHONY: db/setup
db/setup: secrets ## Start local PostgreSQL container
	@echo $(db_password) > $(db_password_file)
	$(CONTAINER_TOOL) run --name psql-hyperfleet -e POSTGRES_DB=$(db_name) -e POSTGRES_USER=$(db_user) -e POSTGRES_PASSWORD=$(db_password) -p $(db_port):5432 -d $(db_image)

.PHONY: db/login
db/login: ## Login to local PostgreSQL
	$(CONTAINER_TOOL) exec -it psql-hyperfleet bash -c "psql -h localhost -U $(db_user) $(db_name)"

.PHONY: db/teardown
db/teardown: ## Stop and remove local PostgreSQL container
	$(CONTAINER_TOOL) stop psql-hyperfleet
	$(CONTAINER_TOOL) rm psql-hyperfleet

##@ Container Images

.PHONY: image
image: ## Build container image with configurable registry/tag
	@echo "Building container image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)..."
	$(CONTAINER_TOOL) build \
		--platform linux/amd64 \
		--build-arg GIT_SHA=$(GIT_SHA) \
		--build-arg GIT_DIRTY=$(GIT_DIRTY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "Image built: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-push
image-push: image ## Build and push container image
	@echo "Pushing image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)..."
	$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	@echo "Image pushed: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-dev
image-dev: ## Build and push to personal Quay registry (requires QUAY_USER)
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
		--platform linux/amd64 \
		--build-arg BASE_IMAGE=alpine:3.21 \
		--build-arg GIT_SHA=$(GIT_SHA) \
		--build-arg GIT_DIRTY=$(GIT_DIRTY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VERSION=$(VERSION) \
		-t quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG) .
	@echo "Pushing dev image..."
	$(CONTAINER_TOOL) push quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)
	@echo ""
	@echo "Dev image pushed: quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)"
