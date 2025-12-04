.DEFAULT_GOAL := help

# Include bingo-managed tool variables
include .bingo/Variables.mk

# CGO_ENABLED=0 is not FIPS compliant. large commercial vendors and FedRAMP require FIPS compliant crypto
CGO_ENABLED := 1

# Enable users to override the golang used to accomodate custom installations
GO ?= go

# Version information for build metadata
version:=$(shell date +%s)

# a tool for managing containers and images, etc. You can set it as docker
container_tool ?= podman

# Database connection details
db_name:=hyperfleet
db_host=hyperfleet-db.$(namespace)
db_port=5432
db_user:=hyperfleet
db_password:=foobar-bizz-buzz
db_password_file=${PWD}/secrets/db.password
db_sslmode:=disable
db_image?=docker.io/library/postgres:14.2

# Log verbosity level
glog_v:=10

# Location of the JSON web key set used to verify tokens:
jwks_url:=https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs

# Test output files
unit_test_json_output ?= ${PWD}/unit-test-results.json
integration_test_json_output ?= ${PWD}/integration-test-results.json

# Prints a list of useful targets.
help:
	@echo ""
	@echo "HyperFleet API - Cluster Lifecycle Management Service"
	@echo ""
	@echo "make verify               verify source code"
	@echo "make lint                 run golangci-lint"
	@echo "make binary               compile binaries"
	@echo "make install              compile binaries and install in GOPATH bin"
	@echo "make secrets              initialize secrets directory with default values"
	@echo "make run                  run the application"
	@echo "make run/docs             run swagger and host the api spec"
	@echo "make test                 run unit tests"
	@echo "make test-integration     run integration tests"
	@echo "make generate             generate openapi modules"
	@echo "make generate-mocks       generate mock implementations for services"
	@echo "make generate-all         generate all code (openapi + mocks)"
	@echo "make clean                delete temporary generated files"
	@echo "$(fake)"
.PHONY: help

# Encourage consistent tool versions
OPENAPI_GENERATOR_VERSION:=5.4.0
GO_VERSION:=go1.24.

### Constants:
version:=$(shell date +%s)

# Version information for ldflags
git_sha:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
git_dirty:=$(shell git diff --quiet 2>/dev/null || echo "-modified")
build_version:=$(git_sha)$(git_dirty)
build_time:=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
ldflags=-X github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.Version=$(build_version) -X 'github.com/openshift-hyperfleet/hyperfleet-api/pkg/api.BuildTime=$(build_time)'

### Envrionment-sourced variables with defaults
# Can be overriden by setting environment var before running
# Example:
#   OCM_ENV=unit_testing make run
#   export OCM_ENV=testing; make run
# Set the environment to development by default
ifndef OCM_ENV
	OCM_ENV:=development
endif

ifndef TEST_SUMMARY_FORMAT
	TEST_SUMMARY_FORMAT=short-verbose
endif

ifndef OCM_BASE_URL
	OCM_BASE_URL:="https://api.integration.openshift.com"
endif

# Checks if a GOPATH is set, or emits an error message
check-gopath:
ifndef GOPATH
	$(error GOPATH is not set)
endif
.PHONY: check-gopath

# Verifies that source passes standard checks.
verify: check-gopath
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
.PHONY: verify

# Runs our linter to verify that everything is following best practices
# Linter is set to ignore `unused` stuff due to example being incomplete by definition
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run -e unused \
		./cmd/... \
		./pkg/...
.PHONY: lint

# Build binaries
# NOTE it may be necessary to use CGO_ENABLED=0 for backwards compatibility with centos7 if not using centos7
binary: check-gopath generate-all
	echo "Building version: ${build_version}"
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} build -ldflags="$(ldflags)" -o hyperfleet-api ./cmd/hyperfleet-api
.PHONY: binary

# Install
install: check-gopath generate-all
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=boringcrypto ${GO} install -ldflags="$(ldflags)" ./cmd/hyperfleet-api
	@ ${GO} version | grep -q "$(GO_VERSION)" || \
		( \
			printf '\033[41m\033[97m\n'; \
			echo "* Your go version is not the expected $(GO_VERSION) *" | sed 's/./*/g'; \
			echo "* Your go version is not the expected $(GO_VERSION) *"; \
			echo "* Your go version is not the expected $(GO_VERSION) *" | sed 's/./*/g'; \
			printf '\033[0m'; \
		)
.PHONY: install

# Initialize secrets directory with default values
secrets:
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
.PHONY: secrets

# Runs the unit tests.
#
# Args:
#   TESTFLAGS: Flags to pass to `go test`. The `-v` argument is always passed.
#
# Examples:
#   make test TESTFLAGS="-run TestSomething"
test: install secrets $(GOTESTSUM)
	OCM_ENV=unit_testing $(GOTESTSUM) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...
.PHONY: test

# Runs the unit tests with json output
#
# Args:
#   TESTFLAGS: Flags to pass to `go test`. The `-v` argument is always passed.
#
# Examples:
#   make test-unit-json TESTFLAGS="-run TestSomething"
ci-test-unit: install secrets $(GOTESTSUM)
	OCM_ENV=unit_testing $(GOTESTSUM) --jsonfile-timing-events=$(unit_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -v $(TESTFLAGS) \
		./pkg/... \
		./cmd/...
.PHONY: ci-test-unit

# Runs the integration tests.
#
# Args:
#   TESTFLAGS: Flags to pass to `go test`. The `-v` argument is always passed.
#
# Example:
#   make test-integration
#   make test-integration TESTFLAGS="-run TestAccounts"     acts as TestAccounts* and run TestAccountsGet, TestAccountsPost, etc.
#   make test-integration TESTFLAGS="-run TestAccountsGet"  runs TestAccountsGet
#   make test-integration TESTFLAGS="-short"                skips long-run tests
ci-test-integration: install secrets $(GOTESTSUM)
	TESTCONTAINERS_RYUK_DISABLED=true OCM_ENV=integration_testing $(GOTESTSUM) --jsonfile-timing-events=$(integration_test_json_output) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration
.PHONY: ci-test-integration

# Runs the integration tests.
#
# Args:
#   TESTFLAGS: Flags to pass to `go test`. The `-v` argument is always passed.
#
# Example:
#   make test-integration
#   make test-integration TESTFLAGS="-run TestAccounts"     acts as TestAccounts* and run TestAccountsGet, TestAccountsPost, etc.
#   make test-integration TESTFLAGS="-run TestAccountsGet"  runs TestAccountsGet
#   make test-integration TESTFLAGS="-short"                skips long-run tests
test-integration: install secrets $(GOTESTSUM)
	TESTCONTAINERS_RYUK_DISABLED=true OCM_ENV=integration_testing $(GOTESTSUM) --format $(TEST_SUMMARY_FORMAT) -- -p 1 -ldflags -s -v -timeout 1h $(TESTFLAGS) \
			./test/integration
.PHONY: test-integration

# Regenerate openapi client and models
generate:
	rm -rf pkg/api/openapi
	mkdir -p data/generated/openapi
	$(container_tool) build -t hyperfleet-openapi -f Dockerfile.openapi .
	$(eval OPENAPI_IMAGE_ID=`$(container_tool) create -t hyperfleet-openapi -f Dockerfile.openapi .`)
	$(container_tool) cp $(OPENAPI_IMAGE_ID):/local/pkg/api/openapi ./pkg/api/openapi
	$(container_tool) cp $(OPENAPI_IMAGE_ID):/local/data/generated/openapi/openapi.go ./data/generated/openapi/openapi.go
.PHONY: generate

# Generate mock implementations for service interfaces
generate-mocks: $(MOCKGEN)
	${GO} generate ./pkg/services/...
.PHONY: generate-mocks

# Generate all code (openapi + mocks)
generate-all: generate generate-mocks
.PHONY: generate-all

# Regenerate openapi client and models using vendor (avoids downloading dependencies)
generate-vendor:
	rm -rf pkg/api/openapi
	mkdir -p data/generated/openapi
	$(container_tool) build -t hyperfleet-openapi-vendor -f Dockerfile.openapi.vendor .
	$(eval OPENAPI_IMAGE_ID=`$(container_tool) create -t hyperfleet-openapi-vendor -f Dockerfile.openapi.vendor .`)
	$(container_tool) cp $(OPENAPI_IMAGE_ID):/local/pkg/api/openapi ./pkg/api/openapi
	$(container_tool) cp $(OPENAPI_IMAGE_ID):/local/data/generated/openapi/openapi.go ./data/generated/openapi/openapi.go
.PHONY: generate-vendor

run: binary
	./hyperfleet-api migrate
	./hyperfleet-api serve
.PHONY: run

run-no-auth: binary
	./hyperfleet-api migrate
	./hyperfleet-api serve --enable-authz=false --enable-jwt=false

# Run Swagger nd host the api docs
run/docs:
	@echo "Please open http://localhost/"
	docker run -d -p 80:8080 -e SWAGGER_JSON=/hyperfleet.yaml -v $(PWD)/openapi/hyperfleet.yaml:/hyperfleet.yaml swaggerapi/swagger-ui
.PHONY: run/docs

# Delete temporary files
clean:
	rm -rf \
		$(binary) \
		data/generated/openapi/*.json \
		secrets \
.PHONY: clean

.PHONY: cmds
cmds:
	for cmd in $$(ls cmd); do \
		CGO_ENABLED=$(CGO_ENABLED) ${GO} build \
			-ldflags="$(ldflags)" \
			-o "$${cmd}" \
			"./cmd/$${cmd}" \
			|| exit 1; \
	done


.PHONY: db/setup
db/setup: secrets
	@echo $(db_password) > $(db_password_file)
	$(container_tool) run --name psql-hyperfleet -e POSTGRES_DB=$(db_name) -e POSTGRES_USER=$(db_user) -e POSTGRES_PASSWORD=$(db_password) -p $(db_port):5432 -d $(db_image)

.PHONY: db/login
db/login:
	$(container_tool) exec -it psql-hyperfleet bash -c "psql -h localhost -U $(db_user) $(db_name)"

.PHONY: db/teardown
db/teardown:
	$(container_tool) stop psql-hyperfleet
	$(container_tool) rm psql-hyperfleet
