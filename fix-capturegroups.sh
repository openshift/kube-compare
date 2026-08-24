#!/bin/bash
sed -i 's/type CapturedValues struct {/\/\/ CapturedValues holds captured values from regex.\ntype CapturedValues struct {/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type CapturegroupsInlineDiff struct{}/\/\/ CapturegroupsInlineDiff handles inline diffs for capture groups.\ntype CapturegroupsInlineDiff struct{}/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type CgInfo struct {/\/\/ CgInfo holds information about a capture group.\ntype CgInfo struct {/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Return a list of the valid-looking capturegroup indices within the given pattern string./\/\/ CapturegroupIndex returns a list of the valid-looking capturegroup indices within the given pattern string./' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Transforms all non-capturegroup text in the pattern via Regex.QuoteMeta(),/\/\/ CapturegroupQuoteMeta transforms all non-capturegroup text in the pattern via Regex.QuoteMeta(),/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/func NewDiffInfo(pattern string, sharedCapturedValues CapturedValues) \*diffInfo {/\/\/ NewDiffInfo creates a new DiffInfo.\nfunc NewDiffInfo(pattern string, sharedCapturedValues CapturedValues) DiffInfo {\n/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type diffInfo struct/\/\/ DiffInfo holds info about a diff.\ntype DiffInfo struct/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\*diffInfo/\*DiffInfo/g' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/o := diffInfo/o := DiffInfo/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/return &o/return o/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Main entrypoint called by compare.go/\/\/ Diff is the main entrypoint called by compare.go/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Validation entrypoint called by referenceV2.go/\/\/ Validate is the validation entrypoint called by referenceV2.go/' pkg/compare/capturegroupsInlineDiff.go
