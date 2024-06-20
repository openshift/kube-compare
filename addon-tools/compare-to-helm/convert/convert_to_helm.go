package convert

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/openshift/kube-compare/pkg/compare"
	"github.com/spf13/cobra"
	"github.com/wolfeidau/unflatten"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const helpersFileName = "_helpers.tpl"

func createChart(temps map[string]string, values map[string]any, dir string) {
	var files []*chart.File
	var valuesF []*chart.File
	y, _ := chartutil.Values(values).YAML()
	valuesF = append(valuesF, &chart.File{Name: "values.yaml", Data: []byte(y)})
	for tempName, tempContent := range temps {
		files = append(files, &chart.File{Name: "templates/" + tempName, Data: []byte(tempContent)})
	}
	// Define a new chart
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:        path.Base(dir),
			Description: "A Helm chart created with Go",
			Version:     "1",
		},
		Templates: files,
		Values:    values,
		Raw:       valuesF,
	}
	// Save the chart to a directory
	err := chartutil.SaveDir(ch, dir)
	if err != nil {
		panic(err)
	}
}

type Options struct {
	refPath     string
	outputDir   string
	defaultPath string
	valuesPath  string
}

func convertToHelmTemplate(cfs fs.FS, t *compare.ReferenceTemplate) (string, string, error) {
	content, err := fs.ReadFile(cfs, t.Name())
	if err != nil {
		return "", "", err
	}
	compName := strings.TrimSuffix(t.Name(), ".yaml")
	compName = strings.ReplaceAll(compName, "/", "_")
	compName = strings.ReplaceAll(compName, "-", "_")
	content2 := "{{ range .Values." + compName + " -}}\n---\n" + string(content) + "{{ end -}}"
	return compName, content2, nil
}

func loadValues(path string) (map[string]interface{}, error) {
	var values map[string]interface{}
	content, err := os.ReadFile(path)
	if err != nil {
		return values, err
	}
	err = yaml.Unmarshal(content, &values)
	return values, err
}

func createHelmHelperFuncs(cfs fs.FS, tempFuncFiles []string) (string, error) {
	var funcs []string
	for _, filePath := range tempFuncFiles {
		file, err := fs.ReadFile(cfs, filePath)
		if err != nil {
			return "", fmt.Errorf("%v", err)
		}
		funcs = append(funcs, string(file))
	}
	return strings.Join(funcs, "\n"), nil
}

func convertToHelm(o *Options) error {
	cfs, err := compare.GetRefFS(o.refPath)
	if err != nil {
		return err
	}
	ref, err := compare.GetReference(cfs, "metadata.yaml")
	if err != nil {
		return err
	}
	temps := make(map[string]string)

	yamlFiles := make(map[string]map[string]interface{})
	if o.defaultPath != "" {
		yamlFiles, err = loadYAMLFiles(o.defaultPath)
		if err != nil {
			return err
		}
	}

	values := make(map[string]any)

	tes, err := compare.ParseTemplates(ref.GetTemplates(), ref.TemplateFunctionFiles, cfs, &ref)
	helpers, err := createHelmHelperFuncs(cfs, ref.TemplateFunctionFiles)
	if err != nil {
		return err
	}
	temps[helpersFileName] = helpers

	for _, t := range tes {

		visitor := CollectExpectedVisitor{}
		Inspect(t.Tree.Root, visitor.Visit())
		compName, compBody, err := convertToHelmTemplate(cfs, t)
		temps[t.Name()] = compBody
		if err != nil {
			return err
		}
		val, err := getValuesFromJSONPaths(yamlFiles[path.Base(t.Name())], visitor.expected)
		tree := unflatten.Unflatten(val, func(k string) []string { return strings.Split(k, ".") })
		var compValues []map[string]interface{}

		values[compName] = append(compValues, tree) // just create a list
	}
	if o.valuesPath != "" {
		preValues, err := loadValues(o.valuesPath)
		if err != nil {
			return err
		}
		merged, err := compare.MergeManifests(&unstructured.Unstructured{Object: preValues}, &unstructured.Unstructured{Object: values})
		values = merged.Object
	}
	createChart(temps, values, o.outputDir)
	return nil
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
				return err
			}
			var parsedContent map[string]interface{}
			if err := yaml.Unmarshal(content, &parsedContent); err != nil {
				return err
			}
			filesMapping[info.Name()] = parsedContent
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return filesMapping, nil
}

func NewCmd() *cobra.Command {
	options := Options{}
	cmd := &cobra.Command{
		Use:   "create-report -j <COMPARE_JSON_OUTPUT_PATH>",
		Short: "convert2helm: ",
		Long:  "",

		RunE: func(cmd *cobra.Command, args []string) error {
			return convertToHelm(&options)
		},
	}
	cmd.Flags().StringVarP(&options.refPath, "reference", "r", "", "Path to reference config file.")
	cmd.Flags().StringVarP(&options.outputDir, "helm-name", "n", "Chart", "Path to save the helm chart")
	cmd.Flags().StringVarP(&options.defaultPath, "defaults", "d", "", "Path to directory with the CRs that the tool will extract default values from")
	cmd.Flags().StringVarP(&options.valuesPath, "values", "v", "", "Path to existing values.yaml file")
	return cmd
}
