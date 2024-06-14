// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
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
		if wasMatched := matchedTemplates[temp]; !wasMatched {
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

type ReferenceTemplate struct {
	*template.Template
	allowMerge bool
	config     map[string]string
}

func (rf ReferenceTemplate) Exec(params map[string]any) (*unstructured.Unstructured, error) {
	return executeYAMLTemplate(rf.Template, params)
}

func NewReferenceTemplate(temp *template.Template) (*ReferenceTemplate, error) {
	rf := &ReferenceTemplate{Template: temp, config: make(map[string]string), allowMerge: true}

	config, err := extractCommentMap(temp)
	if err != nil {
		return rf, err
	}

	rf.config = config
	v, ok := config["allow-undefined-extras"]
	if ok {
		var b bool
		b, err = strconv.ParseBool(v)
		if err != nil {
			err = fmt.Errorf("failed to parse allow-undefined-extras value to bool: %w", err)
		} else {
			rf.allowMerge = b
		}
	}

	return rf, err
}

func parseTemplates(templatePaths, functionTemplates []string, fsys fs.FS, o *Options) ([]*ReferenceTemplate, error) {
	var templates []*ReferenceTemplate
	var errs []error
	for _, temp := range templatePaths {
		parsedTemp, err := template.New(path.Base(temp)).Funcs(FuncMap(o)).ParseFS(fsys, temp)
		if err != nil {
			errs = append(errs, fmt.Errorf(templatesCantBeParsed, temp, err))
			continue
		}
		// recreate template with new name that includes path from reference root:
		parsedTemp, _ = template.New(temp).Funcs(FuncMap(o)).AddParseTree(temp, parsedTemp.Tree)
		if len(functionTemplates) > 0 {
			parsedTemp, err = parsedTemp.ParseFS(fsys, functionTemplates...)
			if err != nil {
				errs = append(errs, fmt.Errorf(templatesFunctionsCantBeParsed, err))
				continue
			}
		}
		tf, err := NewReferenceTemplate(parsedTemp)
		if err != nil {
			errs = append(errs, err)
		}
		templates = append(templates, tf)
	}
	return templates, errors.Join(errs...) // nolint:wrapcheck
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

func executeYAMLTemplatRaw(temp *template.Template, params map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	err := temp.Execute(&buf, params)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to constuct template: %w", err)
	}
	return buf.Bytes(), nil
}
func executeYAMLTemplate(temp *template.Template, params map[string]any) (*unstructured.Unstructured, error) {
	content, err := executeYAMLTemplatRaw(temp, params)
	if err != nil {
		return nil, err
	}
	data := make(map[string]any)
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

func extractCommentMap(temp *template.Template) (map[string]string, error) {
	config := make(map[string]string)
	raw, _ := executeYAMLTemplatRaw(temp, map[string]any{})
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	errs := make([]error, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {
			continue
		}
		comment := strings.Trim(line, "# ")
		mapStr, found := strings.CutPrefix(comment, "cluster-compare:")
		if !found {
			continue
		}
		mapStr = strings.TrimSpace(mapStr)
		for _, kv := range strings.Split(mapStr, ";") {
			pair := strings.Split(kv, "=")
			switch len(pair) {
			case 2:
				config[pair[0]] = pair[1]
			default:
				err := fmt.Errorf("failed to parse template comment config: invalid format in %s expecting the form key=value, got %s", line, kv)
				klog.Error(err)
				errs = append(errs, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		klog.Errorf("failed to read metadata: %s", err)
	}
	return config, errors.Join(errs...)
}
