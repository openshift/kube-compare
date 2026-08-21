# Agent / Developer Guidelines

This file contains internal knowledge, gotchas, and reminders for AI agents
and developers working on this repository.

## Bumping Go Versions

When you upgrade the Go version (e.g., in `go.mod`), you **must** also update
the `GOLANG_CROSS_VERSION` variable in the `Makefile`.

**Why?**
The repository uses the `ghcr.io/goreleaser/goreleaser-cross` Docker image
to compile cross-platform binaries. The tag of this image corresponds to the
bundled Go version. If `go.mod` specifies a newer Go version than what is set
in `GOLANG_CROSS_VERSION`, CI checks (like CodeRabbit) and the release builds
will fail.

**Steps:**

1. Update the `go` directive in `go.mod`.
2. Find the corresponding release of
   [goreleaser-cross](https://github.com/goreleaser/goreleaser-cross/releases)
   that matches the new Go version.
3. Update `GOLANG_CROSS_VERSION ?= vX.Y.Z` in the `Makefile`.

## Committing

Before submitting a pull request, ensure that your code passes all locally
available verification checks. Specifically:

* `make test` or `make test-all`
* `make golangci-lint`

There is also a `make markdownlint` target to test Markdown files, but this
requires `docker` or `podman` installed locally.
