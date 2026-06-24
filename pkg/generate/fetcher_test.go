// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMustGatherFetcher(t *testing.T) {
	t.Parallel()

	t.Run("non-existent directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "missing-must-gather")
		_, err := NewMustGatherFetcher(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("path is a file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		filePath := filepath.Join(dir, "notadir")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))
		_, err := NewMustGatherFetcher(filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestMustGatherFetcherFetchResources(t *testing.T) {
	t.Parallel()

	writeClusterScopedYAML := func(t *testing.T, root, relPath, content string) {
		t.Helper()
		full := filepath.Join(root, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	t.Run("single document matches spec", func(t *testing.T) {
		t.Parallel()
		mg := t.TempDir()
		// Parent of cluster-scoped-resources is the data root discovered by findDataRoots.
		writeClusterScopedYAML(t, mg, filepath.Join("bundle", "cluster-scoped-resources", "cm.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: mycm
`)

		fetcher, err := NewMustGatherFetcher(mg)
		require.NoError(t, err)

		spec := &ResourceSpec{Kind: "ConfigMap", APIVersion: "v1", Required: false}
		objs, err := fetcher.FetchResources(context.Background(), spec)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "mycm", objs[0].GetName())
		assert.Equal(t, "ConfigMap", objs[0].GetKind())
		assert.Equal(t, "v1", objs[0].GetAPIVersion())
	})

	t.Run("namespace filter", func(t *testing.T) {
		t.Parallel()
		mg := t.TempDir()
		nsDir := filepath.Join(mg, "bundle", "namespaces", "app-ns")
		require.NoError(t, os.MkdirAll(nsDir, 0o755))
		yamlContent := `apiVersion: v1
kind: Secret
metadata:
  name: s1
  namespace: app-ns
`
		require.NoError(t, os.WriteFile(filepath.Join(nsDir, "secret.yaml"), []byte(yamlContent), 0o600))

		fetcher, err := NewMustGatherFetcher(mg)
		require.NoError(t, err)

		match := &ResourceSpec{Kind: "Secret", APIVersion: "v1", Namespace: "app-ns"}
		objs, err := fetcher.FetchResources(context.Background(), match)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "s1", objs[0].GetName())

		otherNS := &ResourceSpec{Kind: "Secret", APIVersion: "v1", Namespace: "other-ns"}
		objs, err = fetcher.FetchResources(context.Background(), otherNS)
		require.NoError(t, err)
		assert.Len(t, objs, 0)
	})

	t.Run("list document expands items", func(t *testing.T) {
		t.Parallel()
		mg := t.TempDir()
		writeClusterScopedYAML(t, mg, filepath.Join("q", "cluster-scoped-resources", "list.yaml"), `apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ns-from-list
`)

		fetcher, err := NewMustGatherFetcher(mg)
		require.NoError(t, err)

		spec := &ResourceSpec{Kind: "Namespace", APIVersion: "v1", Required: true}
		objs, err := fetcher.FetchResources(context.Background(), spec)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "ns-from-list", objs[0].GetName())
	})

	t.Run("non-list kind with top-level items keeps outer object", func(t *testing.T) {
		t.Parallel()
		mg := t.TempDir()
		writeClusterScopedYAML(t, mg, filepath.Join("q", "cluster-scoped-resources", "cm-items.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-with-items
items:
  - not-a-full-kubernetes-object
`)

		fetcher, err := NewMustGatherFetcher(mg)
		require.NoError(t, err)

		spec := &ResourceSpec{Kind: "ConfigMap", APIVersion: "v1", Required: true}
		objs, err := fetcher.FetchResources(context.Background(), spec)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "cm-with-items", objs[0].GetName())
		assert.Equal(t, "ConfigMap", objs[0].GetKind())
	})

	t.Run("typed list kind suffix expands items", func(t *testing.T) {
		t.Parallel()
		mg := t.TempDir()
		writeClusterScopedYAML(t, mg, filepath.Join("q", "cluster-scoped-resources", "podlist.yaml"), `apiVersion: v1
kind: PodList
items:
  - apiVersion: v1
    kind: Pod
    metadata:
      name: p-from-podlist
      namespace: default
`)

		fetcher, err := NewMustGatherFetcher(mg)
		require.NoError(t, err)

		spec := &ResourceSpec{Kind: "Pod", APIVersion: "v1", Namespace: "default", Required: true}
		objs, err := fetcher.FetchResources(context.Background(), spec)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "p-from-podlist", objs[0].GetName())
	})

	t.Run("no must-gather data", func(t *testing.T) {
		t.Parallel()
		mg := t.TempDir()
		fetcher, err := NewMustGatherFetcher(mg)
		require.NoError(t, err)

		spec := &ResourceSpec{Kind: "ConfigMap", APIVersion: "v1"}
		_, err = fetcher.FetchResources(context.Background(), spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no must-gather data found")
	})
}
