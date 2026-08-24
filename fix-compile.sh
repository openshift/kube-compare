#!/bin/bash

# capturegroupsInlineDiff.go
sed -i 's/o := diffInfo/o := DiffInfo/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/return &o/return o/' pkg/compare/capturegroupsInlineDiff.go

# inlineDiffType
find pkg/compare -type f -exec sed -i 's/inlineDiffType/InlineDiffType/g' {} +
# It should be InlineDiffType everywhere now.
