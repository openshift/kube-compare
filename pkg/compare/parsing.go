// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"text/template/parse"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type Reference interface {
	GetAPIVersion() string
	GetTemplates() []ReferenceTemplate
	GetValidationIssues(matchedTemplates map[string]int) (map[string]map[string]ValidationIssue, int)
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
	GetDescription() string
}

type TemplateConfig interface {
	GetAllowMerge() bool
	GetFieldsToOmitRefs() []string
	GetInlineDiffFuncs() map[string]inlineDiffType
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
		version = ReferenceVersionV1
	} else {
		version = strings.TrimSpace(fmt.Sprint(versionAny))
	}

	if strings.EqualFold(version, ReferenceVersionV1) {
		ref, err := getReferenceV1(fsys, referenceFileName)
		return ref, err
	} else if strings.EqualFold(version, ReferenceVersionV2) {
		ref, err := getReferenceV2(fsys, referenceFileName)
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
	} else if strings.EqualFold(ref.GetAPIVersion(), ReferenceVersionV2) {
		refV2 := ref.(*ReferenceV2)
		return ParseV2Templates(refV2, fsys)
	}

	return nil, fmt.Errorf("unknown reference file apiVersion: '%s'", ref.GetAPIVersion())
}

type parsableTemplate interface {
	ReferenceTemplate
	ValidateFieldsToOmit(fieldsToOmit FieldsToOmit) error
	setTemplate(t *template.Template)
	setMetadata(m *unstructured.Unstructured)
	prepareForExec()
	postExecValidate() error
}

func parseTemplatesCommon[T parsableTemplate](templates []T, functionFiles []string, fsys fs.FS, fieldsToOmit FieldsToOmit) ([]ReferenceTemplate, error) {
	var errs []error
	result := make([]ReferenceTemplate, 0, len(templates))
	for _, temp := range templates {
		result = append(result, temp)
		parsedTemp, err := template.New(path.Base(temp.GetPath())).Funcs(FuncMap()).ParseFS(fsys, temp.GetPath())
		if err != nil {
			errs = append(errs, fmt.Errorf(templatesCantBeParsed, temp.GetPath(), err))
			continue
		}
		if len(functionFiles) > 0 {
			parsedTemp, err = parsedTemp.ParseFS(fsys, functionFiles...)
			if err != nil {
				errs = append(errs, fmt.Errorf(templatesFunctionsCantBeParsed, err))
				continue
			}
		}
		temp.setTemplate(parsedTemp)
		temp.prepareForExec()
		klog.V(1).Infof("Pre-processing template %s with empty data", temp.GetPath())
		metadata, err := temp.Exec(map[string]any{})
		temp.setMetadata(metadata)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse template %s with empty data: %w", temp.GetPath(), err))
		} else {
			if err := temp.postExecValidate(); err != nil {
				errs = append(errs, err)
			}
			if metadata != nil && metadata.GetKind() == "" {
				errs = append(errs, fmt.Errorf("template missing kind: %s", temp.GetPath()))
			}
		}
		if err := temp.ValidateFieldsToOmit(fieldsToOmit); err != nil {
			errs = append(errs, err)
		}
	}
	return result, errors.Join(errs...) // nolint:wrapcheck
}

type CRMetadata struct {
	Description string `json:"description,omitempty"`
}

type ValidationIssue struct {
	Msg        string                `json:"Msg,omitempty"`
	CRs        []string              `json:"CRs,omitempty"`
	CRMetadata map[string]CRMetadata `json:"crMetadata,omitempty"`
}
