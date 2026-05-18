include .bingo/Variables.mk

.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt

# Binary output directory and name
BIN_DIR := bin
BINARY_NAME := $(BIN_DIR)/hyperfleet-adapter

# Version information
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY ?= $(shell [ -z "$$(git status --porcelain 2>/dev/null)" ] || echo "-modified")
APP_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")

# Go build flags
GOFLAGS ?= -trimpath
VERSION_PKG := github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/version
LDFLAGS := -s -w \
           -X $(VERSION_PKG).Version=$(APP_VERSION) \
           -X $(VERSION_PKG).Commit=$(GIT_SHA) \
           -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)

# Container tool (docker or podman)
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# Container build configuration
PLATFORM ?= linux/amd64
DEV_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi-minimal:latest

# =============================================================================
# Image Configuration
# =============================================================================
IMAGE_REGISTRY ?= quay.io/openshift-hyperfleet
IMAGE_NAME ?= hyperfleet-adapter
IMAGE_TAG ?= $(APP_VERSION)

# Dev image configuration - set QUAY_USER to push to personal registry
# Usage: QUAY_USER=myuser make image-dev
QUAY_USER ?=
DEV_TAG ?= dev-$(GIT_SHA)

# Test parameters
TEST_TIMEOUT := 30m

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: build
build: ## Build the adapter binary
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/adapter

.PHONY: install
install: build ## Build and install binary to GOPATH/bin
	$(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/adapter

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

##@ Testing

.PHONY: test
test: ## Run unit tests with race detection
	$(GO) test -v -race -coverprofile=coverage.out -timeout $(TEST_TIMEOUT) \
		$$($(GO) list ./... | grep -v /test/)

.PHONY: test-coverage
test-coverage: test ## Run tests and show coverage
	$(GO) tool cover -html=coverage.out

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker/Podman)
	@TEST_TIMEOUT=$(TEST_TIMEOUT) bash scripts/run-integration-tests.sh

.PHONY: image-integration-test
image-integration-test: ## Build integration test image with envtest
	@bash scripts/build-integration-image.sh

.PHONY: test-all
test-all: lint test test-integration test-helm ## Run all checks (lint, unit, integration, helm)

.PHONY: test-helm
test-helm: ## Test Helm charts (lint, template, validate)
	@if ! command -v helm > /dev/null; then \
		echo "ERROR: helm not found. Please install Helm:"; \
		echo "  brew install helm  # macOS"; \
		echo "  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash  # Linux"; \
		exit 1; \
	fi
	@echo "Linting Helm chart..."
	helm lint charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test
	@echo ""
	@echo "Testing template rendering with minimal required values..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic > /dev/null
	@echo "Minimal required values template OK"
	@echo ""
	@echo "Testing template with broker enabled..."
	@output=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.create=true \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set broker.type=googlepubsub) \
		&& echo "$$output" | grep -q 'type: googlepubsub' \
		|| { echo "ERROR: expected googlepubsub broker type in rendered output"; exit 1; }
	@echo "Broker config template OK"
	@echo ""
	@echo "Testing template with HyperFleet API config..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set adapterConfig.hyperfleetApi.baseUrl=http://localhost:8000 \
		--set adapterConfig.hyperfleetApi.version=v1 > /dev/null
	@echo "HyperFleet API config template OK"
	@echo ""
	@echo "Testing template with PDB enabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set podDisruptionBudget.enabled=true \
		--set podDisruptionBudget.minAvailable=1 \
		--set podDisruptionBudget.maxUnavailable=null > /dev/null
	@echo "PDB config template OK"
	@echo ""
	@echo "Testing template with autoscaling..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set autoscaling.enabled=true \
		--set autoscaling.minReplicas=2 \
		--set autoscaling.maxReplicas=5 > /dev/null
	@echo "Autoscaling config template OK"
	@echo ""
	@echo "Testing template with probes enabled..."
	helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set livenessProbe.enabled=true \
		--set readinessProbe.enabled=true \
		--set startupProbe.enabled=true > /dev/null
	@echo "Probes config template OK"
	@echo ""
	@echo "Testing template with ServiceMonitor enabled..."
	@helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set serviceMonitor.enabled=true \
		--api-versions monitoring.coreos.com/v1/ServiceMonitor | grep -q 'kind: ServiceMonitor' \
		|| { echo "ERROR: ServiceMonitor not rendered"; exit 1; }
	@echo "ServiceMonitor enabled template OK"
	@echo ""
	@echo "Testing template with ServiceMonitor enabled but CRD unavailable..."
	@output=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set serviceMonitor.enabled=true) \
		&& ! echo "$$output" | grep -q 'kind: ServiceMonitor' \
		|| { echo "ERROR: ServiceMonitor rendered without CRD"; exit 1; }
	@echo "ServiceMonitor CRD-missing template OK"
	@echo ""
	@echo "Testing template with ServiceMonitor disabled..."
	@output=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.type=googlepubsub \
		--set broker.googlepubsub.subscriptionId=test-sub \
		--set broker.googlepubsub.topic=test-topic \
		--set serviceMonitor.enabled=false \
		--api-versions monitoring.coreos.com/v1/ServiceMonitor) \
		&& ! echo "$$output" | grep -q 'kind: ServiceMonitor' \
		|| { echo "ERROR: ServiceMonitor rendered while disabled"; exit 1; }
	@echo "ServiceMonitor disabled template OK"
	@echo ""
	@echo "Testing template with RabbitMQ broker..."
	@output=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.create=true \
		--set broker.type=rabbitmq \
		--set broker.rabbitmq.url=amqp://guest:guest@rabbitmq:5672/ \
		--set broker.rabbitmq.queue=test-queue \
		--set broker.rabbitmq.exchange=test-exchange \
		--set broker.rabbitmq.routingKey=test-key) \
		&& echo "$$output" | grep -q 'type: rabbitmq' \
		|| { echo "ERROR: expected rabbitmq broker type in rendered output"; exit 1; }
	@echo "RabbitMQ broker template OK"
	@echo ""
	@echo "Testing that rabbitmq broker.type with both sections set selects rabbitmq..."
	@output=$$(helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.create=true \
		--set broker.type=rabbitmq \
		--set broker.rabbitmq.url=amqp://guest:guest@rabbitmq:5672/ \
		--set broker.rabbitmq.queue=test-queue \
		--set broker.rabbitmq.exchange=test-exchange \
		--set broker.rabbitmq.routingKey=test-key \
		--set broker.googlepubsub.projectId=my-project \
		--set broker.googlepubsub.topic=my-topic \
		--set broker.googlepubsub.subscriptionId=my-sub) \
		&& echo "$$output" | grep -q 'type: rabbitmq' \
		|| { echo "ERROR: expected rabbitmq broker type in output"; exit 1; }
	@echo "RabbitMQ broker type selection OK"
	@echo ""
	@echo "Testing that rabbitmq without queue fails validation..."
	@if helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.create=true \
		--set broker.type=rabbitmq \
		--set broker.rabbitmq.url=amqp://guest:guest@rabbitmq:5672/ > /dev/null 2>&1; then \
		echo "ERROR: expected helm template to fail when broker.rabbitmq.queue is missing"; exit 1; \
	fi
	@echo "RabbitMQ missing queue validation OK"
	@echo ""
	@echo "Testing that rabbitmq without exchange fails validation..."
	@if helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.create=true \
		--set broker.type=rabbitmq \
		--set broker.rabbitmq.url=amqp://guest:guest@rabbitmq:5672/ \
		--set broker.rabbitmq.queue=test-queue > /dev/null 2>&1; then \
		echo "ERROR: expected helm template to fail when broker.rabbitmq.exchange is missing"; exit 1; \
	fi
	@echo "RabbitMQ missing exchange validation OK"
	@echo ""
	@echo "Testing that rabbitmq without routingKey fails validation..."
	@if helm template test-release charts/ \
		--set image.registry=quay.io \
		--set image.repository=openshift-hyperfleet/hyperfleet-adapter \
		--set image.tag=test \
		--set adapterConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set adapterTaskConfig.yaml="apiVersion: hyperfleet.redhat.com/v1alpha1" \
		--set broker.create=true \
		--set broker.type=rabbitmq \
		--set broker.rabbitmq.url=amqp://guest:guest@rabbitmq:5672/ \
		--set broker.rabbitmq.queue=test-queue \
		--set broker.rabbitmq.exchange=test-exchange > /dev/null 2>&1; then \
		echo "ERROR: expected helm template to fail when broker.rabbitmq.routingKey is missing"; exit 1; \
	fi
	@echo "RabbitMQ missing routingKey validation OK"

##@ Code Quality

.PHONY: fmt
fmt: $(GOIMPORTS) ## Format code with goimports
	$(GOIMPORTS) -w .

.PHONY: fmt-check
fmt-check: ## Check if code is formatted
	@diff=$$($(GOFMT) -s -d .); \
	if [ -n "$$diff" ]; then \
		echo "Code is not formatted. Run 'make fmt' to fix:"; \
		echo "$$diff"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: verify
verify: fmt-check vet ## Run all verification checks

##@ Dependencies

.PHONY: tidy
tidy: ## Tidy go.mod
	$(GO) mod tidy

.PHONY: download
download: ## Download dependencies
	$(GO) mod download

##@ Container Images

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

.PHONY: image
image: check-container-tool ## Build container image
	@echo "Building image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) ..."
	$(CONTAINER_TOOL) build \
		--platform $(PLATFORM) \
		--build-arg GIT_SHA=$(GIT_SHA) \
		--build-arg GIT_DIRTY=$(GIT_DIRTY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		-t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "Image built: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-push
image-push: check-container-tool image ## Build and push container image
	@echo "Pushing image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) ..."
	$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	@echo "Image pushed: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-dev
image-dev: check-container-tool ## Build and push to personal Quay registry (requires QUAY_USER)
ifndef QUAY_USER
	@echo "Error: QUAY_USER is not set"
	@echo ""
	@echo "Usage: QUAY_USER=myuser make image-dev"
	@echo ""
	@echo "This will build and push to: quay.io/$$QUAY_USER/$(IMAGE_NAME):$(DEV_TAG)"
	@exit 1
endif
	@echo "Building dev image quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG) ..."
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
