// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	sigsyaml "sigs.k8s.io/yaml"
)

var slashRe = regexp.MustCompile(`/+`)

// CRKeyFromObject builds a stable key for a CR: kind_namespace_name, or kind_name for cluster-scoped.
func CRKeyFromObject(obj map[string]any) string {
	kind, _ := obj["kind"].(string)
	if kind == "" {
		kind = "Unknown"
	}
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = make(map[string]any)
	}
	name, _ := meta["name"].(string)
	if name == "" {
		name = "unnamed"
	}
	ns, _ := meta["namespace"].(string)
	var key string
	if ns == "" {
		key = kind + "_" + name
	} else {
		key = kind + "_" + ns + "_" + name
	}
	return slashRe.ReplaceAllString(key, "-")
}

// CRPair is a key and object pair from a Policy document.
type CRPair struct {
	Key string
	Obj map[string]any
}

// ExtractCRsFromPolicyDoc returns CRPair for each CR embedded in a Policy document.
func ExtractCRsFromPolicyDoc(doc map[string]any) []CRPair {
	if kind, _ := doc["kind"].(string); kind != "Policy" {
		return nil
	}
	var out []CRPair
	spec, _ := doc["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	templates, _ := spec["policy-templates"].([]any)
	for _, t := range templates {
		tm, _ := t.(map[string]any)
		if tm == nil {
			continue
		}
		od, _ := tm["objectDefinition"].(map[string]any)
		if od == nil {
			continue
		}
		innerSpec, _ := od["spec"].(map[string]any)
		if innerSpec == nil {
			continue
		}
		objTemplates, _ := innerSpec["object-templates"].([]any)
		for _, ot := range objTemplates {
			otm, _ := ot.(map[string]any)
			if otm == nil {
				continue
			}
			obj, _ := otm["objectDefinition"].(map[string]any)
			if obj == nil {
				continue
			}
			key := CRKeyFromObject(obj)
			out = append(out, CRPair{Key: key, Obj: obj})
		}
	}
	return out
}

// ExtractCRs reads all *.yaml in generatedDir, collects CRs from Policy files, writes extractedDir with one file per key.
func ExtractCRs(generatedDir, extractedDir string) error {
	generatedDir = filepath.Clean(generatedDir)
	extractedDir = filepath.Clean(extractedDir)
	if err := os.MkdirAll(extractedDir, 0o750); err != nil {
		return fmt.Errorf("mkdir extracted dir: %w", err)
	}
	entries, err := readCleanDir(generatedDir)
	if err != nil {
		return fmt.Errorf("read generated dir: %w", err)
	}
	collected := make(map[string]map[string]any)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".yaml") {
			continue
		}
		path, err := safeJoinPath(generatedDir, e.Name())
		if err != nil {
			return fmt.Errorf("unsafe entry in generated dir: %w", err)
		}
		data, err := readCleanFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		docs := unmarshalMultiDocYAML(data)
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			for _, pair := range ExtractCRsFromPolicyDoc(doc) {
				collected[pair.Key] = pair.Obj
			}
		}
	}
	keys := make([]string, 0, len(collected))
	for k := range collected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		norm, err := normalizeYAML(collected[key])
		if err != nil {
			return err
		}
		outPath, err := safeJoinPath(extractedDir, key+".yaml")
		if err != nil {
			return fmt.Errorf("unsafe CR key %q: %w", key, err)
		}
		if err := writeCleanFile(outPath, []byte(norm)); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
	}
	return nil
}

func unmarshalMultiDocYAML(data []byte) []map[string]any {
	var docs []map[string]any
	content := string(data)
	// Strip leading document marker so splitting on "\n---" works for all cases
	content = strings.TrimPrefix(content, "---\n")
	content = strings.TrimPrefix(content, "---\r\n")
	parts := strings.Split(content, "\n---")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var doc map[string]any
		if err := sigsyaml.Unmarshal([]byte(part), &doc); err != nil {
			continue
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
	if len(docs) == 0 {
		var single map[string]any
		if err := sigsyaml.Unmarshal(data, &single); err == nil && single != nil {
			docs = []map[string]any{single}
		}
	}
	return docs
}

func getKeysFromExtractedDir(dir string) ([]string, error) {
	entries, err := readCleanDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".yaml") {
			keys = append(keys, strings.TrimSuffix(name, filepath.Ext(name)))
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// GetKeysFromExtractedDir returns sorted list of CR keys (filename stem without .yaml).
func GetKeysFromExtractedDir(dir string) ([]string, error) {
	return getKeysFromExtractedDir(dir)
}

// Correlate returns onlyOld, onlyNew, inBoth key lists (all sorted).
func Correlate(oldDir, newDir string) (onlyOld, onlyNew, inBoth []string, err error) {
	return correlate(oldDir, newDir)
}

// NormalizeYAML marshals obj with sorted keys for stable diff.
func NormalizeYAML(obj map[string]any) (string, error) {
	return normalizeYAML(obj)
}

// ComputeDiff loads both YAMLs, normalizes them, returns unified diff text.
func ComputeDiff(oldPath, newPath string) (string, error) {
	return computeDiff(oldPath, newPath)
}

// BuildSummary builds the summary block text.
func BuildSummary(onlyOld, onlyNew, inBoth []string, numDiffer int) string {
	return buildSummary(onlyOld, onlyNew, inBoth, numDiffer)
}

func correlate(oldDir, newDir string) (onlyOld, onlyNew, inBoth []string, err error) {
	oldKeys, err := getKeysFromExtractedDir(oldDir)
	if err != nil {
		return nil, nil, nil, err
	}
	newKeys, err := getKeysFromExtractedDir(newDir)
	if err != nil {
		return nil, nil, nil, err
	}
	oldSet := make(map[string]struct{})
	for _, k := range oldKeys {
		oldSet[k] = struct{}{}
	}
	newSet := make(map[string]struct{})
	for _, k := range newKeys {
		newSet[k] = struct{}{}
	}
	for _, k := range oldKeys {
		if _, in := newSet[k]; !in {
			onlyOld = append(onlyOld, k)
		} else {
			inBoth = append(inBoth, k)
		}
	}
	for _, k := range newKeys {
		if _, in := oldSet[k]; !in {
			onlyNew = append(onlyNew, k)
		}
	}
	sort.Strings(onlyOld)
	sort.Strings(onlyNew)
	sort.Strings(inBoth)
	return onlyOld, onlyNew, inBoth, nil
}

func sortMapKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		switch tv := v.(type) {
		case map[string]any:
			out[k] = sortMapKeys(tv)
		case []any:
			out[k] = sortSlice(tv)
		default:
			out[k] = v
		}
	}
	return out
}

func sortSlice(s []any) []any {
	out := make([]any, len(s))
	for i, v := range s {
		switch tv := v.(type) {
		case map[string]any:
			out[i] = sortMapKeys(tv)
		case []any:
			out[i] = sortSlice(tv)
		default:
			out[i] = v
		}
	}
	return out
}

func normalizeYAML(obj map[string]any) (string, error) {
	sorted := sortMapKeys(obj)
	b, err := sigsyaml.Marshal(sorted)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}
	return string(b), nil
}

func computeDiff(oldPath, newPath string) (string, error) {
	oldData, err := readCleanFile(oldPath)
	if err != nil {
		return "", fmt.Errorf("read old %s: %w", oldPath, err)
	}
	newData, err := readCleanFile(newPath)
	if err != nil {
		return "", fmt.Errorf("read new %s: %w", newPath, err)
	}
	var oldObj, newObj map[string]any
	if err := sigsyaml.Unmarshal(oldData, &oldObj); err != nil {
		oldObj = nil
	}
	if err := sigsyaml.Unmarshal(newData, &newObj); err != nil {
		newObj = nil
	}
	if oldObj == nil {
		oldObj = make(map[string]any)
	}
	if newObj == nil {
		newObj = make(map[string]any)
	}
	oldStr, err := normalizeYAML(oldObj)
	if err != nil {
		return "", fmt.Errorf("normalize old: %w", err)
	}
	newStr, err := normalizeYAML(newObj)
	if err != nil {
		return "", fmt.Errorf("normalize new: %w", err)
	}
	return unifiedDiff(oldStr, newStr, filepath.Base(oldPath), filepath.Base(newPath)), nil
}

// unifiedDiff computes a line-level unified diff between two strings using diffmatchpatch.
func unifiedDiff(oldStr, newStr, fromFile, toFile string) string {
	dmp := diffmatchpatch.New()
	a, b, c := dmp.DiffLinesToChars(oldStr, newStr)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, c)
	diffs = dmp.DiffCleanupSemantic(diffs)

	allEqual := true
	for _, d := range diffs {
		if d.Type != diffmatchpatch.DiffEqual {
			allEqual = false
			break
		}
	}
	if allEqual {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("--- " + fromFile + "\n")
	buf.WriteString("+++ " + toFile + "\n")
	for _, d := range diffs {
		lines := strings.Split(strings.TrimRight(d.Text, "\n"), "\n")
		for _, line := range lines {
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				buf.WriteString(" " + line + "\n")
			case diffmatchpatch.DiffDelete:
				buf.WriteString("-" + line + "\n")
			case diffmatchpatch.DiffInsert:
				buf.WriteString("+" + line + "\n")
			}
		}
	}
	return buf.String()
}

func buildSummary(onlyOld, onlyNew, inBoth []string, numDiffer int) string {
	return fmt.Sprintf("Comparison summary\n==================\nOnly in old:  %d\nOnly in new: %d\nIn both:       %d\nDiffer:        %d\n\n",
		len(onlyOld), len(onlyNew), len(inBoth), numDiffer)
}

// ComparisonJSON is the shape of the comparison JSON file.
type ComparisonJSON struct {
	OnlyOld      []string               `json:"only_old"`
	OnlyNew      []string               `json:"only_new"`
	InBoth       []string               `json:"in_both"`
	Diffs        map[string]CRDiffEntry `json:"diffs"`
	Summary      string                 `json:"summary"`
	OldExtracted string                 `json:"old_extracted"`
	NewExtracted string                 `json:"new_extracted"`
}

// CRDiffEntry holds old/new content and diff text for one CR key.
type CRDiffEntry struct {
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
	DiffText   string `json:"diff_text"`
}

// RunCompare extracts CRs from generated policy directories, compares them, and writes a report and
// comparison JSON under sessionDir. Returns the summary text.
func RunCompare(oldGenerated, newGenerated, sessionDir string) (string, error) {
	oldGenerated = filepath.Clean(oldGenerated)
	newGenerated = filepath.Clean(newGenerated)
	sessionDir = filepath.Clean(sessionDir)
	oldExtracted := filepath.Join(sessionDir, "old-extracted")
	newExtracted := filepath.Join(sessionDir, "new-extracted")
	if err := os.MkdirAll(oldExtracted, 0o750); err != nil {
		return "", fmt.Errorf("mkdir old-extracted: %w", err)
	}
	if err := os.MkdirAll(newExtracted, 0o750); err != nil {
		return "", fmt.Errorf("mkdir new-extracted: %w", err)
	}
	if err := ExtractCRs(oldGenerated, oldExtracted); err != nil {
		return "", fmt.Errorf("extract old: %w", err)
	}
	if err := ExtractCRs(newGenerated, newExtracted); err != nil {
		return "", fmt.Errorf("extract new: %w", err)
	}
	reportPath := filepath.Join(sessionDir, "diff-report.txt")
	comparisonJSONPath := filepath.Join(sessionDir, "comparison.json")
	return Run(oldExtracted, newExtracted, reportPath, comparisonJSONPath)
}

// Run performs correlate, diff, writes report and comparison JSON; returns summary.
func Run(oldExtracted, newExtracted, reportPath, comparisonJSONPath string) (string, error) {
	oldExtracted = filepath.Clean(oldExtracted)
	newExtracted = filepath.Clean(newExtracted)
	reportPath = filepath.Clean(reportPath)
	comparisonJSONPath = filepath.Clean(comparisonJSONPath)

	onlyOld, onlyNew, inBoth, err := correlate(oldExtracted, newExtracted)
	if err != nil {
		return "", fmt.Errorf("correlate: %w", err)
	}

	diffs := make(map[string]CRDiffEntry)
	numDiffer := 0
	for _, key := range inBoth {
		oldPath, err := safeJoinPath(oldExtracted, key+".yaml")
		if err != nil {
			return "", fmt.Errorf("unsafe key %q for old: %w", key, err)
		}
		newPath, err := safeJoinPath(newExtracted, key+".yaml")
		if err != nil {
			return "", fmt.Errorf("unsafe key %q for new: %w", key, err)
		}
		diffText, err := computeDiff(oldPath, newPath)
		if err != nil {
			return "", fmt.Errorf("diff %s: %w", key, err)
		}
		oldContent, err := readCleanFile(oldPath)
		if err != nil {
			return "", fmt.Errorf("read old content %s: %w", key, err)
		}
		newContent, err := readCleanFile(newPath)
		if err != nil {
			return "", fmt.Errorf("read new content %s: %w", key, err)
		}
		diffs[key] = CRDiffEntry{
			OldContent: string(oldContent),
			NewContent: string(newContent),
			DiffText:   diffText,
		}
		if strings.TrimSpace(diffText) != "" {
			numDiffer++
		}
	}

	summary := buildSummary(onlyOld, onlyNew, inBoth, numDiffer)

	reportLines := []string{summary, ""}
	for _, key := range inBoth {
		if strings.TrimSpace(diffs[key].DiffText) != "" {
			reportLines = append(reportLines, "--- "+key+" ---", diffs[key].DiffText, "")
		}
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o750); err != nil {
		return "", fmt.Errorf("mkdir report dir: %w", err)
	}
	if err := writeCleanFile(reportPath, []byte(strings.Join(reportLines, "\n"))); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}

	comparison := ComparisonJSON{
		OnlyOld:      onlyOld,
		OnlyNew:      onlyNew,
		InBoth:       inBoth,
		Diffs:        diffs,
		Summary:      summary,
		OldExtracted: oldExtracted,
		NewExtracted: newExtracted,
	}
	comparisonBytes, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal comparison: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(comparisonJSONPath), 0o750); err != nil {
		return "", fmt.Errorf("mkdir comparison dir: %w", err)
	}
	if err := writeCleanFile(comparisonJSONPath, comparisonBytes); err != nil {
		return "", fmt.Errorf("write comparison: %w", err)
	}
	return summary, nil
}
