// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"os"
	"path/filepath"
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
				"selfLink":      "/api/v1/...",
				"annotations": map[string]any{},
				"labels":        map[string]any{},
			},
			"data": map[string]any{
				"k": "v",
			},
			"status": map[string]any{
				"phase": "active",
			},
		},
	}

	out := cleanResource(obj)
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

func TestGeneratorGenerate(t *testing.T) {
	t.Parallel()

	t.Run("required uses allOf in metadata", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		spec := &ResourceSpec{Kind: "ConfigMap", APIVersion: "v1", Required: true}
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{*spec},
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
			spec: {cm},
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
		spec := &ResourceSpec{Kind: "Secret", APIVersion: "v1", Required: false}
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{*spec},
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
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{spec: {sec}})
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
		spec := &ResourceSpec{Kind: "ConfigMap", APIVersion: "v1", Required: true}
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{*spec},
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
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{spec: {one, two}})
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(outDir, "ConfigMap", "dup.yaml"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(outDir, "ConfigMap", "dup-1.yaml"))
		require.NoError(t, err)
	})

	t.Run("skips empty resource list still writes metadata", func(t *testing.T) {
		t.Parallel()
		outDir := t.TempDir()
		specEmpty := &ResourceSpec{Kind: "Pod", APIVersion: "v1", Required: false}
		specFilled := &ResourceSpec{Kind: "Namespace", APIVersion: "v1", Required: true}
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{*specEmpty, *specFilled},
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
			specEmpty:  {},
			specFilled: {ns},
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
		spec := &ResourceSpec{Kind: "Foo/Bar", APIVersion: "v1", Required: true}
		cfg := &RefgenConfig{
			APIVersion: "refgen/v1",
			OutputDir:  outDir,
			Resources:  []ResourceSpec{*spec},
		}
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Foo/Bar",
				"metadata":   map[string]any{"name": "x"},
			},
		}
		g := NewGenerator(cfg, outDir)
		_, err := g.Generate(map[*ResourceSpec][]*unstructured.Unstructured{spec: {obj}})
		require.NoError(t, err)

		safeKind := sanitizePathSegment("Foo/Bar")
		_, err = os.Stat(filepath.Join(outDir, safeKind, "x.yaml"))
		require.NoError(t, err)
	})
}
