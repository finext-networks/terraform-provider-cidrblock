VERSION       ?= 0.1.0
GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(shell go env GOARCH)
BINARY_NAME   ?= terraform-provider-cidrblock
#GOFLAGS       ?= $(GOFLAGS)

# Default target
default: build

## build: Build the provider
.PHONY: build
build:
	go build $(GOFLAGS) -o ${BINARY_NAME}

## install: Build and install the provider
.PHONY: install
install: build
	install ${BINARY_NAME} "${HOME}/.terraform.d/plugins/registry.terraform.io/finext-networks/cidrblock/${VERSION}/${GOOS}_${GOARCH}/${BINARY_NAME}"

## fmt: Format Go code
.PHONY: fmt
fmt:
	go fmt ./...

## lint: Run linter
.PHONY: lint
lint:
	golangci-lint run

## test: Run unit tests
.PHONY: test
test:
	go test $(GOFLAGS) ./... -v -cover

## testacc: Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test $(GOFLAGS) ./... -v -cover

## test-e2e: Build provider locally and run automated HCL integration test suites
.PHONY: test-e2e
test-e2e: build
	@echo "==> Constructing temporary developer runtime overrides..."
	@echo 'provider_installation { dev_overrides { "finext-networks/cidrblock" = "$(shell pwd)" } direct {} }' > .terraformrc
	@echo "==> Running automated E2E integration test suites..."
	@export TF_CLI_CONFIG_FILE=$(shell pwd)/.terraformrc; \
	cd tests && terraform test

## generate: Generate documentation
.PHONY: generate
generate:
	go generate ./...

## tools: Install development tools
.PHONY: tools
tools:
	go install [github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest](https://github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest)
	go install [github.com/golangci/golangci-lint/cmd/golangci-lint@latest](https://github.com/golangci/golangci-lint/cmd/golangci-lint@latest)

## clean: Clean build artifacts and temporary testing environments
.PHONY: clean
clean:
	rm -f ${BINARY_NAME}
	rm -f .terraformrc
	rm -rf tests/.terraform/ tests/.terraform.lock.hcl

