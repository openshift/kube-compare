// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template/parse"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

type Reference interface {
	GetAPIVersion() string
	GetTemplates() []ReferenceTemplate
	GetMissingCRs(matchedTemplates map[string]int) (map[string]map[string][]string, int)
	GetFieldsToOmit() FieldsToOmit
	GetTemplateFunctionFiles() []string
}

type ReferenceTemplate interface {
	GetFieldsToOmit(fieldsToOmit FieldsToOmit) []*ManifestPathV1
	Exec(params map[string]any) (*unstructured.Unstructured, error)
	GetMetadata() *unstructured.Unstructured
	GetIdentifier() string
	GetPath() string
	GetConfig() TemplateConfig
	GetTemplateTree() *parse.Tree
}

type TemplateConfig interface {
	GetAllowMerge() bool
	GetFieldsToOmitRefs() []string
}

type FieldsToOmit interface {
	GetDefault() string
	GetItems() map[string][]*ManifestPathV1
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
	var verCheck map[string]any
	err := parseYaml(fsys, referenceFileName, &verCheck, refConfNotExistsError, refConfigNotInFormat)
	if err != nil {
		return nil, err
	}
	versionAny, ok := verCheck["apiVersion"]
	var version string
	if !ok {
		version = "v1"
	} else {
		version = strings.TrimSpace(fmt.Sprint(versionAny))
	}

	if strings.EqualFold(version, ReferenceVersionV1) {
		ref, err := getReferenceV1(fsys, referenceFileName)
		return ref, err
	}

	return nil, fmt.Errorf("unknown reference file apiVersion: '%s'", version)

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

func ParseTemplates(ref Reference, fsys fs.FS) ([]ReferenceTemplate, error) {
	if strings.EqualFold(ref.GetAPIVersion(), ReferenceVersionV1) {
		refV1 := ref.(*ReferenceV1)
		return ParseV1Templates(refV1, fsys)
	}
	return nil, fmt.Errorf("unknown reference file apiVersion: '%s'", ref.GetAPIVersion())
}
