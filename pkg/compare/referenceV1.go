// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"
	"text/template/parse"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

const ReferenceVersionV1 string = "v1"

type ReferenceV1 struct {
	Version           string `json:"apiVersion,omitempty"`
	normalisedVersion string

	Parts                 []PartV1        `json:"parts"`
	TemplateFunctionFiles []string        `json:"templateFunctionFiles,omitempty"`
	FieldsToOmit          *FieldsToOmitV1 `json:"fieldsToOmit,omitempty"`
}

type PartV1 struct {
	Name       string        `json:"name"`
	Components []ComponentV1 `json:"components"`
}

type ComponentTypeV1 string

const (
	Required ComponentTypeV1 = "Required"
	Optional ComponentTypeV1 = "Optional"
)

type ComponentV1 struct {
	Name              string                 `json:"name"`
	Type              ComponentTypeV1        `json:"type,omitempty"`
	RequiredTemplates []*ReferenceTemplateV1 `json:"requiredTemplates,omitempty"`
	OptionalTemplates []*ReferenceTemplateV1 `json:"optionalTemplates,omitempty"`
}

func (r *ReferenceV1) GetAPIVersion() string {
	return r.normalisedVersion
}
func (r *ReferenceV1) getTemplates() []*ReferenceTemplateV1 {
	var templates []*ReferenceTemplateV1
	for _, part := range r.Parts {
		for _, comp := range part.Components {
			templates = append(templates, comp.RequiredTemplates...)
			templates = append(templates, comp.OptionalTemplates...)
		}
	}
	return templates
}

func (r *ReferenceV1) GetTemplates() []ReferenceTemplate {
	var templates []ReferenceTemplate
	// Repackage getTemplates into []ReferenceTemplate
	// because go's  (or LSPs) type checking isn't quite good enough to accept it
	for _, t := range r.getTemplates() {
		templates = append(templates, t)
	}
	return templates
}

func (r *ReferenceV1) GetFieldsToOmit() FieldsToOmit {
	return r.FieldsToOmit
}

func (r *ReferenceV1) GetTemplateFunctionFiles() []string {
	return r.TemplateFunctionFiles
}

func (c *ComponentV1) getMissingCRs(matchedTemplates map[string]int) ValidationIssue {
	var crs []string
	for _, temp := range c.RequiredTemplates {
		if wasMatched, ok := matchedTemplates[temp.Path]; !ok || wasMatched == 0 {
			crs = append(crs, temp.Path)
		}
	}
	return ValidationIssue{Msg: MissingCRsMsg, CRs: crs}
}

func (p *PartV1) getMissingCRs(matchedTemplates map[string]int) (map[string]ValidationIssue, int) {
	crs := make(map[string]ValidationIssue)
	count := 0
	for _, comp := range p.Components {
		compCRs := comp.getMissingCRs(matchedTemplates)
		missing := compCRs.CRs
		if (len(missing) > 0) && (comp.Type == Required || ((comp.Type == Optional) && len(missing) != len(comp.RequiredTemplates))) {
			crs[comp.Name] = compCRs
			count += len(missing)
		}
	}
	return crs, count
}

func (r *ReferenceV1) GetValidationIssues(matchedTemplates map[string]int) (map[string]map[string]ValidationIssue, int) {
	crs := make(map[string]map[string]ValidationIssue)
	count := 0
	for _, part := range r.Parts {
		crsInPart, countInPart := part.getMissingCRs(matchedTemplates)
		if countInPart > 0 {
			crs[part.Name] = crsInPart
			count += countInPart
		}
	}
	return crs, count
}

func getReferenceV1(fsys fs.FS, referenceFileName string) (*ReferenceV1, error) {
	result := &ReferenceV1{}
	err := parseYaml(fsys, referenceFileName, &result, refConfNotExistsError, refConfigNotInFormat)
	if err != nil {
		return result, err
	}
	if result.FieldsToOmit == nil {
		result.FieldsToOmit = &FieldsToOmitV1{}
	}
	err = result.FieldsToOmit.process()
	if err != nil {
		return result, err
	}
	result.normalisedVersion = ReferenceVersionV1
	return result, nil
}

type FieldsToOmitV1 struct {
	DefaultOmitRef string                       `json:"defaultOmitRef,omitempty"`
	Items          map[string][]*ManifestPathV1 `json:"items,omitempty"`
}

func (toOmit *FieldsToOmitV1) GetDefault() string {
	return toOmit.DefaultOmitRef
}

func (toOmit *FieldsToOmitV1) GetItems() map[string][]*ManifestPathV1 {
	return toOmit.Items
}

const (
	fieldsToOmitDefaultNotFound    = `fieldsToOmit's defaultOmitRef "%s" not found in items`
	fieldsToOmitBuiltInOverwritten = `fieldsToOmit.Map contains the key "%s", this will be overwritten with default values`
)

// Setup FieldsToOmit to be used by setting defaults
// and processing the item strings into paths
func (toOmit *FieldsToOmitV1) process() error {
	if toOmit.Items == nil {
		toOmit.Items = make(map[string][]*ManifestPathV1)
	}

	if _, ok := toOmit.Items[builtInPathsKey]; ok {
		klog.Warningf(fieldsToOmitBuiltInOverwritten, builtInPathsKey)
	}

	toOmit.Items[builtInPathsKey] = builtInPathsV1

	if toOmit.DefaultOmitRef == "" {
		toOmit.DefaultOmitRef = builtInPathsKey
	}

	if _, ok := toOmit.Items[toOmit.DefaultOmitRef]; !ok {
		return fmt.Errorf(fieldsToOmitDefaultNotFound, toOmit.DefaultOmitRef)
	}
	errs := make([]error, 0)
	for _, pathsArray := range toOmit.Items {
		for _, path := range pathsArray {
			err := path.Process()
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

type ReferenceTemplateConfigV1 struct {
	AllowMerge       bool     `json:"ignore-unspecified-fields,omitempty"`
	FieldsToOmitRefs []string `json:"fieldsToOmitRefs,omitempty"`
}

func (config ReferenceTemplateConfigV1) GetAllowMerge() bool {
	return config.AllowMerge
}

func (config ReferenceTemplateConfigV1) GetFieldsToOmitRefs() []string {
	return config.FieldsToOmitRefs
}

type ReferenceTemplateV1 struct {
	*template.Template `json:"-"`
	Path               string                    `json:"path"`
	Config             ReferenceTemplateConfigV1 `json:"config,omitempty"`
	metadata           *unstructured.Unstructured
}

func (rf ReferenceTemplateV1) GetFieldsToOmit(fieldsToOmit FieldsToOmit) []*ManifestPathV1 {
	result := make([]*ManifestPathV1, 0)
	// ValidateFieldsToOmit should check the ok

	items := fieldsToOmit.GetItems()
	if len(rf.Config.FieldsToOmitRefs) == 0 {
		result = append(result, items[fieldsToOmit.GetDefault()]...)
		return result
	}

	for _, feildsRef := range rf.Config.FieldsToOmitRefs {
		result = append(result, items[feildsRef]...)
	}
	return result
}

const (
	fieldsToOmitRefsNotFound = `fieldsToOmitRefs entry "%s" not found it fieldsToOmit Items`
)

func (rf ReferenceTemplateV1) ValidateFieldsToOmit(fieldsToOmit FieldsToOmit) error {
	errs := make([]error, 0)
	items := fieldsToOmit.GetItems()
	for _, feildsRef := range rf.Config.FieldsToOmitRefs {
		if _, ok := items[feildsRef]; !ok {
			errs = append(errs, fmt.Errorf(fieldsToOmitRefsNotFound, feildsRef))
		}
	}
	return errors.Join(errs...)
}

const noValue = "<no value>"

func (rf ReferenceTemplateV1) Exec(params map[string]any) (*unstructured.Unstructured, error) {
	var buf bytes.Buffer
	err := rf.Template.Execute(&buf, params)
	if err != nil {
		return nil, fmt.Errorf("failed to constuct template: %w", err)
	}
	data := make(map[string]any)
	content := buf.Bytes()
	err = yaml.Unmarshal(bytes.ReplaceAll(content, []byte(noValue), []byte("")), &data)
	if err != nil {
		return nil, fmt.Errorf(
			"template: %s isn't a yaml file after injection. yaml unmarshal error: %w. The Template After Execution: %s",
			rf.GetIdentifier(), err, string(content),
		)
	}
	return &unstructured.Unstructured{Object: data}, nil
}

func (rf ReferenceTemplateV1) GetPath() string {
	return rf.Path
}

func (rf ReferenceTemplateV1) GetIdentifier() string {
	return rf.GetPath()
}

func (rf ReferenceTemplateV1) GetMetadata() *unstructured.Unstructured {
	return rf.metadata
}

func (rf ReferenceTemplateV1) GetConfig() TemplateConfig {
	return rf.Config
}

func (rf ReferenceTemplateV1) GetTemplateTree() *parse.Tree {
	return rf.Tree
}

const builtInPathsKey = "cluster-compare-built-in"

var builtInPathsV1 = []*ManifestPathV1{
	{PathToKey: "metadata.resourceVersion"},
	{PathToKey: "metadata.generation"},
	{PathToKey: "metadata.uid"},
	{PathToKey: "metadata.generateName"},
	{PathToKey: "metadata.creationTimestamp"},
	{PathToKey: "metadata.finalizers"},
	{PathToKey: `"kubectl.kubernetes.io/last-applied-configuration"`},
	{PathToKey: `metadata.annotations."kubectl.kubernetes.io/last-applied-configuration"`},
	{PathToKey: "status"},
}

type ManifestPathV1 struct {
	PathToKey string `json:"pathToKey"`
	IsPrefix  bool   `json:"isPrefix,omitempty"`
	parts     []string
}

func (p *ManifestPathV1) Process() error {
	if len(p.parts) > 0 {
		return nil
	}

	pathToKey, _ := strings.CutPrefix(p.PathToKey, ".")
	r := csv.NewReader(strings.NewReader(pathToKey))
	r.Comma = '.'
	fields, err := r.Read()
	if err != nil {
		return fmt.Errorf("failed to parse path: %w", err)
	}
	p.parts = fields
	return nil
}

func ParseV1Templates(ref *ReferenceV1, fsys fs.FS) ([]ReferenceTemplate, error) {
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
		temp.metadata, err = temp.Exec(map[string]any{}) // Extract Metadata
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse template %s with empty data: %w", temp.Path, err))
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
