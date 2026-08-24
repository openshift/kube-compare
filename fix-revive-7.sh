#!/bin/bash
# Remove detached package comments
sed -i '/^\/\/ Package main provides the tool./d' addon-tools/helm-convert/helm-convert.go
sed -i '1i // Package main provides the tool.' addon-tools/helm-convert/helm-convert.go

sed -i '/^\/\/ Package main provides the CLI./d' cmd/kubectl-cluster_compare.go
sed -i '1i // Package main provides the CLI.' cmd/kubectl-cluster_compare.go

sed -i '/^\/\/ Package compare provides compare utilities./d' pkg/compare/capturegroupsInlineDiff.go
sed -i '1i // Package compare provides compare utilities.' pkg/compare/capturegroupsInlineDiff.go

sed -i 's/Json      string = "json"/JSON      string = "json"/g' pkg/compare/compare.go
sed -i 's/Json/JSON/g' pkg/compare/compare.go

sed -i 's/func(_ \*cobra.Command, args \[\]string, toComplete string) (\[\]string, cobra.ShellCompDirective) {/func(_ \*cobra.Command, _ \[\]string, toComplete string) (\[\]string, cobra.ShellCompDirective) {/g' pkg/compare/compare.go

sed -i 's/func(c string) (string, error) {/func(_ string) (string, error) {/g' pkg/compare/container_test.go

sed -i 's/f\["include"\] = func(_ string, data any) (string, error) {/f\["include"\] = func(_ string, _ any) (string, error) {/g' pkg/compare/funcmap.go

sed -i 's/defaultHttpGetAttempts = 5/defaultHTTPGetAttempts = 5/g' pkg/compare/httpfs.go
sed -i 's/defaultHttpGetAttempts/defaultHTTPGetAttempts/g' pkg/compare/httpfs.go
sed -i 's/func readHttpWithRetries/func readHTTPWithRetries/g' pkg/compare/httpfs.go
sed -i 's/readHttpWithRetries/readHTTPWithRetries/g' pkg/compare/httpfs.go

sed -i 's/func (toOmit \*FieldsToOmitV2) GetDefault() string {/\/\/ GetDefault returns the default fields to omit.\nfunc (toOmit \*FieldsToOmitV2) GetDefault() string {/g' pkg/compare/referenceV2.go
sed -i 's/func (toOmit \*FieldsToOmitV2) GetItems() map\[string\]\[\]\*ManifestPathV1 {/\/\/ GetItems returns the items.\nfunc (toOmit \*FieldsToOmitV2) GetItems() map\[string\]\[\]\*ManifestPathV1 {/g' pkg/compare/referenceV2.go
sed -i 's/type FieldsToOmitV2Entry struct {/\/\/ FieldsToOmitV2Entry holds fields to omit for v2 entry.\ntype FieldsToOmitV2Entry struct {/g' pkg/compare/referenceV2.go
sed -i 's/type ReferenceTemplateV2 struct {/\/\/ ReferenceTemplateV2 represents a reference template in v2.\ntype ReferenceTemplateV2 struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (rf ReferenceTemplateV2) GetConfig() TemplateConfig {/\/\/ GetConfig returns the template config.\nfunc (rf ReferenceTemplateV2) GetConfig() TemplateConfig {/g' pkg/compare/referenceV2.go
sed -i 's/func (rf ReferenceTemplateV2) GetDescription() string {/\/\/ GetDescription returns the description.\nfunc (rf ReferenceTemplateV2) GetDescription() string {/g' pkg/compare/referenceV2.go
sed -i 's/type ReferenceTemplateConfigV2 struct {/\/\/ ReferenceTemplateConfigV2 holds configuration.\ntype ReferenceTemplateConfigV2 struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (config ReferenceTemplateConfigV2) GetInlineDiffFuncs() map\[string\]InlineDiffType {/\/\/ GetInlineDiffFuncs returns inline diff funcs.\nfunc (config ReferenceTemplateConfigV2) GetInlineDiffFuncs() map\[string\]InlineDiffType {/g' pkg/compare/referenceV2.go
sed -i 's/type PerFieldConfigV2 struct {/\/\/ PerFieldConfigV2 holds per field config.\ntype PerFieldConfigV2 struct {/g' pkg/compare/referenceV2.go
sed -i 's/type InlineDiffType string/\/\/ InlineDiffType represents the type of inline diff.\ntype InlineDiffType string/g' pkg/compare/referenceV2.go
sed -i 's/var InlineDiffs = map\[InlineDiffType\]InlineDiff{/\/\/ InlineDiffs maps diff types to implementations.\nvar InlineDiffs = map\[InlineDiffType\]InlineDiff{/g' pkg/compare/referenceV2.go
sed -i 's/type InlineDiff interface {/\/\/ InlineDiff defines an inline diff.\ntype InlineDiff interface {/g' pkg/compare/referenceV2.go
sed -i 's/type PartV2 struct {/\/\/ PartV2 defines a part in v2.\ntype PartV2 struct {/g' pkg/compare/referenceV2.go
sed -i 's/type ComponentV2 struct {/\/\/ ComponentV2 defines a component in v2.\ntype ComponentV2 struct {/g' pkg/compare/referenceV2.go
sed -i 's/type ComponentV2Group interface {/\/\/ ComponentV2Group defines a component group.\ntype ComponentV2Group interface {/g' pkg/compare/referenceV2.go
sed -i 's/	MissingCRsMsg      = "Missing CRs"/	\/\/ MissingCRsMsg is the missing CRs message.\n	MissingCRsMsg      = "Missing CRs"/g' pkg/compare/referenceV2.go
sed -i 's/type OneOf struct {/\/\/ OneOf requires one of the components.\ntype OneOf struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (g \*OneOf) UnmarshalJSON(b \[\]byte) (err error) {/\/\/ UnmarshalJSON unmarshals JSON.\nfunc (g \*OneOf) UnmarshalJSON(b \[\]byte) (err error) {/g' pkg/compare/referenceV2.go
sed -i 's/type NoneOf struct {/\/\/ NoneOf requires none of the components.\ntype NoneOf struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (g \*NoneOf) UnmarshalJSON(b \[\]byte) (err error) {/\/\/ UnmarshalJSON unmarshals JSON.\nfunc (g \*NoneOf) UnmarshalJSON(b \[\]byte) (err error) {/g' pkg/compare/referenceV2.go
sed -i 's/type AllOf struct {/\/\/ AllOf requires all of the components.\ntype AllOf struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (g \*AllOf) UnmarshalJSON(b \[\]byte) (err error) {/\/\/ UnmarshalJSON unmarshals JSON.\nfunc (g \*AllOf) UnmarshalJSON(b \[\]byte) (err error) {/g' pkg/compare/referenceV2.go
sed -i 's/type AnyOf struct {/\/\/ AnyOf requires any of the components.\ntype AnyOf struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (g \*AnyOf) UnmarshalJSON(b \[\]byte) (err error) {/\/\/ UnmarshalJSON unmarshals JSON.\nfunc (g \*AnyOf) UnmarshalJSON(b \[\]byte) (err error) {/g' pkg/compare/referenceV2.go
sed -i 's/type AnyOneOf struct {/\/\/ AnyOneOf requires any one of the components.\ntype AnyOneOf struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (g \*AnyOneOf) UnmarshalJSON(b \[\]byte) (err error) {/\/\/ UnmarshalJSON unmarshals JSON.\nfunc (g \*AnyOneOf) UnmarshalJSON(b \[\]byte) (err error) {/g' pkg/compare/referenceV2.go
sed -i 's/type AllOrNoneOf struct {/\/\/ AllOrNoneOf requires all or none of the components.\ntype AllOrNoneOf struct {/g' pkg/compare/referenceV2.go
sed -i 's/func (g \*AllOrNoneOf) UnmarshalJSON(b \[\]byte) (err error) {/\/\/ UnmarshalJSON unmarshals JSON.\nfunc (g \*AllOrNoneOf) UnmarshalJSON(b \[\]byte) (err error) {/g' pkg/compare/referenceV2.go
sed -i 's/func ParseV2Templates(ref \*ReferenceV2, fsys fs.FS) (\[\]ReferenceTemplate, error) {/\/\/ ParseV2Templates parses v2 templates.\nfunc ParseV2Templates(ref \*ReferenceV2, fsys fs.FS) (\[\]ReferenceTemplate, error) {/g' pkg/compare/referenceV2.go

sed -i 's/ApiVersion   string    `json:"apiVersion,omitempty"`/APIVersion   string    `json:"apiVersion,omitempty"`/g' pkg/compare/useroverride.go
sed -i 's/func (o UserOverride) GetIdentifier() string {/\/\/ GetIdentifier returns the identifier.\nfunc (o UserOverride) GetIdentifier() string {/g' pkg/compare/useroverride.go
sed -i 's/func (o UserOverride) GetName() string {/\/\/ GetName returns the name.\nfunc (o UserOverride) GetName() string {/g' pkg/compare/useroverride.go
sed -i 's/func (o UserOverride) GetMetadata() \*unstructured.Unstructured {/\/\/ GetMetadata returns the metadata.\nfunc (o UserOverride) GetMetadata() \*unstructured.Unstructured {/g' pkg/compare/useroverride.go
sed -i 's/func (o UserOverride) Apply(rendered, live \*unstructured.Unstructured) (\*unstructured.Unstructured, error) {/\/\/ Apply applies the override.\nfunc (o UserOverride) Apply(rendered, live \*unstructured.Unstructured) (\*unstructured.Unstructured, error) {/g' pkg/compare/useroverride.go
sed -i 's/func CreateMergePatch(temp ReferenceTemplate, obj \*InfoObject, reason string) (\*UserOverride, error) {/\/\/ CreateMergePatch creates a merge patch.\nfunc CreateMergePatch(temp ReferenceTemplate, obj \*InfoObject, reason string) (\*UserOverride, error) {/g' pkg/compare/useroverride.go
sed -i 's/func LoadUserOverrides(path string) (\[\]\*UserOverride, error) {/\/\/ LoadUserOverrides loads user overrides.\nfunc LoadUserOverrides(path string) (\[\]\*UserOverride, error) {/g' pkg/compare/useroverride.go

sed -i '/^\/\/ Package generate provides generation utilities./d' pkg/generate/config.go
sed -i '1i // Package generate provides generation utilities.' pkg/generate/config.go

sed -i '/^\/\/ Package objectmeta provides object metadata utilities./d' pkg/objectmeta/server.go
sed -i '1i // Package objectmeta provides object metadata utilities.' pkg/objectmeta/server.go

