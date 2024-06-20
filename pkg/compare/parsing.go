// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

type Reference struct {
	Parts                 []Part     `json:"parts"`
	TemplateFunctionFiles []string   `json:"templateFunctionFiles,omitempty"`
	FieldsToOmit          [][]string `json:"fieldsToOmit,omitempty"`
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

type Component struct {
	Name              string               `json:"name"`
	Type              ComponentType        `json:"type,omitempty"`
	RequiredTemplates []*ReferenceTemplate `json:"requiredTemplates,omitempty"`
	OptionalTemplates []*ReferenceTemplate `json:"optionalTemplates,omitempty"`
}
type ReferenceTemplateConfig struct {
	AllowMerge bool `json:"ignore-unspecified-fields,omitempty"`
}

type ReferenceTemplate struct {
	*template.Template
	Path   string                  `json:"path"`
	Config ReferenceTemplateConfig `json:"config,omitempty"`
}

func (rf ReferenceTemplate) Exec(params map[string]any) (*unstructured.Unstructured, error) {
	return executeYAMLTemplate(rf.Template, params)
}

func (r *Reference) getTemplates() []*ReferenceTemplate {
	var templates []*ReferenceTemplate
	for _, part := range r.Parts {
		for _, comp := range part.Components {
			templates = append(templates, comp.RequiredTemplates...)
			templates = append(templates, comp.OptionalTemplates...)
		}
	}
	return templates
}

func (c *Component) getMissingCRs(matchedTemplates map[string]bool) []string {
	var crs []string
	for _, temp := range c.RequiredTemplates {
		if wasMatched := matchedTemplates[temp.Path]; !wasMatched {
			crs = append(crs, temp.Path)
		}
	}
	return crs
}

func (p *Part) getMissingCRs(matchedTemplates map[string]bool) (map[string][]string, int) {
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

func (r *Reference) getMissingCRs(matchedTemplates map[string]bool) (map[string]map[string][]string, int) {
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

var defaultFieldsToOmit = [][]string{{"metadata", "uid"},
	{"metadata", "resourceVersion"},
	{"metadata", "generation"},
	{"metadata", "generateName"},
	{"metadata", "creationTimestamp"},
	{"metadata", "finalizers"},
	{"kubectl.kubernetes.io/last-applied-configuration"},
	{"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
	{"status"},
}

const (
	refConfNotExistsError          = "Reference config file not found. error: "
	refConfigNotInFormat           = "Reference config isn't in correct format. error: "
	userConfNotExistsError         = "User Config File not found. error: "
	userConfigNotInFormat          = "User config file isn't in correct format. error: "
	templatesCantBeParsed          = "an error occurred while parsing template: %s specified in the config. error: %v"
	templatesFunctionsCantBeParsed = "an error occurred while parsing the template function files specified in the config. error: %v"
)

func getReference(fsys fs.FS) (Reference, error) {
	result := Reference{}
	err := parseYaml(fsys, ReferenceFileName, &result, refConfNotExistsError, refConfigNotInFormat)
	if err != nil {
		return result, err
	}
	if len(result.FieldsToOmit) == 0 {
		result.FieldsToOmit = defaultFieldsToOmit
	}
	return result, nil
}

func parseYaml[T any](fsys fs.FS, filePath string, structType *T, fileNotFoundError, parsingError string) error {
	file, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return fmt.Errorf("%s%w", fileNotFoundError, err)
	}
	err = yaml.UnmarshalStrict(file, structType)
	if err != nil {
		return fmt.Errorf("%s%w", parsingError, err)
	}
	return nil
}

func parseTemplates(templateReference []*ReferenceTemplate, functionTemplates []string, fsys fs.FS) ([]*ReferenceTemplate, error) {
	var errs []error
	for _, temp := range templateReference {
		parsedTemp, err := template.New(path.Base(temp.Path)).Funcs(FuncMap()).ParseFS(fsys, temp.Path)
		if err != nil {
			errs = append(errs, fmt.Errorf(templatesCantBeParsed, temp.Path, err))
			continue
		}
		// recreate template with new name that includes path from reference root:
		parsedTemp, _ = parsedTemp.New(temp.Path).AddParseTree(temp.Path, parsedTemp.Lookup(path.Base(temp.Path)).Tree)
		if len(functionTemplates) > 0 {
			parsedTemp, err = parsedTemp.ParseFS(fsys, functionTemplates...)
			if err != nil {
				errs = append(errs, fmt.Errorf(templatesFunctionsCantBeParsed, err))
				continue
			}
		}
		temp.Template = parsedTemp
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

const noValue = "<no value>"

func executeYAMLTemplate(temp *template.Template, params map[string]any) (*unstructured.Unstructured, error) {
	var buf bytes.Buffer
	err := temp.Execute(&buf, params)
	if err != nil {
		return nil, fmt.Errorf("failed to constuct template: %w", err)
	}
	data := make(map[string]any)
	content := buf.Bytes()
	err = yaml.Unmarshal(bytes.ReplaceAll(content, []byte(noValue), []byte("")), &data)
	if err != nil {
		return nil, fmt.Errorf("template: %s isn't a yaml file after injection. yaml unmarshal error: %w. The Template After Execution: %s", temp.Name(), err, string(content))
	}
	return &unstructured.Unstructured{Object: data}, nil
}
func extractMetadata(t *ReferenceTemplate) (*unstructured.Unstructured, error) {
	yamlTemplate, err := t.Exec(map[string]any{})
	return yamlTemplate, err
}
