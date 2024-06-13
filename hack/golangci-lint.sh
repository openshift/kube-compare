#!/bin/bash

golangci_lint=$(which golangci-lint)
if [ ! -f "${golangci_lint}" ]; then
    echo "Failed to find required command: golangci_lint "
    exit 1
fi

export GOCACHE=/tmp/
export GOLANGCI_LINT_CACHE=/tmp/.cache
"${golangci_lint}" version
"${golangci_lint}" run --verbose --print-resources-usage $(go work edit --json | grep DiskPath | awk '{print $2"/..."}' | tr -d '"' | tr '\n' ' ')
