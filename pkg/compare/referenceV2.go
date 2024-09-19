// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"strings"
	"text/template"
)

const ReferenceVersionV2 string = "v2"

type ReferenceV2 struct {
	Version           string `json:"apiVersion,omitempty"`
	normalisedVersion string

	Parts                 []*PartV2       `json:"parts"`
	TemplateFunctionFiles []string        `json:"templateFunctionFiles,omitempty"`
	FieldsToOmit          *FieldsToOmitV1 `json:"fieldsToOmit,omitempty"`
}

func (r *ReferenceV2) GetAPIVersion() string {
	return r.normalisedVersion
}
func (r *ReferenceV2) getTemplates() []*ReferenceTemplateV1 {
	var templates []*ReferenceTemplateV1
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
	SetTemplates([]*ReferenceTemplateV1)
	GetTemplates() []*ReferenceTemplateV1
	UnmarshalJSON([]byte) (err error)
	getMissingCRs(map[string]int) (ValidationIssue, int)
}

type componentGroup struct {
	templates []*ReferenceTemplateV1
}

func (g *componentGroup) SetTemplates(t []*ReferenceTemplateV1) {
	g.templates = t
}

func (g *componentGroup) GetTemplates() []*ReferenceTemplateV1 {
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
	list := make([]*ReferenceTemplateV1, 0)
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

func (comp ComponentV2) getTemplates() []*ReferenceTemplateV1 {
	templates := make([]*ReferenceTemplateV1, 0)
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
		result.FieldsToOmit = &FieldsToOmitV1{}
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
