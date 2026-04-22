// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"
)

const minimalPolicyYAML = `
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: test-policy
  namespace: ztp-common
spec:
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: test-config-policy
        spec:
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: v1
                kind: ConfigMap
                metadata:
                  name: my-config
                  namespace: openshift-monitoring
                data:
                  key: value
            - complianceType: musthave
              objectDefinition:
                apiVersion: operator.openshift.io/v1alpha1
                kind: ImageContentSourcePolicy
                metadata:
                  name: my-icsp
                spec:
                  repositoryDigestMirrors: []
`

const policyWithDuplicateKey = `
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: dup-policy
  namespace: ztp-common
spec:
  policy-templates:
    - objectDefinition:
        spec:
          object-templates:
            - objectDefinition:
                kind: ConfigMap
                metadata:
                  name: same-name
                  namespace: ns1
                data:
                  first: a
            - objectDefinition:
                kind: ConfigMap
                metadata:
                  name: same-name
                  namespace: ns1
                data:
                  second: b
`

func TestGetKeysFromExtractedDir_Empty(t *testing.T) {
	dir := t.TempDir()
	keys, err := GetKeysFromExtractedDir(dir)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestGetKeysFromExtractedDir_Missing(t *testing.T) {
	keys, err := GetKeysFromExtractedDir(filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestGetKeysFromExtractedDir_YAMLStems(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ConfigMap_ns_foo.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Secret_ns_bar.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o600))

	keys, err := GetKeysFromExtractedDir(dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ConfigMap_ns_foo", "Secret_ns_bar"}, keys)
}

func TestCorrelate(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "OnlyOld.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "OnlyNew.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "Both.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "Both.yaml"), []byte("{}"), 0o600))

	onlyOld, onlyNew, inBoth, err := Correlate(oldDir, newDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"OnlyOld"}, onlyOld)
	assert.Equal(t, []string{"OnlyNew"}, onlyNew)
	assert.Equal(t, []string{"Both"}, inBoth)
}

func TestCorrelate_InBothSorted(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()
	for _, name := range []string{"Z", "A", "M"} {
		require.NoError(t, os.WriteFile(filepath.Join(oldDir, name+".yaml"), []byte("{}"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(newDir, name+".yaml"), []byte("{}"), 0o600))
	}
	_, _, inBoth, err := Correlate(oldDir, newDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "M", "Z"}, inBoth)
}

func TestNormalizeYAML_SortedKeys(t *testing.T) {
	obj := map[string]any{"b": 2, "a": 1}
	out, err := NormalizeYAML(obj)
	require.NoError(t, err)
	assert.Contains(t, out, "a:")
	assert.Contains(t, out, "b:")
	aIdx := strings.Index(out, "a:")
	bIdx := strings.Index(out, "b:")
	assert.Less(t, aIdx, bIdx)
}

func TestComputeDiff_Identical(t *testing.T) {
	dir := t.TempDir()
	content := "key: value\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(content), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(content), 0o600))

	diff, err := ComputeDiff(filepath.Join(dir, "a.yaml"), filepath.Join(dir, "b.yaml"))
	require.NoError(t, err)
	assert.Empty(t, diff)
}

func TestComputeDiff_Different(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("key: value1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("key: value2\n"), 0o600))

	diff, err := ComputeDiff(filepath.Join(dir, "a.yaml"), filepath.Join(dir, "b.yaml"))
	require.NoError(t, err)
	assert.True(t, strings.Contains(diff, "value1") || strings.Contains(diff, "value2"))
	assert.True(t, strings.Contains(diff, "---") || strings.Contains(diff, "+++"))
}

func TestBuildSummary(t *testing.T) {
	s := BuildSummary([]string{"a"}, []string{"b"}, []string{"c", "d"}, 1)
	assert.Contains(t, s, "Only in old:  1")
	assert.Contains(t, s, "Only in new: 1")
	assert.Contains(t, s, "In both:       2")
	assert.Contains(t, s, "Differ:        1")
}

func TestRun_ReportAndJSON(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "old")
	newDir := filepath.Join(root, "new")
	require.NoError(t, os.MkdirAll(oldDir, 0o750))
	require.NoError(t, os.MkdirAll(newDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "Same.yaml"), []byte("x: 1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "Same.yaml"), []byte("x: 1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "OnlyO.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "OnlyN.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "Diff.yaml"), []byte("a: 1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "Diff.yaml"), []byte("a: 2\n"), 0o600))

	reportPath := filepath.Join(root, "report.txt")
	jsonPath := filepath.Join(root, "comparison.json")
	summary, err := Run(oldDir, newDir, reportPath, jsonPath)
	require.NoError(t, err)
	assert.Contains(t, summary, "Only in old:  1")
	assert.Contains(t, summary, "Only in new: 1")
	assert.Contains(t, summary, "In both:       2")

	reportData, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	assert.Contains(t, string(reportData), "Comparison summary")

	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	var data ComparisonJSON
	require.NoError(t, json.Unmarshal(jsonData, &data))
	assert.Equal(t, []string{"OnlyO"}, data.OnlyOld)
	assert.Equal(t, []string{"OnlyN"}, data.OnlyNew)
	assert.ElementsMatch(t, []string{"Diff", "Same"}, data.InBoth)
	assert.Contains(t, data.Diffs, "Diff")
	assert.NotEmpty(t, data.Diffs["Diff"].OldContent)
	assert.NotEmpty(t, data.Diffs["Diff"].NewContent)
	assert.NotEmpty(t, data.Diffs["Diff"].DiffText)
	assert.Equal(t, oldDir, data.OldExtracted)
	assert.Equal(t, newDir, data.NewExtracted)
}

func TestRun_CreatesParentDirs(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "old")
	newDir := filepath.Join(root, "new")
	require.NoError(t, os.MkdirAll(oldDir, 0o750))
	require.NoError(t, os.MkdirAll(newDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "K.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "K.yaml"), []byte("{}"), 0o600))

	reportPath := filepath.Join(root, "out", "sub", "report.txt")
	jsonPath := filepath.Join(root, "out", "sub", "comparison.json")
	_, err := Run(oldDir, newDir, reportPath, jsonPath)
	require.NoError(t, err)
	assert.FileExists(t, reportPath)
	assert.FileExists(t, jsonPath)
}

func TestRun_NoOverlap(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "old")
	newDir := filepath.Join(root, "new")
	require.NoError(t, os.MkdirAll(oldDir, 0o750))
	require.NoError(t, os.MkdirAll(newDir, 0o750))

	reportPath := filepath.Join(root, "report.txt")
	jsonPath := filepath.Join(root, "comparison.json")
	summary, err := Run(oldDir, newDir, reportPath, jsonPath)
	require.NoError(t, err)
	assert.Contains(t, summary, "Only in old:  0")
	assert.Contains(t, summary, "In both:       0")

	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	var data ComparisonJSON
	require.NoError(t, json.Unmarshal(jsonData, &data))
	assert.Empty(t, data.OnlyOld)
	assert.Empty(t, data.InBoth)
	assert.Empty(t, data.Diffs)
}

func TestCRKeyFromObject(t *testing.T) {
	cases := []struct {
		name     string
		obj      map[string]any
		expected string
	}{
		{
			name: "namespaced resource",
			obj: map[string]any{
				"kind":     "ConfigMap",
				"metadata": map[string]any{"name": "foo", "namespace": "openshift-monitoring"},
			},
			expected: "ConfigMap_openshift-monitoring_foo",
		},
		{
			name: "cluster-scoped resource",
			obj: map[string]any{
				"kind":     "ImageContentSourcePolicy",
				"metadata": map[string]any{"name": "my-icsp"},
			},
			expected: "ImageContentSourcePolicy_my-icsp",
		},
		{
			name: "empty namespace is cluster-scoped",
			obj: map[string]any{
				"kind":     "Resource",
				"metadata": map[string]any{"name": "x", "namespace": ""},
			},
			expected: "Resource_x",
		},
		{
			name: "sanitizes slashes",
			obj: map[string]any{
				"kind":     "Something",
				"metadata": map[string]any{"name": "a/b/c", "namespace": "ns"},
			},
			expected: "Something_ns_a-b-c",
		},
		{
			name:     "missing kind uses Unknown",
			obj:      map[string]any{"metadata": map[string]any{"name": "n", "namespace": "ns"}},
			expected: "Unknown_ns_n",
		},
		{
			name:     "missing name uses unnamed",
			obj:      map[string]any{"kind": "ConfigMap", "metadata": map[string]any{"namespace": "ns"}},
			expected: "ConfigMap_ns_unnamed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, CRKeyFromObject(tc.obj))
		})
	}
}

func TestExtractCRsFromPolicyDoc_MinimalPolicy(t *testing.T) {
	var doc map[string]any
	require.NoError(t, sigsyaml.Unmarshal([]byte(minimalPolicyYAML), &doc))

	result := ExtractCRsFromPolicyDoc(doc)
	require.Len(t, result, 2)
	keys := make([]string, len(result))
	for i, r := range result {
		keys[i] = r.Key
	}
	assert.Contains(t, keys, "ConfigMap_openshift-monitoring_my-config")
	assert.Contains(t, keys, "ImageContentSourcePolicy_my-icsp")
}

func TestExtractCRsFromPolicyDoc_NonPolicy(t *testing.T) {
	doc := map[string]any{"kind": "ConfigMap", "metadata": map[string]any{}}
	assert.Empty(t, ExtractCRsFromPolicyDoc(doc))
}

func TestExtractCRsFromPolicyDoc_NoPolicyTemplates(t *testing.T) {
	doc := map[string]any{"kind": "Policy", "spec": map[string]any{}}
	assert.Empty(t, ExtractCRsFromPolicyDoc(doc))
}

func TestExtractCRs_OneFilePerCR(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	extracted := filepath.Join(root, "extracted")
	require.NoError(t, os.MkdirAll(generated, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(generated, "policies.yaml"), []byte(minimalPolicyYAML), 0o600))

	require.NoError(t, ExtractCRs(generated, extracted))

	entries, err := os.ReadDir(extracted)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	assert.ElementsMatch(t, []string{
		"ConfigMap_openshift-monitoring_my-config.yaml",
		"ImageContentSourcePolicy_my-icsp.yaml",
	}, names)
}

func TestExtractCRs_ValidContent(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	extracted := filepath.Join(root, "extracted")
	require.NoError(t, os.MkdirAll(generated, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(generated, "p.yaml"), []byte(minimalPolicyYAML), 0o600))

	require.NoError(t, ExtractCRs(generated, extracted))

	data, err := os.ReadFile(filepath.Join(extracted, "ConfigMap_openshift-monitoring_my-config.yaml"))
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, sigsyaml.Unmarshal(data, &obj))
	assert.Equal(t, "ConfigMap", obj["kind"])
	meta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "my-config", meta["name"])
	assert.Equal(t, "openshift-monitoring", meta["namespace"])
	dataMap, ok := obj["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", dataMap["key"])
}

func TestExtractCRs_DuplicateKeyLastWins(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	extracted := filepath.Join(root, "extracted")
	require.NoError(t, os.MkdirAll(generated, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(generated, "dup.yaml"), []byte(policyWithDuplicateKey), 0o600))

	require.NoError(t, ExtractCRs(generated, extracted))

	data, err := os.ReadFile(filepath.Join(extracted, "ConfigMap_ns1_same-name.yaml"))
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, sigsyaml.Unmarshal(data, &obj))
	dataMap, ok := obj["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "b", dataMap["second"])
	assert.NotContains(t, dataMap, "first")
}

func TestExtractCRs_CreatesExtractedDir(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	extracted := filepath.Join(root, "extracted")
	require.NoError(t, os.MkdirAll(generated, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(generated, "p.yaml"), []byte(minimalPolicyYAML), 0o600))

	_, err := os.Stat(extracted)
	require.True(t, os.IsNotExist(err))

	require.NoError(t, ExtractCRs(generated, extracted))

	info, err := os.Stat(extracted)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRunCompare_ExtractsAndWritesArtifacts(t *testing.T) {
	root := t.TempDir()
	oldGen := filepath.Join(root, "old-gen")
	newGen := filepath.Join(root, "new-gen")
	require.NoError(t, os.MkdirAll(oldGen, 0o750))
	require.NoError(t, os.MkdirAll(newGen, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(oldGen, "p.yaml"), []byte(minimalPolicyYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newGen, "p.yaml"), []byte(minimalPolicyYAML), 0o600))

	sessionDir := filepath.Join(root, "session")
	require.NoError(t, os.MkdirAll(sessionDir, 0o750))

	summary, err := RunCompare(oldGen, newGen, sessionDir)
	require.NoError(t, err)
	assert.Contains(t, summary, "Comparison summary")
	assert.FileExists(t, filepath.Join(sessionDir, "diff-report.txt"))
	assert.FileExists(t, filepath.Join(sessionDir, "comparison.json"))
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "argocd/example/acmpolicygenerator", CRSPath)
	assert.Equal(t, "source-crs", SourceCRSPath)
	assert.Contains(t, ListOfCRsForSNO, "acm-common-ranGen.yaml")
	assert.Len(t, ListOfCRsForSNO, 4)
}
