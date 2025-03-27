package convert

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/openshift/kube-compare/pkg/compare"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const helpersFileName = "_helpers.tpl"
const valuesFileName = "values.yaml"
const helmTemplatesDir = "templates"

func NewCmd() *cobra.Command {
	options := Options{}
	cmd := &cobra.Command{
		Use:   "helm-convert -r <REFERENCE_PATH> -n <CHART_DIRECTORY> [-d <EXISTING_CRS_DIR>] [-v <PREVIOUS_VALUES_PATH>] [--description <DESCRIPTION>] [--helm-version <VERSION>]",
		Short: "Convert kube-compare reference configs into a Helm chart.",
		Long: `The 'helm-convert' command generates a Helm chart from kube-compare reference configurations and creates a values.yaml file based on the values used in the templates included in the reference. 
You need to provide the path to the reference YAML file using the -r flag and the directory where the Helm chart should be created using the -n flag. 
Optionally, you can specify a directory containing existing custom resources (CRs) with the -d flag to auto-extract default values, and use the -v flag to provide a previous values.yaml file for updating the Helm chart. 
The resulting Helm chart will include templates for each reference and will use the values.yaml file to define the variables needed to create CRs. 
The tool helps automate the creation of values.yaml and supports default values extraction from existing CRs. For detailed usage and examples, refer to the documentation.`,

		RunE: func(cmd *cobra.Command, args []string) error {
			if options.refPath == "" {
				return fmt.Errorf("path to reference config file is required, pass by -r/--reference")
			}
			return convertToHelm(&options)
		},
	}
	cmd.Flags().StringVarP(&options.refPath, "reference", "r", "", "Path to reference config file.")
	cmd.Flags().StringVarP(&options.outputDir, "helm-name", "n", "Chart", "Path to save the helm chart")
	cmd.Flags().StringVarP(&options.defaultPath, "defaults", "d", "", "Path to directory with the CRs that the tool will extract default values from")
	cmd.Flags().StringVarP(&options.valuesPath, "values", "v", "", "Path to existing values.yaml file")
	cmd.Flags().StringVar(&options.chartDescription, "description", "This Helm Chart was generated from a kube-compare reference", "Description for generated Helm Chart")
	cmd.Flags().StringVar(&options.chartVersion, "helm-version", "1", "Version of generated Helm Chart")
	return cmd
}

type Options struct {
	refPath          string
	outputDir        string
	defaultPath      string
	valuesPath       string
	chartDescription string
	chartVersion     string
}

func convertToHelm(o *Options) error {
	helmTemplates := make(map[string]string)
	helmValues := make(map[string]any)
	var preValues map[string]any
	crsWithDefaults := make(map[string]map[string]interface{})

	compareOptions := compare.Options{ReferenceConfig: o.refPath, TmpDir: ""}
	cfs, err := compareOptions.GetRefFS()
	if err != nil {
		return fmt.Errorf("failed to get filesystem of cluster-compare reference %w", err)
	}

	templates, helperFuncs, err := getTemplates(cfs, filepath.Base(o.refPath))
	if err != nil {
		return err
	}
	helmTemplates[helpersFileName] = helperFuncs

	if o.defaultPath != "" {
		crsWithDefaults, err = loadYAMLFiles(o.defaultPath)
		if err != nil {
			return err
		}
	}

	if o.valuesPath != "" {
		preValues, err = loadValues(o.valuesPath)
		if err != nil {
			return err
		}
	}

	for _, t := range templates {

		visitor := ExpectedValuesFinder{}
		Inspect(t.GetTemplateTree().Root, visitor.Visit())

		helmTemplate, err := convertToHelmTemplate(cfs, t, preValues)
		if err != nil {
			return err
		}
		helmTemplates[t.GetIdentifier()] = helmTemplate

		val, err := getValuesFromJson(crsWithDefaults[path.Base(t.GetIdentifier())], visitor.expected)
		if err != nil {
			return err
		}

		var compValues []map[string]interface{}

		tempValues, err := Unflatten(val)
		if err != nil {
			return err
		}

		if len(tempValues) != 0 {
			helmValues[getCompName(t.GetIdentifier())] = append(compValues, tempValues)
		}
	}

	if preValues != nil {
		merged, err := compare.MergeManifests(&unstructured.Unstructured{Object: preValues}, &unstructured.Unstructured{Object: helmValues})
		if err != nil {
			return fmt.Errorf("failed to merge given values with generated values %w", err)
		}
		helmValues = merged.Object
	}

	return createChart(helmTemplates, helmValues, o.outputDir, o.chartDescription, o.chartVersion)
}

func getTemplates(cfs fs.FS, referenceFileName string) ([]compare.ReferenceTemplate, string, error) {
	ref, err := compare.GetReference(cfs, referenceFileName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get cluster-compare reference  %w", err)
	}
	templates, err := compare.ParseTemplates(ref, cfs)
	if err != nil {
		return templates, "", fmt.Errorf("failed to parse cluster-compare reference templates %w", err)
	}
	helperFuncs, err := createHelmHelperFuncs(cfs, ref.GetTemplateFunctionFiles())
	if err != nil {
		return templates, "", err
	}
	return templates, helperFuncs, nil
}

func createHelmHelperFuncs(cfs fs.FS, tempFuncFiles []string) (string, error) {
	var funcs []string
	for _, filePath := range tempFuncFiles {
		file, err := fs.ReadFile(cfs, filePath)
		if err != nil {
			return "", fmt.Errorf("%w", err)
		}
		funcs = append(funcs, string(file))
	}
	return strings.Join(funcs, "\n"), nil
}

// Function to walk through directories, load YAML files, and create a mapping
func loadYAMLFiles(root string) (map[string]map[string]interface{}, error) {
	filesMapping := make(map[string]map[string]interface{})
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read yaml file %s: %w", path, err)
			}
			var parsedContent map[string]interface{}
			if err := yaml.Unmarshal(content, &parsedContent); err != nil {
				return fmt.Errorf("file in  %s ends ith yaml/yml but is not valid yaml : %w", path, err)
			}
			filesMapping[info.Name()] = parsedContent
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load yaml files: %w", err)
	}
	return filesMapping, nil
}

func sectionFor(compName, key string, helmValues map[string]any) (map[string]any, error) {
	if values, ok := helmValues[compName].([]any); ok && len(values) > 0 {
		if compSection, ok := values[0].(map[string]any); ok && len(compSection) > 0 {
			if section, ok := compSection[key].(map[string]any); ok && len(section) > 0 {
				return section, nil
			}
		}
	}
	return nil, fmt.Errorf("no %q found for %q", key, compName)
}

func removeSection(compName, key string, helmValues map[string]any) {
	if values, ok := helmValues[compName].([]any); ok && len(values) > 0 {
		if section, ok := values[0].(map[string]any); ok && len(section) > 0 {
			delete(section, key)
		}
	}
}

func cgDefaultsFor(compName string, helmValues map[string]any) (map[string]any, error) {
	return sectionFor(compName, "captureGroup_defaults", helmValues)
}

func removeCgDefaults(compName string, helmValues map[string]any) {
	removeSection(compName, "captureGroup_defaults", helmValues)
}

func capturegroupSubstitution(content, compName string, helmValues map[string]any) string {
	if dflts, err := cgDefaultsFor(compName, helmValues); err == nil {
		cgs := compare.CapturegroupIndex(content)
		contentBuilder := strings.Builder{}
		idx := 0
		for _, group := range cgs {
			if idx < group.Start {
				contentBuilder.WriteString(content[idx:group.Start])
			}
			if dflt, ok := dflts[group.Name]; ok {
				fmt.Fprintf(os.Stderr, "  %s replacing CaptureGroup (?<%s>...) at [%d:%d] with default: %v\n", compName, group.Name, group.Start, group.End, dflt)
				contentBuilder.WriteString(fmt.Sprintf("%v", dflt))
			} else {
				contentBuilder.WriteString(content[group.Start:group.End])
			}
			idx = group.End
		}
		if idx < len(content) {
			contentBuilder.WriteString(content[idx:])
		}
		content = contentBuilder.String()
		// Now that we've fully consumed the defaults, strip them so they don't appear in the Helm chart...
		removeCgDefaults(compName, helmValues)
	}
	return content
}

const lookupRoot = "$.Values.global.lookup_substitutions"

type Lookup struct {
	Text  string
	Array bool
	Key   string
	Start int
	End   int
}

var encoder = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func encodeLookupKey(in string) string {
	return strings.TrimRight(encoder.ReplaceAllLiteralString(in, "_"), "_")
}

// Find all `lookupCR` or `lookupCRs` in the text field
func findLookups(content string) []Lookup {
	result := make([]Lookup, 0)
	// The outer loop finds the beginning of the lookup keywords
	for i := 0; i < len(content); i++ {
		idx := strings.Index(content[i:], "lookupCR")
		if idx == -1 {
			// No lookups in the remaining text
			break
		}
		lookup := Lookup{
			Start: idx + i,
		}
		i = lookup.Start + 8
		if i >= len(content) {
			// EOS
			break
		}
		switch c := content[i]; {
		case c == 's':
			lookup.Array = true
			i++
		case !unicode.IsSpace(rune(c)):
			continue
		}
		// Consume the next 4 words, looking out for double-quoted strings and parenthenticals
		words := 0
		pDepth := 0
		quoting := false
		for ; i < len(content); i++ {
			// Find next non-space character
			idx = strings.IndexFunc(content[i:], func(r rune) bool {
				return !unicode.IsSpace(r)
			})
			if idx == -1 {
				// End of string; Not a complete lookup.
				break
			}
			i += idx
			// find the next word end
			for ; i < len(content); i++ {
				switch content[i] {
				case '\\':
					// Escape next character
					i++
				case '(':
					if !quoting {
						pDepth++
					}
				case ')':
					if !quoting {
						pDepth--
					}
				case '"':
					if pDepth == 0 {
						quoting = !quoting
					}
				}
				// Ignore everything until we're out of quotes and out of parentheses
				if pDepth == 0 && !quoting {
					idx = strings.IndexFunc(content[i:], unicode.IsSpace)
					// Either result (end-of-string with no spaces, or found a space) is end-of-word
					words++
					if idx == -1 {
						i = len(content)
					} else {
						i += idx
					}
					break
				}
			}
			if words == 4 {
				lookup.End = i
				lookup.Text = content[lookup.Start:lookup.End]
				lookup.Key = encodeLookupKey(lookup.Text)
				result = append(result, lookup)
				break
			}
		}
	}
	return result
}

func lookupSubstitution(content, compName string) string {
	lookups := findLookups(content)
	if len(lookups) > 0 {
		fmt.Fprintf(os.Stderr, "  %s detected %d lookups: %v\n", compName, len(lookups), lookups)
		contentBuilder := strings.Builder{}
		idx := 0
		for _, lookup := range lookups {
			if idx < lookup.Start {
				contentBuilder.WriteString(content[idx:lookup.Start])
			}
			sub := fmt.Sprintf("(%s.%s)", lookupRoot, lookup.Key)
			fmt.Fprintf(os.Stderr, "  %s replacing {{ %s }} at [%d:%d] with %s\n", compName, lookup.Text, lookup.Start, lookup.End, sub)
			contentBuilder.WriteString(sub)
			idx = lookup.End
		}
		if idx < len(content) {
			contentBuilder.WriteString(content[idx:])
		}
		content = contentBuilder.String()
	}

	return content
}

func convertToHelmTemplate(cfs fs.FS, t compare.ReferenceTemplate, helmValues map[string]any) (string, error) {
	var templateStructure = `{{- $values := list (dict)}}
{{- if .Values.%v}}
{{- $values = .Values.%v }}
{{- end }}
{{- range $values -}}
---
%v 
{{ end -}}
`
	data, err := fs.ReadFile(cfs, t.GetIdentifier())
	if err != nil {
		return "", fmt.Errorf("failed to read template named: %s %w", t.GetIdentifier(), err)
	}

	compName := getCompName(t.GetIdentifier())

	content := string(data)

	if len(t.GetConfig().GetInlineDiffFuncs()) > 0 {
		content = capturegroupSubstitution(content, compName, helmValues)
	}

	// Replace all lookupCR/lookupCRs with canned data
	content = lookupSubstitution(content, compName)

	helmTemplate := fmt.Sprintf(templateStructure, compName, compName, content)

	return helmTemplate, nil
}

func getCompName(templateName string) string {
	compName := strings.TrimSuffix(templateName, ".yaml")
	compName = strings.TrimSuffix(compName, ".yml")
	compName = strings.ReplaceAll(compName, "/", "_")
	compName = strings.ReplaceAll(compName, "-", "_")
	compName = strings.ReplaceAll(compName, ".", "_")
	return compName
}

func loadValues(path string) (map[string]interface{}, error) {
	var values map[string]interface{}
	content, err := os.ReadFile(path)
	if err != nil {
		return values, fmt.Errorf("values file passed to command does not exist: %w", err)
	}
	err = yaml.Unmarshal(content, &values)
	if err != nil {
		return nil, fmt.Errorf("values file passed to command is not valid YAML: %w", err)
	}
	return values, nil
}

func createChart(temps map[string]string, values map[string]any, dir, description, version string) error {
	var files []*chart.File
	var valuesF []*chart.File
	y, err := chartutil.Values(values).YAML()
	if err != nil {
		return fmt.Errorf("failed to convert chart values to YAML: %w", err)
	}
	valuesF = append(valuesF, &chart.File{Name: valuesFileName, Data: []byte(y)})
	for name, content := range temps {
		files = append(files, &chart.File{Name: path.Join(helmTemplatesDir, name), Data: []byte(content)})
	}
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:        path.Base(dir),
			Description: description,
			Version:     version,
		},
		Templates: files,
		Values:    values,
		Raw:       valuesF,
	}
	err = chartutil.SaveDir(ch, path.Dir(dir))
	if err != nil {
		return fmt.Errorf("failed to save helm chart in dir: %w", err)
	}
	return nil
}
