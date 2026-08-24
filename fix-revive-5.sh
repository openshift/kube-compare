#!/bin/bash
sed -i 's/DiffSeparator         = "**********************************\\n"/\/\/ DiffSeparator separates diff outputs.\n\tDiffSeparator         = "**********************************\\n"/g' pkg/compare/compare.go
sed -i 's/Json      string = "json"/\/\/ Json is json format.\n\tJson      string = "json"/g' pkg/compare/compare.go
sed -i 's/var OutputFormats = \[\]string{Json, Yaml, PatchYaml, Junit}/\/\/ OutputFormats defines the available output formats.\nvar OutputFormats = \[\]string{Json, Yaml, PatchYaml, Junit}/g' pkg/compare/compare.go
sed -i 's/type Options struct {/\/\/ Options holds command options.\ntype Options struct {/g' pkg/compare/compare.go
sed -i 's/func NewCmd(f kcmdutil.Factory, streams genericiooptions.IOStreams) \*cobra.Command {/\/\/ NewCmd creates a new compare command.\nfunc NewCmd(f kcmdutil.Factory, streams genericiooptions.IOStreams) \*cobra.Command {/g' pkg/compare/compare.go
sed -i 's/func NewOptions(ioStreams genericiooptions.IOStreams) \*Options {/\/\/ NewOptions creates new options.\nfunc NewOptions(ioStreams genericiooptions.IOStreams) \*Options {/g' pkg/compare/compare.go
sed -i 's/func (o \*Options) GetRefFS() (fs.FS, error) {/\/\/ GetRefFS returns the reference file system.\nfunc (o \*Options) GetRefFS() (fs.FS, error) {/g' pkg/compare/compare.go
sed -i 's/func (o \*Options) Complete(f kcmdutil.Factory, cmd \*cobra.Command, args \[\]string) error {/\/\/ Complete completes the options.\nfunc (o \*Options) Complete(f kcmdutil.Factory, cmd \*cobra.Command, args \[\]string) error {/g' pkg/compare/compare.go
sed -i 's/type MergeError struct {/\/\/ MergeError represents an error during merge.\ntype MergeError struct {/g' pkg/compare/compare.go
sed -i 's/type InlineDiffError struct {/\/\/ InlineDiffError represents an inline diff error.\ntype InlineDiffError struct {/g' pkg/compare/compare.go
sed -i 's/func (obj InfoObject) Name() string {/\/\/ Name returns the name of the object.\nfunc (obj InfoObject) Name() string {/g' pkg/compare/compare.go

sed -i 's/requestedResources := lo.Map(resourcesByKind\[p\], func(value \*unstructured.Unstructured, index int) any {/requestedResources := lo.Map(resourcesByKind\[p\], func(value \*unstructured.Unstructured, _ int) any {/g' pkg/compare/compare_test.go
sed -i 's/func(path string, info os.FileInfo, err error) error {/func(path string, _ os.FileInfo, err error) error {/g' pkg/compare/compare_test.go

sed -i 's/func NewMetricsTracker() \*MetricsTracker {/\/\/ NewMetricsTracker creates a new metrics tracker.\nfunc NewMetricsTracker() \*MetricsTracker {/g' pkg/compare/correlator.go
sed -i 's/type FieldCorrelator\[T CorrelationEntry\] struct {/\/\/ FieldCorrelator correlates by field.\ntype FieldCorrelator\[T CorrelationEntry\] struct {/g' pkg/compare/correlator.go
sed -i 's/func (f \*FieldCorrelator\[T\]) ClaimTemplates(templates \[\]T) \[\]T {/\/\/ ClaimTemplates claims templates.\nfunc (f \*FieldCorrelator\[T\]) ClaimTemplates(templates \[\]T) \[\]T {/g' pkg/compare/correlator.go
sed -i 's/func (f \*FieldCorrelator\[T\]) ValidateTemplates() error {/\/\/ ValidateTemplates validates templates.\nfunc (f \*FieldCorrelator\[T\]) ValidateTemplates() error {/g' pkg/compare/correlator.go
sed -i 's/func (f FieldCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/\/\/ Match matches the object.\nfunc (f FieldCorrelator\[T\]) Match(object \*unstructured.Unstructured) (\[\]T, error) {/g' pkg/compare/correlator.go
sed -i 's/group_hash, err := f.hashFunc(object, "")/groupHash, err := f.hashFunc(object, "")/g' pkg/compare/correlator.go
sed -i 's/group_hash/groupHash/g' pkg/compare/correlator.go

sed -i 's/type Reference interface {/\/\/ Reference defines a reference.\ntype Reference interface {/g' pkg/compare/parsing.go
sed -i 's/type ReferenceTemplate interface {/\/\/ ReferenceTemplate defines a reference template.\ntype ReferenceTemplate interface {/g' pkg/compare/parsing.go
sed -i 's/type TemplateConfig interface {/\/\/ TemplateConfig defines template configuration.\ntype TemplateConfig interface {/g' pkg/compare/parsing.go
sed -i 's/type FieldsToOmit interface {/\/\/ FieldsToOmit defines fields to omit.\ntype FieldsToOmit interface {/g' pkg/compare/parsing.go
sed -i 's/func GetReference(fsys fs.FS, referenceFileName string) (Reference, error) {/\/\/ GetReference gets the reference.\nfunc GetReference(fsys fs.FS, referenceFileName string) (Reference, error) {/g' pkg/compare/parsing.go
sed -i 's/type UserConfig struct {/\/\/ UserConfig holds user config.\ntype UserConfig struct {/g' pkg/compare/parsing.go
sed -i 's/type CorrelationSettings struct {/\/\/ CorrelationSettings holds correlation settings.\ntype CorrelationSettings struct {/g' pkg/compare/parsing.go
sed -i 's/type ManualCorrelation struct {/\/\/ ManualCorrelation holds manual correlation.\ntype ManualCorrelation struct {/g' pkg/compare/parsing.go
sed -i 's/func ParseTemplates(ref Reference, fsys fs.FS) (\[\]ReferenceTemplate, error) {/\/\/ ParseTemplates parses templates.\nfunc ParseTemplates(ref Reference, fsys fs.FS) (\[\]ReferenceTemplate, error) {/g' pkg/compare/parsing.go
sed -i 's/type CRMetadata struct {/\/\/ CRMetadata holds metadata.\ntype CRMetadata struct {/g' pkg/compare/parsing.go
sed -i 's/type ValidationIssue struct {/\/\/ ValidationIssue holds a validation issue.\ntype ValidationIssue struct {/g' pkg/compare/parsing.go

sed -i 's/	Optional ComponentTypeV1 = "Optional"/	\/\/ Optional is an optional component.\n	Optional ComponentTypeV1 = "Optional"/g' pkg/compare/referenceV1.go
sed -i 's/const ReferenceVersionV2 string = "v2"/\/\/ ReferenceVersionV2 is the v2 reference version.\nconst ReferenceVersionV2 string = "v2"/g' pkg/compare/referenceV2.go
sed -i 's/type ReferenceV2 struct {/\/\/ ReferenceV2 is a v2 reference.\ntype ReferenceV2 struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (r \*ReferenceV2) GetAPIVersion() string {/\/\/ GetAPIVersion returns the API version.\nfunc (r \*ReferenceV2) GetAPIVersion() string {/g' pkg/compare/referenceV2.go
sed -i 's/func (r \*ReferenceV2) GetTemplates() \[\]ReferenceTemplate {/\/\/ GetTemplates returns the templates.\nfunc (r \*ReferenceV2) GetTemplates() \[\]ReferenceTemplate {/g' pkg/compare/referenceV2.go
sed -i 's/func (r \*ReferenceV2) GetFieldsToOmit() FieldsToOmit {/\/\/ GetFieldsToOmit returns the fields to omit.\nfunc (r \*ReferenceV2) GetFieldsToOmit() FieldsToOmit {/g' pkg/compare/referenceV2.go
sed -i 's/func (r \*ReferenceV2) GetTemplateFunctionFiles() \[\]string {/\/\/ GetTemplateFunctionFiles returns the template function files.\nfunc (r \*ReferenceV2) GetTemplateFunctionFiles() \[\]string {/g' pkg/compare/referenceV2.go
sed -i 's/func (r \*ReferenceV2) GetValidationIssues(matchedTemplates map\[string\]int) (map\[string\]map\[string\]ValidationIssue, int) {/\/\/ GetValidationIssues returns the validation issues.\nfunc (r \*ReferenceV2) GetValidationIssues(matchedTemplates map\[string\]int) (map\[string\]map\[string\]ValidationIssue, int) {/g' pkg/compare/referenceV2.go
sed -i 's/type FieldsToOmitV2 struct {/\/\/ FieldsToOmitV2 holds fields to omit for v2.\ntype FieldsToOmitV2 struct {/g' pkg/compare/referenceV2.go

sed -i 's/type UserOverride struct {/\/\/ UserOverride represents a user override.\ntype UserOverride struct {/g' pkg/compare/useroverride.go

