#!/bin/bash

# addon-tools/helm-convert/convert/convert.go
sed -i 's/RunE: func(_ \*cobra.Command, args \[\]string) error {/RunE: func(_ \*cobra.Command, _ \[\]string) error {/' addon-tools/helm-convert/convert/convert.go

# addon-tools/report-creator/report/create.go
sed -i '1i // Package report provides report creation utilities.' addon-tools/report-creator/report/create.go
sed -i 's/RunE: func(_ \*cobra.Command, args \[\]string) error {/RunE: func(_ \*cobra.Command, _ \[\]string) error {/' addon-tools/report-creator/report/create.go

# cmd/kubectl-cluster_compare.go
# Remove detached comment
sed -i '/^\/\/ Package main provides the CLI./d' cmd/kubectl-cluster_compare.go
sed -i '1i // Package main provides the CLI.' cmd/kubectl-cluster_compare.go

# pkg/compare/compare.go
sed -i 's/cmd.SetFlagErrorFunc(func(command \*cobra.Command, err error) error {/cmd.SetFlagErrorFunc(func(_ \*cobra.Command, err error) error {/' pkg/compare/compare.go
sed -i 's/func(cmd \*cobra.Command, args \[\]string, toComplete string) (\[\]string, cobra.ShellCompDirective) {/func(_ \*cobra.Command, args \[\]string, toComplete string) (\[\]string, cobra.ShellCompDirective) {/' pkg/compare/compare.go
sed -i 's/numDiffCRs += 1/numDiffCRs++/' pkg/compare/compare.go
sed -i 's/numPatched += 1/numPatched++/' pkg/compare/compare.go

# pkg/compare/container_test.go
# revert back the lookPath args
sed -i 's/lookPath = func(cmd string) (string, error) {/lookPath = func(c string) (string, error) {/g' pkg/compare/container_test.go
sed -i 's/&& cmd ==/&& c ==/g' pkg/compare/container_test.go

# pkg/compare/correlator.go
sed -i 's/var FieldSeparator = "_"/\/\/ FieldSeparator is the default field separator.\nvar FieldSeparator = "_"/g' pkg/compare/correlator.go
sed -i 's/func NewMultiCorrelator\[T CorrelationEntry\](correlators \[\]Correlator\[T\]) \*MultiCorrelator\[T\] {/\/\/ NewMultiCorrelator creates a new multi correlator.\nfunc NewMultiCorrelator\[T CorrelationEntry\](correlators \[\]Correlator\[T\]) \*MultiCorrelator\[T\] {/g' pkg/compare/correlator.go
sed -i 's/func (c \*MultiCorrelator\[T\]) AddCorrelator(correlator Correlator\[T\]) {/\/\/ AddCorrelator adds a correlator to the multi correlator.\nfunc (c \*MultiCorrelator\[T\]) AddCorrelator(correlator Correlator\[T\]) {/g' pkg/compare/correlator.go
sed -i 's/func (c MultiCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/\/\/ Match matches the object using all correlators.\nfunc (c MultiCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/g' pkg/compare/correlator.go
sed -i 's/type CorrelationEntry interface {/\/\/ CorrelationEntry is an interface for correlation entries.\ntype CorrelationEntry interface {/g' pkg/compare/correlator.go
sed -i 's/func NewExactMatchCorrelator\[T CorrelationEntry\](matchPairs map\[string\]string, templates \[\]T) (\*ExactMatchCorrelator\[T\], error) {/\/\/ NewExactMatchCorrelator creates a new exact match correlator.\nfunc NewExactMatchCorrelator\[T CorrelationEntry\](matchPairs map\[string\]string, templates \[\]T) (\*ExactMatchCorrelator\[T\], error) {/g' pkg/compare/correlator.go
sed -i 's/func (c ExactMatchCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/\/\/ Match matches the object using exact matching.\nfunc (c ExactMatchCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/g' pkg/compare/correlator.go
sed -i 's/groupHashFunc := func(cr \*unstructured.Unstructured, replaceEmptyWith string) (group string, err error) {/groupHashFunc := func(cr \*unstructured.Unstructured, _ string) (group string, err error) {/g' pkg/compare/correlator.go
sed -i 's/func (c \*GroupCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/\/\/ Match matches the object using group matching.\nfunc (c \*GroupCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/g' pkg/compare/correlator.go
sed -i 's/c.MatchedTemplatesNames\[temp.GetIdentifier()\] += 1/c.MatchedTemplatesNames\[temp.GetIdentifier()\]++/g' pkg/compare/correlator.go

# pkg/compare/funcmap.go
sed -i 's/var FuncHelp = make(map\[string\]string)/\/\/ FuncHelp holds documentation for functions.\nvar FuncHelp = make(map\[string\]string)/g' pkg/compare/funcmap.go
sed -i 's/const SprigImportFlag = `<<sprig>>`/\/\/ SprigImportFlag is used to flag sprig imports.\nconst SprigImportFlag = `<<sprig>>`/g' pkg/compare/funcmap.go
sed -i 's/f\["include"\] = func(name string, data any) (string, error) {/f\["include"\] = func(_ string, data any) (string, error) {/g' pkg/compare/funcmap.go
sed -i 's/func DisplayFuncmap(w io.Writer) error {/\/\/ DisplayFuncmap writes the available functions to the writer.\nfunc DisplayFuncmap(w io.Writer) error {/g' pkg/compare/funcmap.go
sed -i 's/\/\/ In order to use `lookupCRs` and `lookupCR`, AllCRs must be populated/\/\/ AllCRs holds all custom resources. In order to use `lookupCRs` and `lookupCR`, AllCRs must be populated./g' pkg/compare/funcmap.go
sed -i 's/type DoNotMatch struct {/\/\/ DoNotMatch indicates a regex should not match.\ntype DoNotMatch struct {/g' pkg/compare/funcmap.go

# pkg/compare/output.go
sed -i 's/func (s DiffSum) HasDiff() bool {/\/\/ HasDiff returns whether there is a diff.\nfunc (s DiffSum) HasDiff() bool {/g' pkg/compare/output.go
sed -i 's/func (s DiffSum) WasPatched() bool {/\/\/ WasPatched returns whether it was patched.\nfunc (s DiffSum) WasPatched() bool {/g' pkg/compare/output.go
sed -i 's/s.UnmatchedCRS = lo.Map(c.UnMatchedCRs, func(r \*unstructured.Unstructured, i int) string {/s.UnmatchedCRS = lo.Map(c.UnMatchedCRs, func(r \*unstructured.Unstructured, _ int) string {/g' pkg/compare/output.go
sed -i 's/s.MatchedByReferenceOnly = lo.Map(matchedByReferenceOnly, func(t ReferenceTemplate, i int) string {/s.MatchedByReferenceOnly = lo.Map(matchedByReferenceOnly, func(t ReferenceTemplate, _ int) string {/g' pkg/compare/output.go
sed -i 's/func (o Output) Print(format string, out io.Writer, showEmptyDiffs bool) (int, error) {/\/\/ Print outputs the results.\nfunc (o Output) Print(format string, out io.Writer, showEmptyDiffs bool) (int, error) {/g' pkg/compare/output.go

# pkg/compare/output_test.go
sed -i 's/actualFailCount += 1/actualFailCount++/g' pkg/compare/output_test.go
sed -i 's/actualSkipCount += 1/actualSkipCount++/g' pkg/compare/output_test.go

# pkg/compare/referenceV2.go
sed -i 's/func (g \*AnyOf) getMissingCRs(matchedTemplates map\[string\]int) (ValidationIssue, int) {/func (g \*AnyOf) getMissingCRs(_ map\[string\]int) (ValidationIssue, int) {/g' pkg/compare/referenceV2.go

# pkg/compare/regexInlineDiff.go
sed -i 's/type RegexInlineDiff struct{}/\/\/ RegexInlineDiff handles inline diffs for regex.\ntype RegexInlineDiff struct{}/g' pkg/compare/regexInlineDiff.go
sed -i 's/func (id RegexInlineDiff) Diff(regex, crValue string, sharedCapturedValues CapturedValues) (string, CapturedValues) {/\/\/ Diff returns the inline diff for regex.\nfunc (id RegexInlineDiff) Diff(regex, crValue string, sharedCapturedValues CapturedValues) (string, CapturedValues) {/g' pkg/compare/regexInlineDiff.go
sed -i 's/func (id RegexInlineDiff) Validate(regex string) error {/\/\/ Validate validates a regex.\nfunc (id RegexInlineDiff) Validate(regex string) error {/g' pkg/compare/regexInlineDiff.go

# pkg/generate/config.go
sed -i '1i // Package generate provides generation utilities.' pkg/generate/config.go

# pkg/junit/junit.go
# Remove detached comment and add clean one
sed -i '/^\/\/ Package junit provides junit generation utilities./d' pkg/junit/junit.go
sed -i '1i // Package junit provides junit utilities.' pkg/junit/junit.go
sed -i 's/func NewTestSuites(name string) \*TestSuites {/\/\/ NewTestSuites creates a new TestSuites.\nfunc NewTestSuites(name string) \*TestSuites {/g' pkg/junit/junit.go
sed -i 's/func (id \*TestSuites) AddSuite(suite TestSuite) {/\/\/ AddSuite adds a TestSuite.\nfunc (id \*TestSuites) AddSuite(suite TestSuite) {/g' pkg/junit/junit.go
sed -i 's/func (id \*TestSuites) WithSuite(suite TestSuite) \*TestSuites {/\/\/ WithSuite adds a TestSuite and returns TestSuites.\nfunc (id \*TestSuites) WithSuite(suite TestSuite) \*TestSuites {/g' pkg/junit/junit.go
sed -i 's/func (id \*TestSuite) AddCase(tcase TestCase) {/\/\/ AddCase adds a TestCase.\nfunc (id \*TestSuite) AddCase(tcase TestCase) {/g' pkg/junit/junit.go
sed -i 's/id.Tests += 1/id.Tests++/g' pkg/junit/junit.go
sed -i 's/id.Failures += 1/id.Failures++/g' pkg/junit/junit.go
sed -i 's/id.Skipped += 1/id.Skipped++/g' pkg/junit/junit.go
sed -i 's/func (id \*TestSuite) WithCase(tcase TestCase) \*TestSuite {/\/\/ WithCase adds a TestCase and returns TestSuite.\nfunc (id \*TestSuite) WithCase(tcase TestCase) \*TestSuite {/g' pkg/junit/junit.go
sed -i 's/func NewTestSuite(name string) TestSuite {/\/\/ NewTestSuite creates a new TestSuite.\nfunc NewTestSuite(name string) TestSuite {/g' pkg/junit/junit.go
sed -i 's/func Marshal(suites TestSuites) (\[\]byte, error) {/\/\/ Marshal marshals TestSuites to XML.\nfunc Marshal(suites TestSuites) (\[\]byte, error) {/g' pkg/junit/junit.go
sed -i 's/func Write(out io.Writer, suites TestSuites) error {/\/\/ Write writes TestSuites XML to writer.\nfunc Write(out io.Writer, suites TestSuites) error {/g' pkg/junit/junit.go

# pkg/objectmeta/server.go
sed -i '/^\/\/ Package objectmeta provides object metadata utilities./d' pkg/objectmeta/server.go
sed -i '1i // Package objectmeta provides object metadata utilities.' pkg/objectmeta/server.go

