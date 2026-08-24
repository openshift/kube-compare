#!/bin/bash

# pkg/compare/referenceV1.go
sed -i 's/const ReferenceVersionV1 string = "v1"/\/\/ ReferenceVersionV1 is the v1 reference version.\nconst ReferenceVersionV1 string = "v1"/' pkg/compare/referenceV1.go
sed -i 's/type ReferenceV1 struct {/\/\/ ReferenceV1 is a v1 reference.\ntype ReferenceV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/type PartV1 struct {/\/\/ PartV1 is a v1 part.\ntype PartV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/type ComponentTypeV1 string/\/\/ ComponentTypeV1 is a v1 component type.\ntype ComponentTypeV1 string/' pkg/compare/referenceV1.go
sed -i 's/	Required ComponentTypeV1 = "Required"/	\/\/ Required is a required component type.\n	Required ComponentTypeV1 = "Required"/' pkg/compare/referenceV1.go
sed -i 's/type ComponentV1 struct {/\/\/ ComponentV1 is a v1 component.\ntype ComponentV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/func (r \*ReferenceV1) GetAPIVersion() string {/\/\/ GetAPIVersion returns the API version.\nfunc (r \*ReferenceV1) GetAPIVersion() string {/' pkg/compare/referenceV1.go
sed -i 's/func (r \*ReferenceV1) GetTemplates() \[\]ReferenceTemplate {/\/\/ GetTemplates returns the templates.\nfunc (r \*ReferenceV1) GetTemplates() \[\]ReferenceTemplate {/' pkg/compare/referenceV1.go
sed -i 's/func (r \*ReferenceV1) GetFieldsToOmit() FieldsToOmit {/\/\/ GetFieldsToOmit returns the fields to omit.\nfunc (r \*ReferenceV1) GetFieldsToOmit() FieldsToOmit {/' pkg/compare/referenceV1.go
sed -i 's/func (r \*ReferenceV1) GetTemplateFunctionFiles() \[\]string {/\/\/ GetTemplateFunctionFiles returns the template function files.\nfunc (r \*ReferenceV1) GetTemplateFunctionFiles() \[\]string {/' pkg/compare/referenceV1.go
sed -i 's/func (r \*ReferenceV1) GetValidationIssues(matchedTemplates map\[string\]int) (map\[string\]map\[string\]ValidationIssue, int) {/\/\/ GetValidationIssues returns the validation issues.\nfunc (r \*ReferenceV1) GetValidationIssues(matchedTemplates map\[string\]int) (map\[string\]map\[string\]ValidationIssue, int) {/' pkg/compare/referenceV1.go
sed -i 's/type FieldsToOmitV1 struct {/\/\/ FieldsToOmitV1 holds fields to omit for v1.\ntype FieldsToOmitV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/func (toOmit \*FieldsToOmitV1) GetDefault() string {/\/\/ GetDefault returns the default fields to omit.\nfunc (toOmit \*FieldsToOmitV1) GetDefault() string {/' pkg/compare/referenceV1.go
sed -i 's/func (toOmit \*FieldsToOmitV1) GetItems() map\[string\]\[\]\*ManifestPathV1 {/\/\/ GetItems returns the items to omit.\nfunc (toOmit \*FieldsToOmitV1) GetItems() map\[string\]\[\]\*ManifestPathV1 {/' pkg/compare/referenceV1.go
sed -i 's/type ReferenceTemplateConfigV1 struct {/\/\/ ReferenceTemplateConfigV1 holds configuration for v1 reference templates.\ntype ReferenceTemplateConfigV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/func (config ReferenceTemplateConfigV1) GetAllowMerge() bool {/\/\/ GetAllowMerge returns whether merging is allowed.\nfunc (config ReferenceTemplateConfigV1) GetAllowMerge() bool {/' pkg/compare/referenceV1.go

# Handle GetInlineDiffFuncs unexported return
sed -i 's/type inlineDiffType/type InlineDiffType/' pkg/compare/referenceV1.go
sed -i 's/inlineDiffType/InlineDiffType/g' pkg/compare/referenceV1.go
sed -i 's/func (config ReferenceTemplateConfigV1) GetInlineDiffFuncs() map\[string\]InlineDiffType {/\/\/ GetInlineDiffFuncs returns the inline diff functions.\nfunc (config ReferenceTemplateConfigV1) GetInlineDiffFuncs() map\[string\]InlineDiffType {/' pkg/compare/referenceV1.go

sed -i 's/func (config ReferenceTemplateConfigV1) GetFieldsToOmitRefs() \[\]string {/\/\/ GetFieldsToOmitRefs returns the fields to omit references.\nfunc (config ReferenceTemplateConfigV1) GetFieldsToOmitRefs() \[\]string {/' pkg/compare/referenceV1.go
sed -i 's/type ReferenceTemplateV1 struct {/\/\/ ReferenceTemplateV1 is a v1 reference template.\ntype ReferenceTemplateV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetFieldsToOmit(fieldsToOmit FieldsToOmit) \[\]\*ManifestPathV1 {/\/\/ GetFieldsToOmit returns the fields to omit for this template.\nfunc (rf ReferenceTemplateV1) GetFieldsToOmit(fieldsToOmit FieldsToOmit) \[\]\*ManifestPathV1 {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) ValidateFieldsToOmit(fieldsToOmit FieldsToOmit) error {/\/\/ ValidateFieldsToOmit validates the fields to omit.\nfunc (rf ReferenceTemplateV1) ValidateFieldsToOmit(fieldsToOmit FieldsToOmit) error {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) Exec(params map\[string\]any) (\*unstructured.Unstructured, error) {/\/\/ Exec executes the template.\nfunc (rf ReferenceTemplateV1) Exec(params map\[string\]any) (\*unstructured.Unstructured, error) {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetPath() string {/\/\/ GetPath returns the template path.\nfunc (rf ReferenceTemplateV1) GetPath() string {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetIdentifier() string {/\/\/ GetIdentifier returns the template identifier.\nfunc (rf ReferenceTemplateV1) GetIdentifier() string {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetDescription() string {/\/\/ GetDescription returns the template description.\nfunc (rf ReferenceTemplateV1) GetDescription() string {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetMetadata() \*unstructured.Unstructured {/\/\/ GetMetadata returns the template metadata.\nfunc (rf ReferenceTemplateV1) GetMetadata() \*unstructured.Unstructured {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetConfig() TemplateConfig {/\/\/ GetConfig returns the template config.\nfunc (rf ReferenceTemplateV1) GetConfig() TemplateConfig {/' pkg/compare/referenceV1.go
sed -i 's/func (rf ReferenceTemplateV1) GetTemplateTree() \*parse.Tree {/\/\/ GetTemplateTree returns the parsed template tree.\nfunc (rf ReferenceTemplateV1) GetTemplateTree() \*parse.Tree {/' pkg/compare/referenceV1.go
sed -i 's/type ManifestPathV1 struct {/\/\/ ManifestPathV1 represents a manifest path in v1.\ntype ManifestPathV1 struct {/' pkg/compare/referenceV1.go
sed -i 's/func (p \*ManifestPathV1) Process() error {/\/\/ Process processes the manifest path.\nfunc (p \*ManifestPathV1) Process() error {/' pkg/compare/referenceV1.go
sed -i 's/func ParseV1Templates(ref \*ReferenceV1, fsys fs.FS) (\[\]ReferenceTemplate, error) {/\/\/ ParseV1Templates parses templates for a v1 reference.\nfunc ParseV1Templates(ref \*ReferenceV1, fsys fs.FS) (\[\]ReferenceTemplate, error) {/' pkg/compare/referenceV1.go

# pkg/compare/unstructured.go
sed -i 's/func NestedString(obj any, fields ...string) (string, bool, error) {/\/\/ NestedString returns the string value of a nested field.\nfunc NestedString(obj any, fields ...string) (string, bool, error) {/' pkg/compare/unstructured.go
sed -i 's/func SetNestedString(obj any, value string, fields ...string) error {/\/\/ SetNestedString sets the string value of a nested field.\nfunc SetNestedString(obj any, value string, fields ...string) error {/' pkg/compare/unstructured.go

# pkg/objectmeta/server.go
sed -i '1i // Package objectmeta provides object metadata utilities.' pkg/objectmeta/server.go

# pkg/testutils/testutils.go
sed -i '1i // Package testutils provides testing utilities.' pkg/testutils/testutils.go
sed -i 's/func GetFile(t \*testing.T, fileName, value string, update bool) string {/\/\/ GetFile gets or updates a test file.\nfunc GetFile(t \*testing.T, fileName, value string, update bool) string {/' pkg/testutils/testutils.go
sed -i 's/type FixupOptions struct {/\/\/ FixupOptions holds options for fixing up test data.\ntype FixupOptions struct {/' pkg/testutils/testutils.go
sed -i 's/func RemoveInconsistentInfo(t \*testing.T, text string, opt FixupOptions) string {/\/\/ RemoveInconsistentInfo removes inconsistent info from test data.\nfunc RemoveInconsistentInfo(t \*testing.T, text string, opt FixupOptions) string {/' pkg/testutils/testutils.go

