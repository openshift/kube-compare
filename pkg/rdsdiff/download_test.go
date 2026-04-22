// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubTreeURL_FullURL(t *testing.T) {
	u, err := ParseGitHubTreeURL("https://github.com/openshift-kni/telco-reference/tree/konflux-telco-core-rds-4-20/telco-ran/configuration")
	require.NoError(t, err)
	assert.Equal(t, "openshift-kni", u.Owner)
	assert.Equal(t, "telco-reference", u.Repo)
	assert.Equal(t, "konflux-telco-core-rds-4-20", u.Branch)
	assert.Equal(t, "telco-ran/configuration", u.Path)
	assert.Equal(t, "https://github.com/openshift-kni/telco-reference/archive/refs/heads/konflux-telco-core-rds-4-20.zip", u.ArchiveURL())
	assert.Equal(t, "telco-reference-konflux-telco-core-rds-4-20", u.TopLevelDir())
}

func TestParseGitHubTreeURL_BranchOnly(t *testing.T) {
	u, err := ParseGitHubTreeURL("https://github.com/org/repo/tree/main")
	require.NoError(t, err)
	assert.Equal(t, "org", u.Owner)
	assert.Equal(t, "repo", u.Repo)
	assert.Equal(t, "main", u.Branch)
	assert.Empty(t, u.Path)
}

func TestParseGitHubTreeURL_Errors(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty URL", ""},
		{"non-GitHub URL", "https://gitlab.com/org/repo/-/tree/branch/path"},
		{"URL without tree", "https://github.com/org/repo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseGitHubTreeURL(tc.url)
			assert.Error(t, err)
		})
	}
}

func TestValidateConfigurationRoot_MissingSourceCRS(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, CRSPath), 0o750))
	err := ValidateConfigurationRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source-crs")
}

func TestValidateConfigurationRoot_MissingCRSPath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, SourceCRSPath), 0o750))
	err := ValidateConfigurationRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CRSPath)
}

func TestValidateConfigurationRoot_Success(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, SourceCRSPath), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, CRSPath), 0o750))
	require.NoError(t, ValidateConfigurationRoot(root))
}

func TestDownloadURL_EmptyURL(t *testing.T) {
	dir := t.TempDir()
	_, err := DownloadURL("", dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL is required")
}

func TestDownloadURL_UnsupportedScheme(t *testing.T) {
	dir := t.TempDir()
	_, err := DownloadURL("ftp://example.com/archive.zip", dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported scheme")
}

func TestSessionDir_CreatesUnderWorkDir(t *testing.T) {
	workDir := t.TempDir()
	sessionDir, err := SessionDir(workDir, "req-1")
	require.NoError(t, err)
	assert.True(t, len(sessionDir) > len(workDir))
	assert.Contains(t, sessionDir, "rds-diff-req-1")

	info, err := os.Stat(sessionDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSessionDir_UsesTempWhenEmpty(t *testing.T) {
	sessionDir, err := SessionDir("", "req-2")
	require.NoError(t, err)
	defer os.RemoveAll(sessionDir)
	assert.NotEmpty(t, sessionDir)
	assert.Contains(t, sessionDir, "rds-diff-req-2")
}
