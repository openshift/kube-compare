// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GitHubTreeURL holds parsed parts of a GitHub tree URL.
// Format: https://github.com/<owner>/<repo>/tree/<branch>/<path>
type GitHubTreeURL struct {
	Owner  string
	Repo   string
	Branch string
	Path   string // e.g. "telco-ran/configuration"
}

// DefaultDownloadTimeout is the HTTP client timeout for fetching archives.
const DefaultDownloadTimeout = 5 * time.Minute

// MaxExtractedFileSize is the maximum size in bytes for a single extracted file (500 MB).
const MaxExtractedFileSize = 500 * 1024 * 1024

// safeJoinPath joins destDir and name and returns an error if the result escapes destDir (zip-slip prevention).
func safeJoinPath(destDir, name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "" || clean == "." {
		return destDir, nil
	}
	if filepath.IsAbs(clean) || strings.Contains(clean, "..") {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	joined := filepath.Join(destDir, filepath.FromSlash(clean))
	rel, err := filepath.Rel(destDir, joined)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes destination: %s", name)
	}
	return joined, nil
}

// ParseGitHubTreeURL parses a full GitHub tree URL and returns owner, repo, branch, path.
// Path may be empty if URL points to repo root. Returns error for non-GitHub or malformed URLs.
//
// Format: https://github.com/<owner>/<repo>/tree/<branch>/<path>
func ParseGitHubTreeURL(raw string) (*GitHubTreeURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" || u.Host != "github.com" {
		return nil, errors.New("only https://github.com/.../tree/<branch>/<path> URLs are supported")
	}
	path := strings.Trim(u.Path, "/")
	parts := strings.SplitN(path, "/", 6)
	// need at least: owner, repo, "tree", branch (4 segments)
	if len(parts) < 4 || parts[2] != "tree" {
		return nil, errors.New("URL must match https://github.com/<owner>/<repo>/tree/<branch>/<path>")
	}
	owner, repo, branch := parts[0], parts[1], parts[3]
	subPath := ""
	if len(parts) > 4 {
		subPath = strings.Join(parts[4:], "/")
	}
	return &GitHubTreeURL{
		Owner:  owner,
		Repo:   repo,
		Branch: branch,
		Path:   strings.TrimSuffix(subPath, "/"),
	}, nil
}

// ArchiveURL returns the GitHub archive URL for this tree (branch only; path is applied after extract).
func (g *GitHubTreeURL) ArchiveURL() string {
	return fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", g.Owner, g.Repo, g.Branch)
}

// TopLevelDir is the single top-level directory inside the zip (GitHub uses <repo>-<branch>).
func (g *GitHubTreeURL) TopLevelDir() string {
	return g.Repo + "-" + g.Branch
}

// EffectiveRoot returns the path inside the extracted archive that is the configuration root.
// Empty Path means repo root, so EffectiveRoot == TopLevelDir.
func (g *GitHubTreeURL) EffectiveRoot(extractDir string) string {
	if g.Path == "" {
		return filepath.Join(extractDir, g.TopLevelDir())
	}
	return filepath.Join(extractDir, g.TopLevelDir(), filepath.FromSlash(g.Path))
}

// DownloadGitHubTree downloads the GitHub archive for the given tree URL, extracts it to destDir,
// and returns the effective root path (extractDir/topLevel/path). destDir must exist.
func DownloadGitHubTree(treeURL, destDir string, client *http.Client) (string, error) {
	parsed, err := ParseGitHubTreeURL(treeURL)
	if err != nil {
		return "", fmt.Errorf("parse github tree URL: %w", err)
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultDownloadTimeout}
	}
	archiveURL := parsed.ArchiveURL()
	resp, err := client.Get(archiveURL) // nolint:noctx
	if err != nil {
		return "", fmt.Errorf("download %s: %w", archiveURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %s", archiveURL, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read archive: %w", err)
	}
	if err := extractZipBytes(data, destDir); err != nil {
		return "", fmt.Errorf("extract zip: %w", err)
	}
	return parsed.EffectiveRoot(destDir), nil
}

// downloadDirectURL fetches a non-GitHub URL and extracts zip or tar.gz into destDir.
func downloadDirectURL(rawURL, destDir string, client *http.Client) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("unsupported scheme %q (use https or http)", u.Scheme)
	}
	resp, err := client.Get(rawURL) // nolint:noctx
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("download returned empty body")
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	pathLower := strings.ToLower(u.Path)
	isZip := strings.Contains(ct, "zip") || strings.HasSuffix(pathLower, ".zip")
	isGzip := strings.Contains(ct, "gzip") || strings.HasSuffix(pathLower, ".gz") || strings.HasSuffix(pathLower, ".tgz")

	if isZip {
		effectiveRoot, err := extractZip(data, destDir)
		if err != nil {
			return "", fmt.Errorf("extract zip: %w", err)
		}
		return effectiveRoot, nil
	}
	if isGzip || (len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b) {
		effectiveRoot, err := extractTarGz(data, destDir)
		if err != nil {
			return "", fmt.Errorf("extract tar.gz: %w", err)
		}
		return effectiveRoot, nil
	}
	if effectiveRoot, err := extractZip(data, destDir); err == nil {
		return effectiveRoot, nil
	}
	if effectiveRoot, err := extractTarGz(data, destDir); err == nil {
		return effectiveRoot, nil
	}
	return "", fmt.Errorf("unsupported archive format (expected zip or tar.gz); Content-Type: %q", ct)
}

// DownloadURL downloads from any supported URL and extracts the archive to destDir.
// Returns the effective root path where extracted content lives (single top-level dir if present, else destDir).
// Supports:
//   - GitHub tree URLs: https://github.com/<owner>/<repo>/tree/<branch>/<path> (fetches GitHub archive zip).
//   - Direct URLs to a .zip or .tar.gz/.tgz archive; the response body is extracted as-is.
//
// destDir must exist. If the download or extraction fails, returns an error.
func DownloadURL(rawURL, destDir string, client *http.Client) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("URL is required")
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultDownloadTimeout}
	}
	if _, parseErr := ParseGitHubTreeURL(rawURL); parseErr == nil {
		return DownloadGitHubTree(rawURL, destDir, client)
	}
	return downloadDirectURL(rawURL, destDir, client)
}

// extractZipBytes extracts zip data into destDir without tracking top-level dirs.
func extractZipBytes(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	destDirClean := filepath.Clean(destDir)
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			name = strings.TrimSuffix(name, "/")
			if name == "" {
				continue
			}
			dstPath, err := safeJoinPath(destDirClean, name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dstPath, 0o750); err != nil {
				return fmt.Errorf("mkdir %s: %w", dstPath, err)
			}
			continue
		}
		if err := extractZipFile(f, destDirClean); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, destDirClean string) error {
	dstPath, err := safeJoinPath(destDirClean, f.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", dstPath, err)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()
	dst, err := os.Create(dstPath) // #nosec G304 -- dstPath from safeJoinPath
	if err != nil {
		return fmt.Errorf("create %s: %w", dstPath, err)
	}
	defer dst.Close()
	if _, err = io.Copy(dst, io.LimitReader(rc, MaxExtractedFileSize)); err != nil { // #nosec G110 -- size bounded by MaxExtractedFileSize
		return fmt.Errorf("copy %s: %w", f.Name, err)
	}
	return nil
}

// extractZip extracts zip data into destDir and returns the effective root
// (single top-level directory if present, else destDir).
func extractZip(data []byte, destDir string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	destDirClean := filepath.Clean(destDir)
	topLevel := detectZipTopLevel(zr)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			name := strings.TrimSuffix(f.Name, "/")
			if name == "" {
				continue
			}
			dstPath, err := safeJoinPath(destDirClean, name)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(dstPath, 0o750); err != nil {
				return "", fmt.Errorf("mkdir %s: %w", dstPath, err)
			}
			continue
		}
		if err := extractZipFile(f, destDirClean); err != nil {
			return "", err
		}
	}
	if topLevel != "" {
		return filepath.Join(destDir, topLevel), nil
	}
	return destDir, nil
}

// detectZipTopLevel returns the single top-level directory name if all entries share one,
// or empty string if there are multiple top-level entries.
func detectZipTopLevel(zr *zip.Reader) string {
	var topLevel string
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			name = strings.TrimSuffix(name, "/")
			if name == "" {
				continue
			}
		}
		first := strings.SplitN(name, "/", 2)[0]
		if topLevel == "" {
			topLevel = first
		} else if topLevel != first {
			return ""
		}
	}
	return topLevel
}

// extractTarGz extracts gzip-compressed tar data into destDir and returns the effective root.
func extractTarGz(data []byte, destDir string) (string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()
	destDirClean := filepath.Clean(destDir)
	tr := tar.NewReader(gr)
	var topLevel string
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}
		name := strings.TrimPrefix(filepath.Clean(header.Name), ".")
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}
		dstPath, err := safeJoinPath(destDirClean, name)
		if err != nil {
			return "", err
		}
		first := strings.SplitN(name, "/", 2)[0]
		if topLevel == "" {
			topLevel = first
		} else if topLevel != first {
			topLevel = ""
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dstPath, 0o750); err != nil {
				return "", fmt.Errorf("mkdir %s: %w", dstPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
				return "", fmt.Errorf("mkdir parent of %s: %w", dstPath, err)
			}
			outFile, err := os.Create(dstPath) // #nosec G304 -- dstPath from safeJoinPath
			if err != nil {
				return "", fmt.Errorf("create %s: %w", dstPath, err)
			}
			if _, err = io.Copy(outFile, io.LimitReader(tr, MaxExtractedFileSize)); err != nil { // #nosec G110 -- size bounded by MaxExtractedFileSize
				outFile.Close()
				return "", fmt.Errorf("copy %s: %w", name, err)
			}
			outFile.Close()
		default:
			// skip symlinks, etc.
		}
	}
	if topLevel != "" {
		return filepath.Join(destDir, topLevel), nil
	}
	return destDir, nil
}

// ValidateConfigurationRoot checks that source-crs and CRSPath exist under root.
func ValidateConfigurationRoot(root string) error {
	sourceCRS := filepath.Join(root, SourceCRSPath)
	crsPath := filepath.Join(root, CRSPath)
	if _, err := os.Stat(sourceCRS); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration root missing %s (path: %s)", SourceCRSPath, sourceCRS)
		}
		return fmt.Errorf("stat %s: %w", sourceCRS, err)
	}
	if _, err := os.Stat(crsPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration root missing %s (path: %s)", CRSPath, crsPath)
		}
		return fmt.Errorf("stat %s: %w", crsPath, err)
	}
	return nil
}
