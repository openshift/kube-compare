// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type Reference struct {
	Parts                 []Part       `json:"parts"`
	TemplateFunctionFiles []string     `json:"templateFunctionFiles,omitempty"`
	FieldsToOmit          FieldsToOmit `json:"fieldsToOmit,omitempty"`
}

type Part struct {
	Name       string      `json:"name"`
	Components []Component `json:"components"`
}

type ComponentType string

const (
	Required ComponentType = "Required"
	Optional ComponentType = "Optional"
)

const (
	fieldsToOmitBuiltInOverwritten = `fieldsToOmit.Map contains the key "%s", this will be overwritten with default values`
	fieldsToOmitDefaultNotFound    = `fieldsToOmit's defaultOmitRef "%s" not found in items`
	fieldsToOmitRefsNotFound       = `fieldsToOmitRefs entry "%s" not found it fieldsToOmit Items`
)

type FieldsToOmit struct {
	DefaultOmitRef string                     `json:"defaultOmitRef,omitempty"`
	Items          map[string][]*ManifestPath `json:"items,omitempty"`
}

// Setup FieldsToOmit to be used by setting defaults
// and processing the item strings into paths
func (toOmit *FieldsToOmit) process() error {
	if toOmit.Items == nil {
		toOmit.Items = make(map[string][]*ManifestPath)
	}

	if _, ok := toOmit.Items[builtInPathsKey]; ok {
		klog.Warningf(fieldsToOmitBuiltInOverwritten, builtInPathsKey)
	}

	toOmit.Items[builtInPathsKey] = builtInPaths

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

type Component struct {
	Name              string               `json:"name"`
	Type              ComponentType        `json:"type,omitempty"`
	RequiredTemplates []*ReferenceTemplate `json:"requiredTemplates,omitempty"`
	OptionalTemplates []*ReferenceTemplate `json:"optionalTemplates,omitempty"`
}
type ReferenceTemplateConfig struct {
	AllowMerge       bool     `json:"ignore-unspecified-fields,omitempty"`
	FieldsToOmitRefs []string `json:"fieldsToOmitRefs,omitempty"`
}

type ReferenceTemplate struct {
	*template.Template
	Path     string                  `json:"path"`
	Config   ReferenceTemplateConfig `json:"config,omitempty"`
	metadata *unstructured.Unstructured
}

func (rf ReferenceTemplate) FieldsToOmit(fieldsToOmit FieldsToOmit) []*ManifestPath {
	result := make([]*ManifestPath, 0)
	if len(rf.Config.FieldsToOmitRefs) == 0 {
		return fieldsToOmit.Items[fieldsToOmit.DefaultOmitRef]
	}

	for _, feildsRef := range rf.Config.FieldsToOmitRefs {
		result = append(result, fieldsToOmit.Items[feildsRef]...)
	}
	return result
}

func (rf ReferenceTemplate) ValidateFieldsToOmit(fieldsToOmit FieldsToOmit) error {
	errs := make([]error, 0)
	for _, feildsRef := range rf.Config.FieldsToOmitRefs {
		if _, ok := fieldsToOmit.Items[feildsRef]; !ok {
			errs = append(errs, fmt.Errorf(fieldsToOmitRefsNotFound, feildsRef))
		}
	}
	return errors.Join(errs...)
}

const noValue = "<no value>"

func (rf ReferenceTemplate) Exec(params map[string]any) (*unstructured.Unstructured, error) {
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
			rf.Name(), err, string(content),
		)
	}
	return &unstructured.Unstructured{Object: data}, nil
}

func (rf ReferenceTemplate) Name() string {
	return rf.Path
}

func (r *Reference) GetTemplates() []*ReferenceTemplate {
	var templates []*ReferenceTemplate
	for _, part := range r.Parts {
		for _, comp := range part.Components {
			templates = append(templates, comp.RequiredTemplates...)
			templates = append(templates, comp.OptionalTemplates...)
		}
	}
	return templates
}

func (c *Component) getMissingCRs(matchedTemplates map[string]int) []string {
	var crs []string
	for _, temp := range c.RequiredTemplates {
		if wasMatched, ok := matchedTemplates[temp.Path]; !ok || wasMatched == 0 {
			crs = append(crs, temp.Path)
		}
	}
	return crs
}

func (p *Part) getMissingCRs(matchedTemplates map[string]int) (map[string][]string, int) {
	crs := make(map[string][]string)
	count := 0
	for _, comp := range p.Components {
		compCRs := comp.getMissingCRs(matchedTemplates)
		if (len(compCRs) > 0) && (comp.Type == Required || ((comp.Type == Optional) && len(compCRs) != len(comp.RequiredTemplates))) {
			crs[comp.Name] = compCRs
			count += len(compCRs)
		}
	}
	return crs, count
}

func (r *Reference) getMissingCRs(matchedTemplates map[string]int) (map[string]map[string][]string, int) {
	crs := make(map[string]map[string][]string)
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

const builtInPathsKey = "cluster-compare-built-in"

var builtInPaths = []*ManifestPath{
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

type ManifestPath struct {
	PathToKey string `json:"pathToKey"`
	IsPrefix  bool   `json:"isPrefix,omitempty"`
	parts     []string
}

func (p *ManifestPath) Process() error {
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

const (
	refConfNotExistsError          = "Reference config file not found. error: %w"
	refConfigNotInFormat           = "Reference config isn't in correct format. error: %w"
	userConfNotExistsError         = "User Config File not found. error: %w"
	userConfigNotInFormat          = "User config file isn't in correct format. error: %w"
	templatesCantBeParsed          = "an error occurred while parsing template: %s specified in the config. error: %w"
	templatesFunctionsCantBeParsed = "an error occurred while parsing the template function files specified in the config. error: %w"
)

func GetReference(fsys fs.FS, referenceFileName string) (Reference, error) {
	result := Reference{}
	err := parseYaml(fsys, referenceFileName, &result, refConfNotExistsError, refConfigNotInFormat)
	if err != nil {
		return result, err
	}
	err = result.FieldsToOmit.process()
	if err != nil {
		return result, err
	}
	return result, nil
}

func parseYaml[T any](fsys fs.FS, filePath string, structType *T, fileNotFoundError, parsingError string) error {
	file, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return fmt.Errorf(fileNotFoundError, err)
	}
	err = yaml.UnmarshalStrict(file, structType)
	if err != nil {
		return fmt.Errorf(parsingError, err)
	}
	return nil
}

func ParseTemplates(templateReference []*ReferenceTemplate, functionTemplates []string, fsys fs.FS, ref *Reference) ([]*ReferenceTemplate, error) {
	var errs []error
	for _, temp := range templateReference {
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
	return templateReference, errors.Join(errs...) // nolint:wrapcheck
}

type UserConfig struct {
	CorrelationSettings CorrelationSettings `json:"correlationSettings"`
}

type CorrelationSettings struct {
	ManualCorrelation ManualCorrelation `json:"manualCorrelation"`
}

type ManualCorrelation struct {
	CorrelationPairs map[string]string `json:"correlationPairs"`
}

func parseDiffConfig(filePath string) (UserConfig, error) {
	result := UserConfig{}
	confPath, err := filepath.Abs(filePath)
	if err != nil {
		return result, fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
	}
	err = parseYaml(os.DirFS("/"), confPath[1:], &result, userConfNotExistsError, userConfigNotInFormat)
	return result, err
}
