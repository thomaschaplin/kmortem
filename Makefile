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

.PHONY: all build test fmt vet lint clean docker-build docker-push deploy undeploy manifests generate help

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

## manifests: Generate CRD and RBAC manifests (requires controller-gen).
manifests:
	$(CONTROLLER_GEN) rbac:roleName=kmortem-manager-role crd paths="./..." \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac

## generate: Generate DeepCopy methods (requires controller-gen).
generate:
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

## deploy: Deploy the operator to the cluster.
deploy: manifests
	kubectl apply -f config/manager/namespace.yaml
	kubectl apply -f config/crd/bases/
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/manager.yaml

## undeploy: Remove the operator from the cluster.
undeploy:
	kubectl delete --ignore-not-found=true -f config/manager/manager.yaml
	kubectl delete --ignore-not-found=true -f config/rbac/
	kubectl delete --ignore-not-found=true -f config/crd/bases/

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
