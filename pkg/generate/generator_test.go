// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "simple", input: "my-resource", expected: "my-resource"},
		{name: "empty", input: "", expected: "unnamed"},
		{name: "only special chars", input: "@#$", expected: "unnamed"},
		{name: "spaces and dots", input: "a b.c", expected: "a-b.c"},
		{name: "collapse dashes", input: "a---b", expected: "a-b"},
		{name: "trim dashes", input: "--x--", expected: "x"},
		{name: "dot only", input: ".", expected: "unnamed"},
		{name: "dot dot", input: "..", expected: "unnamed"},
		{name: "contains dot dot after sanitize", input: "a..b", expected: "unnamed"},
		{name: "trim dots and dashes", input: "..--x--..", expected: "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, sanitizeFilename(tt.input))
		})
	}
}

func TestSanitizePathSegment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "simple kind", input: "ConfigMap", expected: "ConfigMap"},
		{name: "empty", input: "", expected: "resource"},
		{name: "dot only", input: ".", expected: "resource"},
		{name: "dot dot", input: "..", expected: "resource"},
		{name: "contains dot dot after sanitize", input: "a..b", expected: "resource"},
		{name: "forward slash", input: "Foo/Bar", expected: "Foo-Bar"},
		{name: "backslash", input: `Foo\Bar`, expected: "Foo-Bar"},
		{name: "trim dots and dashes", input: "..--X--..", expected: "X"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, sanitizePathSegment(tt.input))
		})
	}
}

func TestCleanResource(t *testing.T) {
	t.Parallel()
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":            "cm1",
				"namespace":       "ns1",
				"resourceVersion": "99",
				"uid":             "abc",
				"creationTimestamp": map[string]any{
					"time": "now",
				},
				"generation": int64(1),
				"managedFields": []any{
					map[string]any{"manager": "kubectl"},
				},
				"selfLink":    "/api/v1/...",
				"annotations": map[string]any{},
				"labels":      map[string]any{},
			},
			"data": map[string]any{
				"k": "v",
			},
			"status": map[string]any{
				"phase": "active",
			},
		},
	}

	out := cleanResource(obj, defaultFieldsToOmit())
	require.NotContains(t, out, "status")
	md, ok := out["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "cm1", md["name"])
	assert.Equal(t, "ns1", md["namespace"])
	for _, removed := range []string{"resourceVersion", "uid", "creationTimestamp", "generation", "managedFields", "selfLink"} {
		assert.NotContains(t, md, removed)
	}
	assert.NotContains(t, md, "annotations")
	assert.NotContains(t, md, "labels")
	data, ok := out["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "v", data["k"])
}

func TestCleanResourceKeepsUnlistedAnnotationsAndLabels(t *testing.T) {
	t.Parallel()
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "cm1",
				"annotations": map[string]any{
					"keep.me/custom": "val",
					"kubectl.kubernetes.io/last-applied-configuration": "blob",
				},
				"labels": map[string]any{
					"app":                         "nginx",
					"kubernetes.io/metadata.name": "should-strip",
					"security.openshift.io/scc.podSecurityLabelSync": "true",
				},
			},
		},
	}

	out := cleanResource(obj, defaultFieldsToOmit())
	md := out["metadata"].(map[string]any)
	ann := md["annotations"].(map[string]any)
	assert.Equal(t, "val", ann["keep.me/custom"])
	assert.NotContains(t, ann, "kubectl.kubernetes.io/last-applied-configuration")

	lbl := md["labels"].(map[string]any)
	assert.Equal(t, "nginx", lbl["app"])
	assert.NotContains(t, lbl, "kubernetes.io/metadata.name")
	assert.NotContains(t, lbl, "security.openshift.io/scc.podSecurityLabelSync")
}

func TestCleanResourceStripsLabelKeyPrefixesFromDefaults(t *testing.T) {
	t.Parallel()
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "cm1",
				"labels": map[string]any{
					"app":                                "nginx",
					"operators.coreos.com/foo":           "bar",
					"operators.coreos.com/test":          "v",
					"pod-security.kubernetes.io/enforce": "restricted",
					"pod-security.kubernetes.io/audit":   "restricted",
					"keep.me/should-stay":                "yes",
				},
			},
		},
	}
	out := cleanResource(obj, defaultFieldsToOmit())
	md := out["metadata"].(map[string]any)
	lbl := md["labels"].(map[string]any)
	assert.Equal(t, "nginx", lbl["app"])
	assert.Equal(t, "yes", lbl["keep.me/should-stay"])
	assert.NotContains(t, lbl, "operators.coreos.com/foo")
	assert.NotContains(t, lbl, "operators.coreos.com/test")
	assert.NotContains(t, lbl, "pod-security.kubernetes.io/enforce")
	assert.NotContains(t, lbl, "pod-security.kubernetes.io/audit")
}

func TestGeneratorMetadataDefaultsIncludeLabelPrefixOmissions(t *testing.T) {
	t.Parallel()
	outDir := t.TempDir()
	cfg := &RefgenConfig{
		APIVersion: "refgen/v1",
		OutputDir:  outDir,
		Resources:  []ResourceSpec{{Kind: "ConfigMap", APIVersion: "v1", Required: true}},
	}
	cm := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "alpha"},
		},
	}
	g := NewGenerator(cfg, outDir)
	_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{&cfg.Resources[0]: {cm}})
	require.NoError(t, err)

	metaRaw, err := os.ReadFile(filepath.Join(outDir, "metadata.yaml"))
	require.NoError(t, err)
	var meta map[string]any
	require.NoError(t, yaml.Unmarshal(metaRaw, &meta))
	fto := meta["fieldsToOmit"].(map[string]any)
	items := fto["items"].(map[string]any)
	defaults := items["defaults"].([]any)
	var psa, olm bool
	for _, e := range defaults {
		m := e.(map[string]any)
		p, _ := m["pathToKey"].(string)
		isP := false
		if v, ok := m["isPrefix"]; ok {
			switch x := v.(type) {
			case bool:
				isP = x
			case string:
				isP = strings.EqualFold(x, "true")
			}
		}
		if p == `metadata.labels."pod-security.kubernetes.io/"` && isP {
			psa = true
		}
		if p == `metadata.labels."operators.coreos.com/"` && isP {
			olm = true
		}
	}
	assert.True(t, psa, "defaults should include pod-security label prefix with isPrefix")
	assert.True(t, olm, "defaults should include operators.coreos.com label prefix with isPrefix")
}

func TestCleanResourceCustomOmitFromConfig(t *testing.T) {
	t.Parallel()
	fto := mergeFieldsToOmit(&RefgenConfig{
		OmitAnnotations: []string{"my.operator/strip-me"},
		OmitLabels:      []string{"ephemeral.cluster/hash"},
	})
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "cm1",
				"annotations": map[string]any{
					"keep.me/custom":       "val",
					"my.operator/strip-me": "gone",
				},
				"labels": map[string]any{
					"app":                         "nginx",
					"ephemeral.cluster/hash":      "abc",
					"kubernetes.io/metadata.name": "strip-default",
				},
			},
		},
	}
	out := cleanResource(obj, fto)
	md := out["metadata"].(map[string]any)
	ann := md["annotations"].(map[string]any)
	assert.Equal(t, "val", ann["keep.me/custom"])
	assert.NotContains(t, ann, "my.operator/strip-me")

	lbl := md["labels"].(map[string]any)
	assert.Equal(t, "nginx", lbl["app"])
	assert.NotContains(t, lbl, "ephemeral.cluster/hash")
	assert.NotContains(t, lbl, "kubernetes.io/metadata.name")
}

func TestGeneratorGenerate(t *testing.T) {
	t.Parallel()

	t.Run("required uses allOf in metadata", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{{Kind: "ConfigMap", APIVersion: "v1", Required: true}},
		}
		cm := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "alpha",
				},
			},
		}
		g := NewGenerator(cfg, outDir)
		resourcesBySpec := map[*ResourceSpec][]*unstructured.Unstructured{
			&cfg.Resources[0]: {cm},
		}
		absOut, err := g.Generate(resourcesBySpec)
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(absOut))

		cmPath := filepath.Join(outDir, "ConfigMap", "alpha.yaml")
		_, err = os.Stat(cmPath)
		require.NoError(t, err)

		metaRaw, err := os.ReadFile(filepath.Join(outDir, "metadata.yaml"))
		require.NoError(t, err)
		var meta map[string]any
		require.NoError(t, yaml.Unmarshal(metaRaw, &meta))
		assert.Equal(t, "v2", meta["apiVersion"])
		fto, ok := meta["fieldsToOmit"].(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, fto)

		parts := meta["parts"].([]any)
		require.Len(t, parts, 1)
		part := parts[0].(map[string]any)
		comps := part["components"].([]any)
		require.Len(t, comps, 1)
		comp := comps[0].(map[string]any)
		_, hasAll := comp["allOf"]
		_, hasAny := comp["anyOf"]
		assert.True(t, hasAll)
		assert.False(t, hasAny)
	})

	t.Run("optional uses anyOf in metadata", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{{Kind: "Secret", APIVersion: "v1", Required: false}},
		}
		sec := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]any{
					"name": "token",
				},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{&cfg.Resources[0]: {sec}})
		require.NoError(t, err)

		metaRaw, err := os.ReadFile(filepath.Join(outDir, "metadata.yaml"))
		require.NoError(t, err)
		var meta map[string]any
		require.NoError(t, yaml.Unmarshal(metaRaw, &meta))
		parts := meta["parts"].([]any)
		require.Len(t, parts, 1)
		part := parts[0].(map[string]any)
		comp := part["components"].([]any)[0].(map[string]any)
		_, hasAll := comp["allOf"]
		_, hasAny := comp["anyOf"]
		assert.False(t, hasAll)
		assert.True(t, hasAny)
	})

	t.Run("duplicate names get suffix", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{{Kind: "ConfigMap", APIVersion: "v1", Required: true}},
		}
		one := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "dup"},
			},
		}
		two := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "dup"},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{&cfg.Resources[0]: {one, two}})
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(outDir, "ConfigMap", "dup.yaml"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(outDir, "ConfigMap", "dup-1.yaml"))
		require.NoError(t, err)
	})

	t.Run("same Kind different Required does not clobber prior spec files", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources: []ResourceSpec{
				{Kind: "Namespace", APIVersion: "v1", Required: true},
				{Kind: "Namespace", APIVersion: "v1", Required: false},
			},
		}
		nsOne := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]any{"name": "required-ns"},
			},
		}
		nsTwo := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]any{"name": "optional-ns"},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{
			&cfg.Resources[0]: {nsOne},
			&cfg.Resources[1]: {nsTwo},
		})
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(outDir, "Namespace", "required-ns.yaml"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(outDir, "Namespace", "optional-ns.yaml"))
		require.NoError(t, err)
		metaRaw, err := os.ReadFile(filepath.Join(outDir, "metadata.yaml"))
		require.NoError(t, err)
		var meta map[string]any
		require.NoError(t, yaml.Unmarshal(metaRaw, &meta))
		parts := meta["parts"].([]any)
		require.Len(t, parts, 2, "each ResourceSpec row must get its own metadata part even when Kind matches")
		// Order follows refgen config: required first, then optional.
		partReq := parts[0].(map[string]any)
		compReq := partReq["components"].([]any)[0].(map[string]any)
		_, hasAllReq := compReq["allOf"]
		_, hasAnyReq := compReq["anyOf"]
		assert.True(t, hasAllReq)
		assert.False(t, hasAnyReq)
		pathsReq := compReq["allOf"].([]any)
		require.Len(t, pathsReq, 1)
		assert.Equal(t, "Namespace/required-ns.yaml", pathsReq[0].(map[string]any)["path"])

		partOpt := parts[1].(map[string]any)
		compOpt := partOpt["components"].([]any)[0].(map[string]any)
		_, hasAllOpt := compOpt["allOf"]
		_, hasAnyOpt := compOpt["anyOf"]
		assert.False(t, hasAllOpt)
		assert.True(t, hasAnyOpt)
		pathsOpt := compOpt["anyOf"].([]any)
		require.Len(t, pathsOpt, 1)
		assert.Equal(t, "Namespace/optional-ns.yaml", pathsOpt[0].(map[string]any)["path"])
	})

	t.Run("skips empty resource list still writes metadata", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources: []ResourceSpec{
				{Kind: "Pod", APIVersion: "v1", Required: false},
				{Kind: "Namespace", APIVersion: "v1", Required: true},
			},
		}
		ns := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]any{"name": "openshift"},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{
			&cfg.Resources[0]: {},
			&cfg.Resources[1]: {ns},
		})
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(outDir, "metadata.yaml"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(outDir, "Namespace", "openshift.yaml"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(outDir, "Pod"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("sanitized kind directory", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{{Kind: "Foo/Bar", APIVersion: "v1", Required: true}},
		}
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Foo/Bar",
				"metadata":   map[string]any{"name": "x"},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{&cfg.Resources[0]: {obj}})
		require.NoError(t, err)

		safeKind := sanitizePathSegment("Foo/Bar")
		_, err = os.Stat(filepath.Join(outDir, safeKind, "x.yaml"))
		require.NoError(t, err)
	})

	t.Run("custom omitAnnotations and omitLabels in metadata and CRs", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		cfg := &RefgenConfig{
			APIVersion:      "refgen/v1",
			OutputDir:       outDir,
			OmitAnnotations: []string{"company.com/revision"},
			OmitLabels:      []string{"rollout-id"},
			Resources:       []ResourceSpec{{Kind: "ConfigMap", APIVersion: "v1", Required: true}},
		}
		cm := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "app",
					"annotations": map[string]any{
						"company.com/revision": "99",
						"keep":                 "yes",
					},
					"labels": map[string]any{
						"rollout-id": "r1",
						"app":        "web",
					},
				},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{&cfg.Resources[0]: {cm}})
		require.NoError(t, err)

		cmRaw, err := os.ReadFile(filepath.Join(outDir, "ConfigMap", "app.yaml"))
		require.NoError(t, err)
		var written map[string]any
		require.NoError(t, yaml.Unmarshal(cmRaw, &written))
		md := written["metadata"].(map[string]any)
		ann := md["annotations"].(map[string]any)
		assert.Equal(t, "yes", ann["keep"])
		assert.NotContains(t, ann, "company.com/revision")
		lbl := md["labels"].(map[string]any)
		assert.Equal(t, "web", lbl["app"])
		assert.NotContains(t, lbl, "rollout-id")

		metaRaw, err := os.ReadFile(filepath.Join(outDir, "metadata.yaml"))
		require.NoError(t, err)
		var meta map[string]any
		require.NoError(t, yaml.Unmarshal(metaRaw, &meta))
		fto := meta["fieldsToOmit"].(map[string]any)
		items := fto["items"].(map[string]any)
		defaults := items["defaults"].([]any)
		var hasAnn, hasLbl bool
		for _, e := range defaults {
			m := e.(map[string]any)
			p, _ := m["pathToKey"].(string)
			if p == `metadata.annotations."company.com/revision"` {
				hasAnn = true
			}
			if p == `metadata.labels."rollout-id"` {
				hasLbl = true
			}
		}
		assert.True(t, hasAnn, "custom annotation path in fieldsToOmit.defaults")
		assert.True(t, hasLbl, "custom label path in fieldsToOmit.defaults")
	})
}
