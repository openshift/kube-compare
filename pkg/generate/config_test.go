// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid minimal applies defaults", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		err := os.WriteFile(path, []byte(`resources:
  - kind: Namespace
    apiVersion: v1
    required: false
`), 0o600)
		require.NoError(t, err)

		cfg, err := LoadConfig(path)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "refgen/v1", cfg.APIVersion)
		assert.Equal(t, "./generated-reference", cfg.OutputDir)
		require.Len(t, cfg.Resources, 1)
		assert.Equal(t, "Namespace", cfg.Resources[0].Kind)
		assert.Equal(t, "v1", cfg.Resources[0].APIVersion)
		assert.False(t, cfg.Resources[0].Required)
	})

	t.Run("valid with explicit apiVersion and outputDir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		err := os.WriteFile(path, []byte(`apiVersion: refgen/v1
outputDir: ./out
resources:
  - kind: ConfigMap
    apiVersion: v1
    required: true
`), 0o600)
		require.NoError(t, err)

		cfg, err := LoadConfig(path)
		require.NoError(t, err)
		assert.Equal(t, "refgen/v1", cfg.APIVersion)
		assert.Equal(t, "./out", cfg.OutputDir)
		require.Len(t, cfg.Resources, 1)
		assert.True(t, cfg.Resources[0].Required)
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "nope.yaml")
		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration file not found")
		assert.Contains(t, err.Error(), path)
	})

	t.Run("invalid YAML", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		require.NoError(t, os.WriteFile(path, []byte("resources: [\n"), 0o600))

		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid YAML in configuration file")
	})

	t.Run("empty resources list", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: refgen/v1
resources: []
`), 0o600))

		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one resource")
	})

	t.Run("resources key omitted", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "nor.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: refgen/v1
`), 0o600))

		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one resource")
	})

	t.Run("unknown top-level field rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "strict.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: refgen/v1
unknownTopLevelKey: true
resources:
  - kind: Namespace
    apiVersion: v1
    required: false
`), 0o600))

		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid YAML in configuration file")
	})

	t.Run("valid omitAnnotations and omitLabels", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "omit.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: refgen/v1
omitAnnotations:
  - my.operator/audit-id
omitLabels:
  - batch.kubernetes.io/job-name
resources:
  - kind: Namespace
    apiVersion: v1
    required: false
`), 0o600))

		cfg, err := LoadConfig(path)
		require.NoError(t, err)
		assert.Equal(t, []string{"my.operator/audit-id"}, cfg.OmitAnnotations)
		assert.Equal(t, []string{"batch.kubernetes.io/job-name"}, cfg.OmitLabels)
	})

	t.Run("empty omit annotation key rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "badomit.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: refgen/v1
omitAnnotations:
  - ""
resources:
  - kind: Namespace
    apiVersion: v1
    required: false
`), 0o600))

		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "omitAnnotations")
	})

	t.Run("omit key with double quote rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "badquote.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: refgen/v1
omitLabels:
  - 'bad"key'
resources:
  - kind: Namespace
    apiVersion: v1
    required: false
`), 0o600))

		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "omitLabels")
	})
}
