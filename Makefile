.DEFAULT_GOAL := help

IMAGE_NAME=kube-compare

PACKAGE_NAME          := github.com/openshift/kube-compare
GOLANG_CROSS_VERSION  ?= v1.22.7

# Auto-detect host OS and architecture if not explicitly set
HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)

# Default to host OS/Arch for local builds, can be overridden for cross-compilation
GOOS ?= $(HOST_OS)
GOARCH ?= $(HOST_ARCH)

# These tags make sure we can statically link and avoid shared dependencies
GO_BUILD_FLAGS_LINUX :=-tags 'include_gcs include_oss containers_image_openpgp gssapi'
GO_BUILD_FLAGS_DARWIN :=-tags 'include_gcs include_oss containers_image_openpgp'
GO_BUILD_FLAGS_WINDOWS :=-tags 'include_gcs include_oss containers_image_openpgp'
GO_BUILD_FLAGS_LINUX_CROSS :=-tags 'include_gcs include_oss containers_image_openpgp'

# Select appropriate build flags based on target OS
ifeq ($(GOOS),darwin)
GO_BUILD_FLAGS := $(GO_BUILD_FLAGS_DARWIN)
else ifeq ($(GOOS),windows)
GO_BUILD_FLAGS := $(GO_BUILD_FLAGS_WINDOWS)
else
GO_BUILD_FLAGS := $(GO_BUILD_FLAGS_LINUX)
endif

# Inject a version and date via ldflags for the '--version' flag
# Upstream builds get their ldflags set via goreleaser automatically
ifneq ($(strip $(OS_GIT_VERSION)),)
	# Downstream builds should have this set:
	#   OS_GIT_VERSION=4.19.0-202412190006.p0.ga217c8d.assembly.stream.el9
	# So use it verbatim for the version string
	BUILD_VERSION ?= $(OS_GIT_VERSION)
else
	# For manual builds, use 'git describe' based on the latest tag
	BUILD_VERSION ?= $(shell git describe --tag | sed -e 's/^v//')
endif
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_LDFLAGS := -ldflags="-X 'main.version=$(BUILD_VERSION)' -X 'main.date=$(BUILD_DATE)'"

OUTPUT_DIR :=_output
GO_BUILD_BINDIR ?=$(OUTPUT_DIR)/bin
CROSS_BUILD_BINDIR ?=$(OUTPUT_DIR)/bin

# Default locations for `make install`
PREFIX ?=  /usr/local
DESTDIR ?= $(PREFIX)/bin

# Autodetect $ENGINE if not set (podman first, then docker)
ifeq ($(origin ENGINE), undefined)
  ENGINE = podman
  ifeq ($(shell which $(ENGINE) 2>/dev/null), )
    ENGINE = docker
  endif
endif

# Container-engine-specific options:
ifeq ($(ENGINE), docker)
  CONTAINER_MOUNTOPT=
  CONTAINER_SOCKETOPT="-v /var/run/docker.sock:/var/run/docker.sock"
else ifeq ($(ENGINE), podman)
  CONTAINER_MOUNTOPT=:Z
  CONTAINER_SOCKETOPT=
endif

.PHONY: help
help: ## Display this help message
	@echo "Available targets:"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ""
	@echo "Build for current OS (auto-detected: $(HOST_OS)/$(HOST_ARCH)):"
	@echo "  make build"
	@echo ""
	@echo "Cross-compile for Linux:"
	@echo "  GOOS=linux GOARCH=amd64 make build"

# Build based on OS and Arch. Full list available in https://pkg.go.dev/internal/platform#pkg-variables
.PHONY: build
build: ## Build the kubectl-cluster_compare binary for current platform
	mkdir -p $(GO_BUILD_BINDIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -mod=vendor $(GO_BUILD_FLAGS) $(GO_LDFLAGS) -o $(GO_BUILD_BINDIR)/kubectl-cluster_compare ./cmd/kubectl-cluster_compare.go

# Install the plugin and completion script in /usr/local/bin (requires sudo on most systems)
.PHONY: install
install:
	@if [ ! -w $(DESTDIR) ]; then \
		echo "Error: No write permission to $(DESTDIR)"; \
		echo "Try one of the following:"; \
		echo "  - Run 'sudo make install' for system-wide installation"; \
		echo "  - Run 'make install-user' to install to ~/.local/bin"; \
		echo "  - Set DESTDIR to a writable directory: 'make install DESTDIR=~/bin'"; \
		exit 1; \
	fi
	install kubectl_complete-cluster_compare $(DESTDIR)
	install $(GO_BUILD_BINDIR)/kubectl-cluster_compare  $(DESTDIR)

# Install the plugin and completion script in ~/.local/bin (no sudo required)
.PHONY: install-user
install-user:
	@mkdir -p ~/.local/bin
	install kubectl_complete-cluster_compare ~/.local/bin
	install $(GO_BUILD_BINDIR)/kubectl-cluster_compare ~/.local/bin
	@echo ""
	@echo "Installation complete! Binary installed to ~/.local/bin"
	@echo "Make sure ~/.local/bin is in your PATH by adding this to your shell config:"
	@echo '  export PATH="$$HOME/.local/bin:$$PATH"'

.PHONY: test-all
test-all: test test-report-creator test-helm-convert

.PHONY: test
test: ## Run tests for pkg/
	go test --race ./pkg/*

.PHONY: test-report-creator
test-report-creator:
	go test --race ./addon-tools/report-creator/*/

.PHONY: build-report-creator
build-report-creator:
	go build $(GO_LDFLAGS) ./addon-tools/report-creator/report-creator.go

.PHONY: build-helm-convert
build-helm-convert:
	go build $(GO_LDFLAGS) ./addon-tools/helm-convert/helm-convert.go

.PHONY: test-helm-convert
test-helm-convert:
	go test --race ./addon-tools/helm-convert/*/

.PHONY: golangci-lint
golangci-lint: ## Run golangci-lint against code.
	@echo "Running golangci-lint"
	hack/golangci-lint.sh

# markdownlint rules, following: https://github.com/openshift/enhancements/blob/master/Makefile
.PHONY: markdownlint-image
markdownlint-image:  ## Build local container markdownlint-image
	$(ENGINE) image build -f ./hack/markdownlint.Dockerfile --tag $(IMAGE_NAME)-markdownlint:latest ./hack

.PHONY: markdownlint-image-clean
markdownlint-image-clean:  ## Remove locally cached markdownlint-image
	$(ENGINE) image rm $(IMAGE_NAME)-markdownlint:latest

# markdownlint main
.PHONY: markdownlint
markdownlint: markdownlint-image  ## run the markdown linter
	$(ENGINE) run \
		--rm=true \
		--env RUN_LOCAL=true \
		--env VALIDATE_MARKDOWN=true \
		--env PULL_BASE_SHA=$(PULL_BASE_SHA) \
		-v $$(pwd):/workdir$(CONTAINER_MOUNTOPT) \
		$(IMAGE_NAME)-markdownlint:latest

.PHONY: image-build
image-build: ## Build container image for kube-compare
	$(ENGINE) build . -t $(IMAGE_NAME):latest

.PHONY: release-dry-run
release-dry-run:
	@$(ENGINE) run \
		--rm \
		-e CGO_ENABLED=1 \
		$(CONTAINER_SOCKETOPT) \
		-v `pwd`:/go/src/$(PACKAGE_NAME)$(CONTAINER_MOUNTOPT) \
		-w /go/src/$(PACKAGE_NAME) \
		ghcr.io/goreleaser/goreleaser-cross:${GOLANG_CROSS_VERSION} \
		release --clean --skip=validate --skip=publish

.PHONY: release
release:
	@$(ENGINE) run \
		--rm \
		-e GITHUB_TOKEN=$(GITHUB_TOKEN) \
		-e CGO_ENABLED=1 \
		-v `pwd`:/go/src/$(PACKAGE_NAME)$(CONTAINER_MOUNTOPT) \
		-w /go/src/$(PACKAGE_NAME) \
		ghcr.io/goreleaser/goreleaser-cross:${GOLANG_CROSS_VERSION} \
		release --clean

.PHONY: cross-build-darwin-amd64
cross-build-darwin-amd64:
	+@GOOS=darwin GOARCH=amd64 GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_DARWIN)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/darwin_amd64 $(MAKE) --no-print-directory build

.PHONY: cross-build-darwin-arm64
cross-build-darwin-arm64:
	+@GOOS=darwin GOARCH=arm64 GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_DARWIN)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/darwin_arm64 $(MAKE) --no-print-directory build

.PHONY: cross-build-windows-amd64
cross-build-windows-amd64:
	+@GOOS=windows GOARCH=amd64 GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_WINDOWS)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/windows_amd64 $(MAKE) --no-print-directory build

.PHONY: cross-build-linux-amd64
cross-build-linux-amd64:
	+@GOOS=linux GOARCH=amd64 GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_LINUX_CROSS)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/linux_amd64 $(MAKE) --no-print-directory build

.PHONY: cross-build-linux-arm64
cross-build-linux-arm64:
	+@GOOS=linux GOARCH=arm64 GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_LINUX_CROSS)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/linux_arm64 $(MAKE) --no-print-directory build

.PHONY: cross-build-linux-ppc64le
cross-build-linux-ppc64le:
	+@GOOS=linux GOARCH=ppc64le GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_LINUX_CROSS)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/linux_ppc64le $(MAKE) --no-print-directory build

.PHONY: cross-build-linux-s390x
cross-build-linux-s390x:
	+@GOOS=linux GOARCH=s390x GO_BUILD_FLAGS="$(GO_BUILD_FLAGS_LINUX_CROSS)" GO_BUILD_BINDIR=$(CROSS_BUILD_BINDIR)/linux_s390x $(MAKE) --no-print-directory build

.PHONY: cross-build
cross-build: cross-build-darwin-amd64 cross-build-darwin-arm64 cross-build-windows-amd64 cross-build-linux-amd64 cross-build-linux-arm64 cross-build-linux-ppc64le cross-build-linux-s390x ## Build for all supported platforms

.PHONY: clean
clean: clean-cross-build ## Clean all build artifacts

.PHONY: clean-cross-build
clean-cross-build: ## Clean cross-build artifacts
	$(RM) -r '$(GO_BUILD_BINDIR)'
	if [ -d '$(OUTPUT_DIR)' ]; then \
		$(RM) -r '$(OUTPUT_DIR)'; \
	fi

.PHONY: version
version: build ## Display version of built binary
	@$(GO_BUILD_BINDIR)/kubectl-cluster_compare --version

.PHONY: dependency-sync
dependency-sync: ## Sync Go dependencies (work, vendor, and mod)
	go work sync
	go work vendor
	go mod tidy
