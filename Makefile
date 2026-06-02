VERSION       ?= 0.1.0
GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(shell go env GOARCH)
BINARY_NAME   ?= terraform-provider-cidrblock
GOFLAGS       ?= $(GOFLAGS)

# Default target
default: build

## build: Build the provider
.PHONY: build
build:
	go build $(GOFLAGS) -o ${BINARY_NAME}

## install: Build and install the provider
.PHONY: install
install: build
	install ${BINARY_NAME} "${HOME}/.terraform.d/plugins/registry.terraform.io/finext/cidrblock/${VERSION}/${GOOS}_${GOARCH}/${BINARY_NAME}"

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

## generate: Generate documentation
.PHONY: generate
generate:
	go generate ./...

## tools: Install development tools
.PHONY: tools
tools:
	go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

## clean: Clean build artifacts
.PHONY: clean
clean:
	rm -f ${BINARY_NAME}
