#!/bin/bash

export GOCACHE=/tmp/
export GOLANGCI_LINT_CACHE=/tmp/.cache

export GOFLAGS="-mod=vendor"
go tool golangci-lint version
go tool golangci-lint run --verbose ./...
