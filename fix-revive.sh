#!/bin/bash

# cmd/kubectl-cluster_compare.go
sed -i '1i // Package main provides the CLI.' cmd/kubectl-cluster_compare.go

# pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type CapturedValues struct {/\/\/ CapturedValues holds captured values from regex.\ntype CapturedValues struct {/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type CapturegroupsInlineDiff struct{}/\/\/ CapturegroupsInlineDiff handles inline diffs for capture groups.\ntype CapturegroupsInlineDiff struct{}/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type CgInfo struct {/\/\/ CgInfo holds information about a capture group.\ntype CgInfo struct {/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Return a list of the valid-looking capturegroup indices within the given pattern string./\/\/ CapturegroupIndex returns a list of the valid-looking capturegroup indices within the given pattern string./' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Transforms all non-capturegroup text in the pattern via Regex.QuoteMeta(),/\/\/ CapturegroupQuoteMeta transforms all non-capturegroup text in the pattern via Regex.QuoteMeta(),/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/func NewDiffInfo(pattern string, sharedCapturedValues CapturedValues) \*diffInfo {/\/\/ NewDiffInfo creates a new DiffInfo.\nfunc NewDiffInfo(pattern string, sharedCapturedValues CapturedValues) DiffInfo {\n/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/type diffInfo struct/\/\/ DiffInfo holds info about a diff.\ntype DiffInfo struct/' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\*diffInfo/\*DiffInfo/g' pkg/compare/capturegroupsInlineDiff.go
sed -i 's/\/\/ Main entrypoint called by compare.go/\/\/ Diff is the main entrypoint called by compare.go/' pkg/compare/capturegroupsInlineDiff.go

# container_test.go ignores
sed -i 's/func TestHelperProcess(t \*testing.T) {/func TestHelperProcess(_ \*testing.T) {/' pkg/compare/container_test.go
sed -i 's/func TestCleanup(t \*testing.T) {/func TestCleanup(_ \*testing.T) {/' pkg/compare/container_test.go
sed -i 's/lookPath = func(cmd string) (string, error) {/lookPath = func(_ string) (string, error) {/' pkg/compare/container_test.go

