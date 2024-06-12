# You can use podman or docker as a container engine. Notice that there are some options that might be only valid for one of them.
ENGINE ?= docker
IMAGE_NAME=kube-compare

PACKAGE_NAME          := github.com/openshift/kube-compare
GOLANG_CROSS_VERSION  ?= v1.22.3


.PHONY: build
build:
	go build ./cmd/kubectl-cluster_compare.go

.PHONY: test
test:
	go test --race ./pkg/*

.PHONY: test-report-creator
test-report-creator:
	go test --race ./addon-tools/report-creator/report

.PHONY: build-report-creator
build-report-creator:
	go build ./addon-tools/report-creator/create-report.go

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
markdownlint: markdownlint-image  ## run the markdown linter
	$(ENGINE) run \
		--rm=true \
		--env RUN_LOCAL=true \
		--env VALIDATE_MARKDOWN=true \
		--env PULL_BASE_SHA=$(PULL_BASE_SHA) \
		-v $$(pwd):/workdir:Z \
		$(IMAGE_NAME)-markdownlint:latest


.PHONY: release-dry-run
release-dry-run:
	@$(ENGINE) run \
		--rm \
		-e CGO_ENABLED=1 \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(PACKAGE_NAME) \
		-w /go/src/$(PACKAGE_NAME) \
		ghcr.io/goreleaser/goreleaser-cross:${GOLANG_CROSS_VERSION} \
		release --clean --skip=validate --skip=publish

.PHONY: release
release:
	@$(ENGINE) run \
		--rm \
		-e GITHUB_TOKEN=$(GITHUB_TOKEN) \
		-e CGO_ENABLED=1 \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(PACKAGE_NAME) \
		-w /go/src/$(PACKAGE_NAME) \
		ghcr.io/goreleaser/goreleaser-cross:${GOLANG_CROSS_VERSION} \
		release --clean
