package compare

import (
	"bytes"
	"fmt"
	"os"
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
	refConfigNotInFormat           = "Reference config file isn't in correct format. error: "
	userConfNotExistsError         = "User Config File not found. error: "
	userConfigNotInFormat          = "User config file isn't in correct format. error: "
	templatesCantBeParsed          = "An error occurred while parsing the templates specified in the config. error: "
	templatesFunctionsCantBeParsed = "An error occurred while parsing the template function files specified in the config. error: "
)

func getReference(ReffDir string) (Reference, error) {
	result := Reference{}
	err := parseYaml(filepath.Join(ReffDir, ReferenceFileName), &result, refConfNotExistsError, refConfigNotInFormat)
	if err != nil {
		return result, err
	}
	if len(result.FieldsToOmit) == 0 {
		result.FieldsToOmit = defaultFieldsToOmit
	}
	return result, nil
}

func parseYaml[T any](filePath string, structType *T, fileNotFoundError string, parsingError string) error {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%s%v", fileNotFoundError, err)
	}
	err = yaml.UnmarshalStrict(file, structType)
	if err != nil {
		return fmt.Errorf("%s%v", parsingError, err)
	}
	return nil
}

func getTemplates(templatePaths []string, functionTemplates []string) ([]*template.Template, error) {
	var templates []*template.Template
	ts, err := template.New("base").Funcs(FuncMap()).ParseFiles(templatePaths...)
	if err != nil {
		return []*template.Template{}, fmt.Errorf("%s%v", templatesCantBeParsed, err)
	}
	for _, temp := range ts.Templates() {
		if temp.Name() == temp.ParseName {
			templates = append(templates, temp)
		}
	}
	if len(functionTemplates) == 0 {
		return templates, nil
	}
	ts, err = ts.ParseFiles(functionTemplates...)
	if err != nil {
		return templates, fmt.Errorf("%s%v", templatesFunctionsCantBeParsed, err)
	}
	return templates, nil
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
	err := parseYaml(filePath, &result, userConfNotExistsError, userConfigNotInFormat)
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
