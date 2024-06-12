#!/bin/bash

golangci_lint=$(which golangci-lint)
if [ ! -f "${golangci_lint}" ]; then
    echo "Failed to find required command: golangci_lint "
    exit 1
fi

export GOCACHE=/tmp/
export GOLANGCI_LINT_CACHE=/tmp/.cache
"${golangci_lint}" version
"${golangci_lint}" run --verbose --print-resources-usage --timeout=5m0s --new-from-rev=722af08 $(go work edit -json | jq -c -r '[.Use[].DiskPath] | map_values(. + "/...")[]' | tr '\n' ' ')
