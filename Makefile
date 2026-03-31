# kmortem — Kubernetes Node Forensics Operator
# "Nodes die. Evidence shouldn't."

BINARY_NAME     ?= kmortem
IMG             ?= kmortem:latest
GOFLAGS         ?=
GOTEST_FLAGS    ?= -v -race -count=1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set).
ifeq (,$(shell go env GOBIN))
GOBIN           = $(shell go env GOPATH)/bin
else
GOBIN           = $(shell go env GOBIN)
endif

.PHONY: all build test fmt vet lint clean docker-build docker-push deploy undeploy manifests generate sync-helm-crds sync-helm-rbac help

all: build

## build: Build the operator binary.
build: fmt vet
	go build $(GOFLAGS) -o bin/$(BINARY_NAME) ./cmd/...

## test: Run unit tests.
test: fmt vet
	go test $(GOTEST_FLAGS) ./...

## fmt: Run go fmt against code.
fmt:
	go fmt ./...

## vet: Run go vet against code.
vet:
	go vet ./...

## lint: Run golangci-lint (requires golangci-lint in PATH).
lint:
	golangci-lint run ./...

## clean: Remove build artifacts.
clean:
	rm -rf bin/

## docker-build: Build the operator container image.
docker-build:
	docker build -t $(IMG) .

## docker-push: Push the operator container image.
docker-push:
	docker push $(IMG)

## manifests: Generate CRD and RBAC manifests and sync into Helm chart.
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=kmortem-manager-role crd paths="./..." \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac
	$(MAKE) sync-helm-crds sync-helm-rbac

## sync-helm-crds: Sync CRD from config/crd/bases/ into Helm chart template.
sync-helm-crds:
	python3 hack/sync-helm-crd.py

## sync-helm-rbac: Sync RBAC rules from config/rbac/ into Helm chart template.
sync-helm-rbac:
	python3 hack/sync-helm-rbac.py

## generate: Generate DeepCopy methods (requires controller-gen).
generate:
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

## deploy: Deploy the operator to the cluster via Helm.
deploy:
	helm upgrade --install kmortem charts/kmortem/ \
		--namespace kmortem

## undeploy: Remove the operator from the cluster.
undeploy:
	helm uninstall kmortem --namespace kmortem

## help: Show this help message.
help:
	@echo "Usage: make <target>"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

# Locate controller-gen; download if not present.
CONTROLLER_GEN = $(GOBIN)/controller-gen
.PHONY: controller-gen
controller-gen:
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@latest)

define go-install-tool
@[ -f $(1) ] || { \
	echo "Downloading $(2)"; \
	GOBIN=$(GOBIN) go install $(2); \
}
endef
