package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	logging "github.com/openshift/kube-compare/addon-tools/generate-metadata/pkg"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

var (
	T bool = true
	F bool = false
)

var defaultRequired *bool = &T

type entry struct {
	componentRequired *bool // these are pointers to define a ternery value, not-set, set-true, set-false
	required          *bool // these are pointers to define a ternery value, not-set, set-true, set-false
	componentName     string
	partName          string
	relPath           string
	path              string
}

func (e entry) ComponentName() string {
	if e.componentName != "" {
		return e.componentName
	}
	return filepath.Base(filepath.Dir(e.path))
}

func (e entry) PartName() string {
	if e.partName != "" {
		return e.partName
	}
	return filepath.Base(filepath.Dir(filepath.Dir(e.path)))
}

var (
	partNameRegex          *regexp.Regexp
	componentNameRegex     *regexp.Regexp
	optionalComponentRegex *regexp.Regexp
	requiredComponentRegex *regexp.Regexp
	optionalRegex          *regexp.Regexp
	requiredRegex          *regexp.Regexp
	logger                 *logging.Logger
)

func init() {
	partNameRegex = regexp.MustCompile(`(?m:^#\s+cluster-compare-part:\s+(.*)\s*$)`)
	componentNameRegex = regexp.MustCompile(`(?m:^#\s+cluster-compare-component:\s+(.*)\s*$)`)
	optionalComponentRegex = regexp.MustCompile(`(?m:^#\s+cluster-compare-component-optional\s*$)`)
	requiredComponentRegex = regexp.MustCompile(`(?m:^#\s+cluster-compare-component-required\s*$)`)
	optionalRegex = regexp.MustCompile(`(?m:^#\s+cluster-compare-optional\s*$)`)
	requiredRegex = regexp.MustCompile(`(?m:^#\s+cluster-compare-required\s*$)`)
}

func createEntry(path, dir string, content []byte) (entry, error) {
	result := entry{path: path}

	relPath, err := filepath.Rel(dir, path)
	if err != nil {
		err = fmt.Errorf("failed to get relitive path: %w", err)
		logger.Error(err)
		return result, err
	}
	result.relPath = relPath

	partName := partNameRegex.FindSubmatch(content)
	if len(partName) != 0 {
		result.partName = string(partName[1])
	}

	componentName := componentNameRegex.FindSubmatch(content)
	if len(componentName) != 0 {
		result.componentName = string(componentName[1])
	}

	componentRequired := requiredComponentRegex.Match(content)
	componentOptional := optionalComponentRegex.Match(content)

	if componentOptional && componentRequired {
		result.componentRequired = &T
		return result, fmt.Errorf("found both required and optional component comments in template %s", path)
	} else if componentOptional {
		result.componentRequired = &F
	} else if componentRequired {
		result.componentRequired = &T
	}

	templateRequired := requiredRegex.Match(content)
	templateOptional := optionalRegex.Match(content)
	if templateOptional && templateRequired {
		// TODO: Maybe default to most restrictive?
		return result, fmt.Errorf("found both required and optional comments in template %s", path)
	} else if templateOptional {
		result.required = &F
	} else if templateRequired {
		result.required = &T
	}

	return result, nil
}

func populateEntry(path, dir string) (entry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("failed to read file: %w", err)
		logger.Error(err)
		return entry{path: path}, err
	}
	return createEntry(path, dir, content)
}

func GatherManifests(dir string, exitOnError bool) ([]entry, error) {
	manifests := make([]entry, 0)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Errorf("error while walking director stucture: %s\n Continuing on", err)
			if exitOnError {
				return err
			}
		}
		if d.IsDir() {
			return nil
		}

		entry, err := populateEntry(path, dir)
		if err != nil {
			logger.Errorf("failed to populate entry for path %s: %s\n Continuing on", path, err)
		}
		manifests = append(manifests, entry)
		return nil
	})
	return manifests, err
}

func GroupManifests(manifests []entry) map[string]map[string][]entry {
	result := make(map[string]map[string][]entry)
	for _, ent := range manifests {
		partName := ent.PartName()
		if _, ok := result[partName]; !ok {
			result[partName] = make(map[string][]entry)
		}
		componentName := ent.ComponentName()
		result[partName][componentName] = append(result[partName][componentName], ent)
	}
	return result
}

type Part struct {
	Name       string
	Components []Component `yaml:"Components"`
}

type ComponentType string

const (
	Required ComponentType = "Required"
	Optional ComponentType = "Optional"
)

type Component struct {
	Name              string
	Type              ComponentType
	RequiredTemplates []string `yaml:"requiredTemplates,omitempty"`
	OptionalTemplates []string `yaml:"optionalTemplates,omitempty"`
}

type MetaData struct {
	Parts []Part `yaml:"Parts"`
}

func joinErrors(errors []error, sep string) string {
	result := ""
	for i, err := range errors {
		if i > 0 {
			result = result + sep
		}
		result = result + err.Error()
	}
	return result
}

func ConstuctMetaData(groupedManifests map[string]map[string][]entry) (MetaData, error) {
	result := MetaData{}
	errors := make([]error, 0)
	for partName, componentMap := range groupedManifests {
		part := Part{Name: partName}
		for componentName, entries := range componentMap {
			component, err := constructComponent(componentName, entries)
			if err != nil {
				errors = append(errors, err)
			}
			part.Components = append(part.Components, component)
		}
		result.Parts = append(result.Parts, part)
	}
	if len(errors) != 0 {
		return result, fmt.Errorf(joinErrors(errors, "\n"))
	}
	return result, nil
}

func constructComponent(name string, templateEntries []entry) (Component, error) {
	result := Component{Name: name}
	runningErrors := make([]error, 0)

	var isRequired *bool
	for _, ent := range templateEntries {
		if ent.componentRequired != nil {
			if ent.componentRequired != isRequired {
				isRequired = &T
				err := errors.New("conflicting component required status")
				logger.Error(err)
				runningErrors = append(runningErrors, err)
			}
		}
		required := ent.required
		if required == nil {
			required = defaultRequired
		}

		if required == &T {
			result.RequiredTemplates = append(result.RequiredTemplates, ent.relPath)
		} else {
			result.OptionalTemplates = append(result.OptionalTemplates, ent.relPath)
		}

	}
	if isRequired == nil {
		isRequired = defaultRequired
	}

	if isRequired == &T {
		result.Type = Required
	} else {
		result.Type = Optional
	}

	if len(runningErrors) != 0 {
		return result, fmt.Errorf(joinErrors(runningErrors, "\n"))
	}
	return result, nil
}

func GenerateMataData(writer io.Writer, dir string, exitOnError bool) {
	// check dir points to a  valid dir
	dir, err := filepath.Abs(dir)
	if err != nil {
		logger.Exitf("Failed to make dir path absolute: %s", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		logger.Exitf("Failed to stat dir path: %s", err)
	}
	if !fi.IsDir() {
		logger.Exitf("dir path is not a valid directory: %s", err)
	}

	manifests, err := GatherManifests(dir, exitOnError)
	if err != nil && exitOnError {
		logger.Exitf("error while gathering manifests: %s", err)
	}
	if len(manifests) == 0 {
		logger.Exit("no manifests found")
	}
	groupedManifests := GroupManifests(manifests)
	metadata, err := ConstuctMetaData(groupedManifests)
	if err != nil && exitOnError {
		logger.Fatal("failed to generate valid metadata file:", err)
	}
	out, err := yaml.Marshal(metadata)
	if err != nil {
		logger.Fatal(err)
	}
	fmt.Fprint(writer, string(out))
}

func InitLogger() {
	logger = logging.GetLogger()
}

func main() {
	var (
		dir, outfile string
		exitOnError  bool
	)

	flag.StringVar(&dir, "dir", "", "Points to the director with you manifests")
	flag.StringVar(&outfile, "outfile", "-", "name of output file")
	flag.BoolVar(&exitOnError, "exitOnError", false, "Points to the director with you manifests")

	klog.InitFlags(nil)
	defer klog.Flush()
	InitLogger()

	flag.Parse()

	var writer io.Writer = os.Stdout
	if outfile != "-" {
		var err error
		writer, err = os.Create(outfile)
		if err != nil {
			logger.Exitf("failed to open output file: %s", err)
		}
	}
	GenerateMataData(writer, dir, exitOnError)
}
