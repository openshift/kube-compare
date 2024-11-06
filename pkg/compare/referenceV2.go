// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"text/template"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const ReferenceVersionV2 string = "v2"

type ReferenceV2 struct {
	Version           string `json:"apiVersion,omitempty"`
	normalisedVersion string

	Parts                 []*PartV2       `json:"parts"`
	TemplateFunctionFiles []string        `json:"templateFunctionFiles,omitempty"`
	FieldsToOmit          *FieldsToOmitV2 `json:"fieldsToOmit,omitempty"`
}

func (r *ReferenceV2) GetAPIVersion() string {
	return r.normalisedVersion
}
func (r *ReferenceV2) getTemplates() []*ReferenceTemplateV2 {
	var templates []*ReferenceTemplateV2
	for _, part := range r.Parts {
		for _, comp := range part.Components {
			templates = append(templates, comp.getTemplates()...)
		}
	}
	return templates
}

func (r *ReferenceV2) GetTemplates() []ReferenceTemplate {
	var templates []ReferenceTemplate
	// Repackage getTemplates into []ReferenceTemplate
	// because go's  (or LSPs) type checking isn't quite good enough to accept it
	for _, t := range r.getTemplates() {
		templates = append(templates, t)
	}
	return templates
}

func (r *ReferenceV2) GetFieldsToOmit() FieldsToOmit {
	return r.FieldsToOmit
}

func (r *ReferenceV2) GetTemplateFunctionFiles() []string {
	return r.TemplateFunctionFiles
}

func (r *ReferenceV2) validate() error {
	errs := make([]error, 0)
	for _, part := range r.Parts {
		for i, comp := range part.Components {
			err := comp.validate(i)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *ReferenceV2) GetValidationIssues(matchedTemplates map[string]int) (map[string]map[string]ValidationIssue, int) {
	crs := make(map[string]map[string]ValidationIssue)
	count := 0
	for _, part := range r.Parts {
		crsInPart, countInPart := part.getValidationIssues(matchedTemplates)
		if len(crsInPart) > 0 {
			crs[part.Name] = crsInPart
			count += countInPart
		}
	}
	return crs, count
}

func getbuiltInPathsV2() []*FieldsToOmitV2Entry {
	res := make([]*FieldsToOmitV2Entry, 0)
	for _, p := range builtInPathsV1 {
		res = append(res, &FieldsToOmitV2Entry{ManifestPathV1: p})
	}
	return res
}

type FieldsToOmitV2 struct {
	DefaultOmitRef string                            `json:"defaultOmitRef,omitempty"`
	Items          map[string][]*FieldsToOmitV2Entry `json:"items,omitempty"`
	items          map[string][]*ManifestPathV1
}

func (toOmit *FieldsToOmitV2) GetDefault() string {
	return toOmit.DefaultOmitRef
}

func (toOmit *FieldsToOmitV2) GetItems() map[string][]*ManifestPathV1 {
	return toOmit.items
}

// Setup FieldsToOmit to be used by setting defaults
// and processing the item strings into paths
func (toOmit *FieldsToOmitV2) process() error {
	if toOmit.items == nil {
		toOmit.items = make(map[string][]*ManifestPathV1)
	}

	if toOmit.Items == nil {
		toOmit.Items = make(map[string][]*FieldsToOmitV2Entry)
	}

	if _, ok := toOmit.Items[builtInPathsKey]; ok {
		klog.Warningf(fieldsToOmitBuiltInOverwritten, builtInPathsKey)
	}

	errs := make([]error, 0)

	toOmit.Items[builtInPathsKey] = getbuiltInPathsV2()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if toOmit.DefaultOmitRef == "" {
		toOmit.DefaultOmitRef = builtInPathsKey
	}

	for key := range toOmit.Items {
		paths, err := processFieldsToOmitEntries(key, toOmit, []string{})
		if err != nil {
			errs = append(errs, err)
		} else {
			// TODO: we should look into dedupe the paths
			toOmit.items[key] = append(toOmit.items[key], paths...)
		}

	}
	return errors.Join(errs...)
}

func processFieldsToOmitEntries(key string, toOmit *FieldsToOmitV2, previousKeys []string) ([]*ManifestPathV1, error) {
	currentKeys := make([]string, 0)
	currentKeys = append(currentKeys, previousKeys...)
	currentKeys = append(currentKeys, key)

	errs := make([]error, 0)
	paths := make([]*ManifestPathV1, 0)
	for _, entry := range toOmit.Items[key] {
		entryPaths, err := entry.process(currentKeys, toOmit)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		paths = append(paths, entryPaths...)

	}
	return paths, errors.Join(errs...)
}

type FieldsToOmitV2Entry struct {
	*ManifestPathV1
	Include         string `json:"include,omitempty"`
	paths           []*ManifestPathV1
	processingError error
}

func (entry *FieldsToOmitV2Entry) process(previousKeys []string, toOmit *FieldsToOmitV2) ([]*ManifestPathV1, error) {
	if len(entry.paths) != 0 {
		return entry.paths, entry.processingError
	}

	paths := make([]*ManifestPathV1, 0)
	if entry.Include == "" && (entry.ManifestPathV1 == nil || entry.PathToKey == "") {
		return paths, fmt.Errorf("must have either include or pathToKey")
	}

	errs := make([]error, 0)
	if entry.ManifestPathV1 != nil && entry.PathToKey != "" {
		err := entry.ManifestPathV1.Process()
		if err != nil {
			errs = append(errs, err)
		} else {
			paths = append(paths, entry.ManifestPathV1)
		}
	}

	if entry.Include != "" {
		foundCircle := slices.Contains(previousKeys, entry.Include)
		if foundCircle {
			circularKeys := make([]string, 0)
			circularKeys = append(circularKeys, previousKeys...)
			circularKeys = append(circularKeys, entry.Include)
			return paths, fmt.Errorf("circular import found %s", strings.Join(circularKeys, " -> "))
		}

		entryPaths, err := processFieldsToOmitEntries(entry.Include, toOmit, previousKeys)
		if err != nil {
			errs = append(errs, err)
		} else {
			paths = append(paths, entryPaths...)
		}
	}

	entry.paths = append(entry.paths, paths...)
	entry.processingError = errors.Join(errs...)
	return paths, entry.processingError
}

type ReferenceTemplateV2 struct {
	Config ReferenceTemplateConfigV2 `json:"config,omitempty"`
	ReferenceTemplateV1
}

func (rf ReferenceTemplateV2) GetConfig() TemplateConfig {
	return rf.Config
}

type ReferenceTemplateConfigV2 struct {
	PerField []*PerFieldConfigV2 `json:"perField,omitempty"`
	ReferenceTemplateConfigV1
}

func (config ReferenceTemplateConfigV2) GetInlineDiffFuncs() map[string]inlineDiffType {
	diffFuncs := make(map[string]inlineDiffType)
	for _, fieldConf := range config.PerField {
		diffFuncs[fieldConf.PathToKey] = fieldConf.InlineDiffFunc
	}
	return diffFuncs
}

func (rf ReferenceTemplateV2) validateConfigPerField() error {
	for pathToKey, inlineDiffFunc := range rf.GetConfig().GetInlineDiffFuncs() {
		listedPath, err := pathToList(pathToKey)
		if err != nil {
			return fmt.Errorf("reference contains template with config per field with pathToKey that is not in "+
				"supoorted format. path: %s. error: %v", pathToKey, err)
		}
		value, exist, err := unstructured.NestedString(rf.metadata.Object, listedPath...)
		if err != nil || !exist {
			return fmt.Errorf("reference contains template with config per field with pathToKey that points to a "+
				"path that does not exist in the template. path: %s", pathToKey)
		}
		validator, ok := InlineDiffs[inlineDiffFunc]
		if !ok {
			return fmt.Errorf("reference contains template with config per field with InlineDiffFunc that does not "+
				"exist. InlineDiffFunc: %s", inlineDiffFunc)
		}
		if err := validator.validate(value); err != nil {
			return fmt.Errorf("reference contains template with config per field with InlineDiffFunc that fails "+
				"validation. InlineDiffFunc: %s. error: %v", inlineDiffFunc, err)
		}
	}
	return nil
}

type PerFieldConfigV2 struct {
	PathToKey      string         `json:"pathToKey,omitempty"`
	InlineDiffFunc inlineDiffType `json:"inlineDiffFunc,omitempty"`
}

type inlineDiffType string

const (
	regex inlineDiffType = "regex"
)

var InlineDiffs = map[inlineDiffType]InlineDiff{regex: RegexInlineDiff{}}

type InlineDiff interface {
	diff(templateValue, crValue string) string
	validate(templateValue string) error
}

type RegexInlineDiff struct{}

func constructFromParts(parts []*syntax.Regexp) *syntax.Regexp {
	if len(parts) == 0 {
		return nil
	}

	if len(parts) == 1 {
		return parts[0]
	}

	return &syntax.Regexp{
		Op:   syntax.OpConcat,
		Sub:  parts,
		Sub0: [1]*syntax.Regexp{parts[0]},
	}
}

// getRegexParts Returns a capture group or all parts before the capture group.
func getRegexParts(r *syntax.Regexp) (*syntax.Regexp, *syntax.Regexp) {
	if r.Op != syntax.OpConcat {
		return r, nil
	}
	leftoverParts := make([]*syntax.Regexp, 0)
	takenParts := make([]*syntax.Regexp, 0)
	for i, p := range r.Sub {
		if p.Op != syntax.OpCapture {
			takenParts = append(takenParts, p)
		} else {
			leftoverPartsIndex := i
			if i == 0 {
				takenParts = append(takenParts, p)
				leftoverPartsIndex = 1
			}
			leftoverParts = r.Sub[leftoverPartsIndex:]
			break
		}
	}

	return constructFromParts(takenParts), constructFromParts(leftoverParts)
}

// Constuct a literal with the current matched then add current part to it.
func getCompiledRegex(matched string, toMatch *syntax.Regexp) (*regexp.Regexp, error) {
	runes := []rune(matched)
	if len(runes) == 0 {
		return regexp.Compile(toMatch.String()) //nolint:wrapcheck
	}

	// Initialise array so we can handle only a single rune
	shortRunes := [2]rune{}
	for n := range runes {
		if n == 2 {
			break
		}
		shortRunes[n] = runes[n]
	}

	res := syntax.Regexp{
		Op: syntax.OpConcat,
		Sub: []*syntax.Regexp{
			{
				Op:    syntax.OpLiteral,
				Rune:  runes,
				Rune0: shortRunes,
			},
			toMatch,
		},
	}
	return regexp.Compile(res.String()) //nolint:wrapcheck

}

func (id RegexInlineDiff) diff(regex, crValue string) string {
	matched := ""
	var toMatch *syntax.Regexp
	leftovers, err := syntax.Parse(regex, syntax.Perl)
	if err != nil {
		return regex
	}
	namedGroupValues := map[string]string{}
	for leftovers != nil {
		toMatch, leftovers = getRegexParts(leftovers)
		re, _ := getCompiledRegex(matched, toMatch)
		matchValues := re.FindStringSubmatch(crValue)

		if len(matchValues) == 0 {
			if matched != crValue {
				return regex
			}
		}

		names := re.SubexpNames()
		if len(names) == 1 {
			// No capture groups just use the entire thing
			matched = matchValues[0]
			continue
		}

		if len(names) > 2 {
			// TODO: properly handle nested capture groups instead of ignoring
			klog.Warningf("nested match value ignored in pattern %s", re.String())
		}
		captureName := names[1]
		if captureName == "" {
			// not a named group don't need to care
			matched = matchValues[0]
			continue
		}
		seen, ok := namedGroupValues[captureName]
		matchedValue := matchValues[1]
		if ok && seen != matchedValue {
			matched += fmt.Sprintf("<previously matched value does not equal the currently matched value '%s' != '%s'>", seen, matchedValue)
		} else {
			if !ok {
				namedGroupValues[captureName] = matchedValue
			}
			matched += matchedValue
		}

	}
	return matched
}

func findCaptureNode(node *syntax.Regexp) []*syntax.Regexp {
	nodes := make([]*syntax.Regexp, 0)
	if node.Op == syntax.OpCapture {
		nodes = append(nodes, node)
	}
	if node.Sub == nil {
		return nodes
	}

	for _, subNode := range node.Sub {
		nodes = append(nodes, findCaptureNode(subNode)...)
	}
	return nodes
}

func warnNestedCaptureGroups(node *syntax.Regexp) {
	captureNodes := findCaptureNode(node)
	for _, capture := range captureNodes {
		nested := findCaptureNode(capture)
		if len(nested) > 1 {
			klog.Warningf("nested capture is not supported: '%s'", capture.String())
		}
	}
}

func (id RegexInlineDiff) validate(regex string) error {
	tree, err := syntax.Parse(regex, syntax.Perl)
	if err != nil {
		return fmt.Errorf("invalid regex passed to inline rgegex diff function: %w", err)
	}
	warnNestedCaptureGroups(tree)
	return nil
}

type PartV2 struct {
	Name       string         `json:"name"`
	Components []*ComponentV2 `json:"components"`
}

func (p *PartV2) getValidationIssues(matchedTemplates map[string]int) (map[string]ValidationIssue, int) {
	issues := make(map[string]ValidationIssue)
	count := 0
	for _, comp := range p.Components {
		compIssues, compCount := comp.getValidationIssues(matchedTemplates)
		if len(compIssues.CRs) > 0 {
			issues[comp.Name] = compIssues
		}
		count += compCount
	}
	return issues, count
}

type ComponentV2 struct {
	Name        string `json:"name"`
	OneOf       `json:"oneOf,omitempty"`
	NoneOf      `json:"noneOf,omitempty"`
	AllOf       `json:"allOf,omitempty"`
	AnyOf       `json:"anyOf,omitempty"`
	AnyOneOf    `json:"anyOneOf,omitempty"`
	AllOrNoneOf `json:"allOrNoneOf,omitempty"`
	parts       []ComponentV2Group
}

type ComponentV2Group interface {
	SetTemplates([]*ReferenceTemplateV2)
	GetTemplates() []*ReferenceTemplateV2
	UnmarshalJSON([]byte) (err error)
	getMissingCRs(map[string]int) (ValidationIssue, int)
}

type componentGroup struct {
	templates []*ReferenceTemplateV2
}

func (g *componentGroup) SetTemplates(t []*ReferenceTemplateV2) {
	g.templates = t
}

func (g *componentGroup) GetTemplates() []*ReferenceTemplateV2 {
	return g.templates
}

func getFieldNameFromStructTag(c *ComponentV2, s ComponentV2Group) string {
	// Because of embedding we can use the type as the field name to lookup the struct tags
	x := strings.Split(fmt.Sprintf("%T", s), ".")
	y := x[len(x)-1]
	field, _ := reflect.TypeOf(c).Elem().FieldByName(y)
	return strings.Split(field.Tag.Get("json"), ",")[0]
}

func componentV2GroupUnmarshalJSON(s ComponentV2Group, b []byte) (err error) {
	list := make([]*ReferenceTemplateV2, 0)
	err = json.Unmarshal(b, &list)
	s.SetTemplates(list)
	return err // nolint wrapcheck
}

const (
	MissingCRsMsg      = "Missing CRs"
	MatchedMoreThanOne = "Should only match one but matched"
)

type OneOf struct {
	componentGroup
}

func (g *OneOf) UnmarshalJSON(b []byte) (err error) {
	return componentV2GroupUnmarshalJSON(g, b)
}

func (g *OneOf) getMissingCRs(matchedTemplates map[string]int) (ValidationIssue, int) {
	matched := make([]string, 0)
	notMatched := make([]string, 0)
	for _, temp := range g.templates {
		if n, ok := matchedTemplates[temp.GetPath()]; !ok || (ok && n == 0) {
			notMatched = append(notMatched, temp.GetPath())
		} else {
			matched = append(matched, temp.GetPath())
		}
	}
	if len(matched) == 0 {
		return ValidationIssue{
			Msg: "One of the following is required",
			CRs: notMatched,
		}, 1
	}
	if len(matched) > 1 {
		return ValidationIssue{
			Msg: MatchedMoreThanOne,
			CRs: matched,
		}, 0
	}
	return ValidationIssue{}, 0
}

type NoneOf struct {
	componentGroup
}

func (g *NoneOf) UnmarshalJSON(b []byte) (err error) {
	return componentV2GroupUnmarshalJSON(g, b)
}

func (g *NoneOf) getMissingCRs(matchedTemplates map[string]int) (ValidationIssue, int) {
	matched := make([]string, 0)
	for _, temp := range g.templates {
		if n, ok := matchedTemplates[temp.GetPath()]; ok && n > 0 {
			matched = append(matched, temp.GetPath())
		}
	}
	if len(matched) > 0 {
		return ValidationIssue{
			Msg: "These should not have been matched",
			CRs: matched,
		}, 0
	}
	return ValidationIssue{}, 0

}

type AllOf struct {
	componentGroup
}

func (g *AllOf) UnmarshalJSON(b []byte) (err error) {
	return componentV2GroupUnmarshalJSON(g, b)
}

func (g *AllOf) getMissingCRs(matchedTemplates map[string]int) (ValidationIssue, int) {
	notMatched := make([]string, 0)
	for _, temp := range g.templates {
		if n, ok := matchedTemplates[temp.GetPath()]; !ok || (ok && n == 0) {
			notMatched = append(notMatched, temp.GetPath())
		}
	}
	if len(notMatched) > 0 {
		return ValidationIssue{
			Msg: MissingCRsMsg,
			CRs: notMatched,
		}, len(notMatched)
	}
	return ValidationIssue{}, 0
}

type AnyOf struct {
	componentGroup
}

func (g *AnyOf) UnmarshalJSON(b []byte) (err error) {
	return componentV2GroupUnmarshalJSON(g, b)
}

func (g *AnyOf) getMissingCRs(matchedTemplates map[string]int) (ValidationIssue, int) {
	return ValidationIssue{}, 0
}

type AnyOneOf struct {
	componentGroup
}

func (g *AnyOneOf) UnmarshalJSON(b []byte) (err error) {
	return componentV2GroupUnmarshalJSON(g, b)
}

func (g *AnyOneOf) getMissingCRs(matchedTemplates map[string]int) (ValidationIssue, int) {
	matched := make([]string, 0)
	for _, temp := range g.templates {
		if n, ok := matchedTemplates[temp.GetPath()]; ok && n > 0 {
			matched = append(matched, temp.GetPath())
		}
	}
	if len(matched) > 1 {
		return ValidationIssue{
			Msg: MatchedMoreThanOne,
			CRs: matched,
		}, 0
	}
	return ValidationIssue{}, 0
}

type AllOrNoneOf struct {
	componentGroup
}

func (g *AllOrNoneOf) UnmarshalJSON(b []byte) (err error) {
	return componentV2GroupUnmarshalJSON(g, b)
}

func (g *AllOrNoneOf) getMissingCRs(matchedTemplates map[string]int) (ValidationIssue, int) {
	matched := make([]string, 0)
	notMatched := make([]string, 0)
	for _, temp := range g.templates {
		if n, ok := matchedTemplates[temp.GetPath()]; !ok || (ok && n == 0) {
			notMatched = append(notMatched, temp.GetPath())
		} else {
			matched = append(matched, temp.GetPath())
		}
	}
	if len(matched) > 0 && len(notMatched) > 0 {
		return ValidationIssue{
			Msg: MissingCRsMsg,
			CRs: notMatched,
		}, len(notMatched)
	}
	return ValidationIssue{}, 0
}

func (comp *ComponentV2) validate(index int) error {
	if len(comp.OneOf.templates) > 0 {
		comp.parts = append(comp.parts, &comp.OneOf)
	}
	if len(comp.NoneOf.templates) > 0 {
		comp.parts = append(comp.parts, &comp.NoneOf)
	}
	if len(comp.AllOf.templates) > 0 {
		comp.parts = append(comp.parts, &comp.AllOf)
	}
	if len(comp.AnyOf.templates) > 0 {
		comp.parts = append(comp.parts, &comp.AnyOf)
	}
	if len(comp.AnyOneOf.templates) > 0 {
		comp.parts = append(comp.parts, &comp.AnyOneOf)
	}
	if len(comp.AllOrNoneOf.templates) > 0 {
		comp.parts = append(comp.parts, &comp.AllOrNoneOf)
	}

	if len(comp.parts) == 0 {
		return fmt.Errorf("component %s has no templates", comp.Name)
	}

	if len(comp.parts) > 1 {
		keys := make([]string, 0)
		for _, g := range comp.parts {
			keys = append(keys, getFieldNameFromStructTag(comp, g))
		}

		return fmt.Errorf("too many keys (%s) in index %d of component %s", strings.Join(keys, ","), index, comp.Name)
	}
	return nil
}

func (comp ComponentV2) getTemplates() []*ReferenceTemplateV2 {
	templates := make([]*ReferenceTemplateV2, 0)
	for _, g := range comp.parts {
		templates = append(templates, g.GetTemplates()...)
	}
	return templates
}

func (comp ComponentV2) getValidationIssues(matchedTemplates map[string]int) (ValidationIssue, int) {
	// Because of the validation in ComponentV2.validate we should ave one and only one
	return comp.parts[0].getMissingCRs(matchedTemplates)
}

func getReferenceV2(fsys fs.FS, referenceFileName string) (*ReferenceV2, error) {
	result := &ReferenceV2{}
	err := parseYaml(fsys, referenceFileName, &result, refConfNotExistsError, refConfigNotInFormat)
	if err != nil {
		return result, err
	}
	if result.FieldsToOmit == nil {
		result.FieldsToOmit = &FieldsToOmitV2{}
	}
	err = result.FieldsToOmit.process()
	if err != nil {
		return result, err
	}
	result.normalisedVersion = ReferenceVersionV2

	err = result.validate()
	if err != nil {
		return result, err
	}
	return result, nil
}

func ParseV2Templates(ref *ReferenceV2, fsys fs.FS) ([]ReferenceTemplate, error) {
	var errs []error
	var result []ReferenceTemplate
	functionTemplates := ref.TemplateFunctionFiles
	for _, temp := range ref.getTemplates() {
		result = append(result, temp)
		parsedTemp, err := template.New(path.Base(temp.Path)).Funcs(FuncMap()).ParseFS(fsys, temp.Path)
		if err != nil {
			errs = append(errs, fmt.Errorf(templatesCantBeParsed, temp.Path, err))
			continue
		}
		if len(functionTemplates) > 0 {
			parsedTemp, err = parsedTemp.ParseFS(fsys, functionTemplates...)
			if err != nil {
				errs = append(errs, fmt.Errorf(templatesFunctionsCantBeParsed, err))
				continue
			}
		}
		temp.Template = parsedTemp
		temp.ReferenceTemplateV1.Config = temp.Config.ReferenceTemplateConfigV1
		temp.metadata, err = temp.Exec(map[string]any{}) // Extract Metadata
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse template %s with empty data: %w", temp.Path, err))
		}
		err = temp.validateConfigPerField()
		if err != nil {
			errs = append(errs, err)
		}
		err = temp.ValidateFieldsToOmit(ref.FieldsToOmit)
		if err != nil {
			errs = append(errs, err)
		}
		if temp.metadata != nil && temp.metadata.GetKind() == "" {
			errs = append(errs, fmt.Errorf("template missing kind: %s", temp.Path))
		}
	}
	return result, errors.Join(errs...) // nolint:wrapcheck
}
