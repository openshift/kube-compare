package compare

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/samber/lo"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// DiffSum Contains the diff output and correlation info of a specific CR
type DiffSum struct {
	DiffOutput         string   `json:"DiffOutput"`
	CorrelatedTemplate string   `json:"CorrelatedTemplate"`
	CRName             string   `json:"CRName"`
	Patched            string   `json:"Patched,omitempty"`
	OverrideReasons    []string `json:"OverrideReason,omitempty"`
	Description        string   `json:"description,omitempty"`
}

func (s DiffSum) String() string {
	t := `
Cluster CR: {{ .CRName }}
Reference File: {{ .CorrelatedTemplate }}
{{- if .Description }}
Description:
{{ .Description | indent 2 }}
{{- end }}
Diff Output: {{or .DiffOutput "None" }}
{{- if ne (len  .Patched) 0 }}
Patched with {{ .Patched }}
{{- if or (eq .OverrideReasons nil) (eq (len .OverrideReasons ) 0)}}
Patch Reasons: {{or .OverrideReasons "<None given>"}}
{{- else }}
Patch Reasons:
{{- range $reason := .OverrideReasons }}
- {{ $reason }}
{{- end }}
{{- end }}
{{- end }}
`
	var buf bytes.Buffer
	tmpl, _ := template.New("DiffSummary").Funcs(sprig.TxtFuncMap()).Parse(t)
	_ = tmpl.Execute(&buf, s)
	return strings.TrimSpace(buf.String())
}

func (s DiffSum) HasDiff() bool {
	return s.DiffOutput != ""
}

func (s DiffSum) WasPatched() bool {
	return s.Patched != ""
}

// Summary Contains all info included in the Summary output of the compare command
type Summary struct {
	ValidationIssues map[string]map[string]ValidationIssue `json:"ValidationIssuses"`
	NumMissing       int                                   `json:"NumMissing"`
	UnmatchedCRS     []string                              `json:"UnmatchedCRS"`
	NumDiffCRs       int                                   `json:"NumDiffCRs"`
	TotalCRs         int                                   `json:"TotalCRs"`
	MetadataHash     string                                `json:"MetadataHash"`
	PatchedCRs       int                                   `json:"patchedCRs"`
}

func newSummary(reference Reference, c *MetricsTracker, numDiffCRs int, templates []ReferenceTemplate, numPatchedCRs int) *Summary {
	s := Summary{NumDiffCRs: numDiffCRs, PatchedCRs: numPatchedCRs}
	s.ValidationIssues, s.NumMissing = reference.GetValidationIssues(c.MatchedTemplatesNames)
	s.TotalCRs = c.getTotalCRs()
	s.UnmatchedCRS = lo.Map(c.UnMatchedCRs, func(r *unstructured.Unstructured, i int) string {
		return apiKindNamespaceName(r)
	})

	hash := sha256.New()

	refBytes, err := yaml.Marshal(reference)
	if err != nil {
		klog.Warning("There was an error in hashing the reference, don't trust the hash")
	}
	hash.Write(refBytes)

	for _, template := range templates {
		for _, node := range template.GetTemplateTree().Root.Nodes {
			hash.Write([]byte(node.String()))
		}
	}

	s.MetadataHash = fmt.Sprintf("%x", hash.Sum(nil))

	return &s
}

func (s Summary) String() string {
	t := `
Summary
CRs with diffs: {{ .NumDiffCRs }}/{{ .TotalCRs }}
{{- if ne (len  .ValidationIssues) 0 }}
CRs in reference missing from the cluster: {{.NumMissing}}
{{- range $groupname, $group := .ValidationIssues }}
{{ $groupname }}:
  {{- range $partname, $issue := $group }}
  {{ $partname }}:
    {{ $issue.Msg }}:
    {{- range $cr := $issue.CRs }}
    - {{ $cr }}
      {{- $md := index $issue.CRMetadata $cr }}
      {{- if $md.Description }}
      Description:
        {{- $md.Description | nindent 8 }}
      {{- end }}
    {{- end }}
  {{- end }}
{{- end }}
{{- else}}
No validation issues with the cluster
{{- end }}
{{- if ne (len  .UnmatchedCRS) 0 }}
Cluster CRs unmatched to reference CRs: {{len  .UnmatchedCRS}}
{{ toYaml .UnmatchedCRS}}
{{- else}}
No CRs are unmatched to reference CRs
{{- end }}
Metadata Hash: {{.MetadataHash}}
{{- if ne .PatchedCRs 0}}
Cluster CRs with patches applied: {{ .PatchedCRs }}
{{- else}}
No patched CRs
{{- end }}
`
	var buf bytes.Buffer
	tmpl, _ := template.New("Summary").Funcs(sprig.TxtFuncMap()).Funcs(template.FuncMap{"toYaml": toYAML}).Parse(t)
	_ = tmpl.Execute(&buf, s)
	return strings.TrimSpace(buf.String())
}

// Output Contains the complete output of the command
type Output struct {
	Summary *Summary   `json:"Summary"`
	Diffs   *[]DiffSum `json:"Diffs"`
	patches []*UserOverride
}

func (o Output) String(showEmptyDiffs bool) string {
	sort.Slice(*o.Diffs, func(i, j int) bool {
		return (*o.Diffs)[i].CorrelatedTemplate+(*o.Diffs)[i].CRName < (*o.Diffs)[j].CorrelatedTemplate+(*o.Diffs)[j].CRName
	})

	diffParts := []string{}

	for _, diffSum := range *o.Diffs {
		if showEmptyDiffs || diffSum.HasDiff() || diffSum.WasPatched() {
			diffParts = append(diffParts, fmt.Sprintln(diffSum.String()))
		}
	}

	var str string
	if len(diffParts) > 0 {
		partsStr := strings.Join(diffParts, fmt.Sprintf("\n%s\n", DiffSeparator))
		str = fmt.Sprintf("%s\n%s\n%s\n", DiffSeparator, partsStr, DiffSeparator)
	}

	return fmt.Sprintf("%s%s\n", str, o.Summary.String())
}

func (o Output) Print(format string, out io.Writer, showEmptyDiffs bool) (int, error) {
	var (
		content []byte
		err     error
	)
	switch format {
	case Json:
		content, err = json.Marshal(o)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal output to json: %w", err)
		}
		content = append(content, []byte("\n")...)

	case Yaml:
		content, err = yaml.Marshal(o)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal output to yaml: %w", err)
		}
	case PatchYaml:
		content, err = yaml.Marshal(o.patches)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal patches to yaml: %w", err)
		}
	default:
		content = []byte(o.String(showEmptyDiffs))
	}
	n, err := out.Write(content)
	if err != nil {
		return n, fmt.Errorf("error occurred when writing output: %w", err)
	}
	return n, nil
}
