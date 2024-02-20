package compare

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"text/template"
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
	Optional               = "Optional"
)

type Component struct {
	Name              string        `json:"name"`
	Type              ComponentType `json:"type,omitempty"`
	RequiredTemplates []string      `json:"requiredTemplates,omitempty"`
	OptionalTemplates []string      `json:"optionalTemplates,omitempty"`
}

func (r *Reference) getTemplates() []string {
	var templates []string
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
		if wasMatched, _ := matchedTemplates[temp]; !wasMatched {
			crs = append(crs, temp)
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

func parseYaml[T any](fsys fs.FS, filePath string, structType *T, fileNotFoundError string, parsingError string) error {
	file, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return fmt.Errorf("%s%v", fileNotFoundError, err)
	}
	err = yaml.UnmarshalStrict(file, structType)
	if err != nil {
		return fmt.Errorf("%s%v", parsingError, err)
	}
	return nil
}

func parseTemplates(templatePaths []string, functionTemplates []string, fsys fs.FS) ([]*template.Template, error) {
	var templates []*template.Template
	var errs []error
	for _, temp := range templatePaths {
		parsedTemp, err := template.New(path.Base(temp)).Funcs(FuncMap()).ParseFS(fsys, temp)
		if err != nil {
			errs = append(errs, fmt.Errorf(templatesCantBeParsed, temp, err))
			continue
		}
		// recreate template with new name that includes path from reference root:
		parsedTemp, _ = template.New(temp).Funcs(FuncMap()).AddParseTree(temp, parsedTemp.Tree)
		if len(functionTemplates) > 0 {
			parsedTemp, err = parsedTemp.ParseFS(fsys, functionTemplates...)
			if err != nil {
				errs = append(errs, fmt.Errorf(templatesFunctionsCantBeParsed, err))
				continue
			}
		}
		templates = append(templates, parsedTemp)
	}
	return templates, errors.Join(errs...)
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
	err = parseYaml(os.DirFS("/"), confPath[1:], &result, userConfNotExistsError, userConfigNotInFormat)
	return result, err
}

const noValue = "<no value>"

func executeYAMLTemplate(temp *template.Template, params map[string]any) (*unstructured.Unstructured, error) {
	var buf bytes.Buffer
	err := temp.Execute(&buf, params)
	if err != nil {
		return nil, err
	}
	data := make(map[string]any)
	err = yaml.Unmarshal(bytes.ReplaceAll(buf.Bytes(), []byte(noValue), []byte("")), &data)
	if err != nil {
		return nil, fmt.Errorf("template: %s isnt an yaml file after injection. yaml unmarshal error: %v. The Template After Execution: %s", temp.Name(), err, buf.String())
	}
	return &unstructured.Unstructured{Object: data}, err
}

func extractMetadata(t *template.Template) (*unstructured.Unstructured, error) {
	yamlTemplate, err := executeYAMLTemplate(t, map[string]any{})
	return yamlTemplate, err
}
