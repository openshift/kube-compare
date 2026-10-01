package compare

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	gcrlogs "github.com/google/go-containerregistry/pkg/logs"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const credentialHelperModeEnv = "KUBE_COMPARE_TEST_CREDENTIAL_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(credentialHelperModeEnv); mode != "" {
		runCredentialHelperProcess(mode)
		return
	}
	os.Exit(m.Run())
}

func runCredentialHelperProcess(mode string) {
	_, _ = io.ReadAll(os.Stdin)
	switch mode {
	case "success":
		_, _ = fmt.Fprint(os.Stdout, `{"ServerURL":"example.test","Username":"helper-user","Secret":"helper-password"}`)
	case "token":
		_, _ = fmt.Fprint(os.Stdout, `{"ServerURL":"example.test","Username":"<token>","Secret":"identity-token"}`)
	case "not-found":
		_, _ = fmt.Fprint(os.Stdout, "credentials not found in native keychain")
		os.Exit(1)
	case "oversized-not-found":
		_, _ = fmt.Fprint(os.Stdout, "credentials not found in native keychain"+strings.Repeat(" ", 1<<20))
		os.Exit(1)
	case "failure":
		_, _ = fmt.Fprint(os.Stdout, "stdout-secret-sentinel")
		_, _ = fmt.Fprint(os.Stderr, "stderr-secret-sentinel")
		os.Exit(1)
	case "malformed":
		_, _ = fmt.Fprint(os.Stdout, "malformed-secret-sentinel")
	case "oversized":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("oversized-secret-sentinel", 100_000))
	case "hang":
		if marker := os.Getenv("KUBE_COMPARE_HELPER_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0o600)
		}
		for {
			time.Sleep(time.Second)
		}
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func TestIsContainer(t *testing.T) {
	tests := map[string]bool{
		"container://my-image:latest:/metadata.yaml": true,
		"container://name":                           true,
		"container://":                               true,
		"file://local/path":                          false,
		"https://example.test/ref":                   false,
		"container:/one-slash":                       false,
		"":                                           false,
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, expected, isContainer(input))
		})
	}
}

func TestParsePath(t *testing.T) {
	digest := strings.Repeat("a", 64)
	tests := []struct {
		name         string
		input        string
		image        string
		metadataPath string
		wantError    bool
	}{
		{name: "documented", input: "container://quay.io/example/ref:v1:/reference/metadata.yaml", image: "quay.io/example/ref:v1", metadataPath: "/reference/metadata.yaml"},
		{name: "registry port", input: "container://registry.example:5000/example/ref:v1:/ref/metadata.yaml", image: "registry.example:5000/example/ref:v1", metadataPath: "/ref/metadata.yaml"},
		{name: "digest", input: "container://quay.io/example/ref@sha256:" + digest + ":/metadata.yaml", image: "quay.io/example/ref@sha256:" + digest, metadataPath: "/metadata.yaml"},
		{name: "tag and digest", input: "container://quay.io/example/ref:v1@sha256:" + digest + ":/ref/metadata.yaml", image: "quay.io/example/ref@sha256:" + digest, metadataPath: "/ref/metadata.yaml"},
		{name: "tagless", input: "container://example/ref:/ref/metadata.yaml", image: "index.docker.io/example/ref:latest", metadataPath: "/ref/metadata.yaml"},
		{name: "ipv6", input: "container://[2001:db8::1]:5000/ref:v1:/ref/metadata.yaml", image: "[2001:db8::1]:5000/ref:v1", metadataPath: "/ref/metadata.yaml"},
		{name: "missing scheme", input: "quay.io/example/ref:v1:/ref/metadata.yaml", wantError: true},
		{name: "missing path", input: "container://quay.io/example/ref:v1", wantError: true},
		{name: "relative path", input: "container://quay.io/example/ref:v1:metadata.yaml", wantError: true},
		{name: "directory path", input: "container://quay.io/example/ref:v1:/ref/", wantError: true},
		{name: "traversal", input: "container://quay.io/example/ref:v1:/ref/../metadata.yaml", wantError: true},
		{name: "backslash", input: `container://quay.io/example/ref:v1:/ref\metadata.yaml`, wantError: true},
		{name: "nul", input: "container://quay.io/example/ref:v1:/ref/meta\x00data.yaml", wantError: true},
		{name: "path colon", input: "container://quay.io/example/ref:v1:/ref/meta:data.yaml", wantError: true},
		{name: "http image", input: "container://https://quay.io/example/ref:v1:/ref/metadata.yaml", wantError: true},
		{name: "repeated colons", input: "container://example/ref:::v1::::/ref/metadata.yaml", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parsePath(test.input)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.image, parsed.image.Name())
			assert.Equal(t, test.metadataPath, parsed.metadataPath)
		})
	}
}

func TestParsePathRedactsMalformedInput(t *testing.T) {
	const secret = "parse-secret-sentinel"
	_, err := parsePath("container://user:" + secret + "@registry.example/ref:/metadata.yaml")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, err.Error(), "user:")
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type anonymousKeychain struct{}

func (anonymousKeychain) Resolve(authn.Resource) (authn.Authenticator, error) {
	return authn.Anonymous, nil
}

func TestHTTPSOnlyTransportRejectsBeforeDelegate(t *testing.T) {
	delegateCalls := 0
	transport := httpsOnlyTransport{delegate: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		delegateCalls++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
	})}

	for _, rawURL := range []string{"http://registry.example/v2/", "ftp://registry.example/layer", "/relative"} {
		request, err := http.NewRequest(http.MethodGet, rawURL, nil)
		require.NoError(t, err)
		_, err = transport.RoundTrip(request)
		require.Error(t, err)
	}
	assert.Zero(t, delegateCalls)

	request, err := http.NewRequest(http.MethodGet, "https://registry.example/v2/", nil)
	require.NoError(t, err)
	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	assert.Equal(t, 1, delegateCalls)
}

func TestPullRemoteImageUsesTLSForLocalhost(t *testing.T) {
	server := httptest.NewTLSServer(registry.New())
	t.Cleanup(server.Close)

	reference, err := name.NewTag(strings.TrimPrefix(server.URL, "https://") + "/team/reference:test")
	require.NoError(t, err)
	transport := httpsOnlyTransport{delegate: server.Client().Transport}
	require.NoError(t, remote.Write(reference, empty.Image, remote.WithTransport(transport)))

	pulled, err := pullRemoteImageWithTransport(
		context.Background(),
		reference,
		anonymousKeychain{},
		v1.Platform{OS: "linux", Architecture: runtime.GOARCH},
		server.Client().Transport,
	)
	require.NoError(t, err)
	wantDigest, err := empty.Image.Digest()
	require.NoError(t, err)
	gotDigest, err := pulled.Digest()
	require.NoError(t, err)
	assert.Equal(t, wantDigest, gotDigest)
}

func TestNormalizeArchivePathPortable(t *testing.T) {
	limits := testLimits()
	tests := []struct {
		name      string
		input     string
		typeflag  byte
		expected  string
		wantError bool
	}{
		{name: "ordinary", input: "ref/metadata.yaml", expected: "ref/metadata.yaml"},
		{name: "single root slash", input: "/ref/metadata.yaml", expected: "ref/metadata.yaml"},
		{name: "directory trailing slash", input: "ref/templates/", typeflag: tar.TypeDir, expected: "ref/templates"},
		{name: "rooted directory trailing slash", input: "/ref/templates/", typeflag: tar.TypeDir, expected: "ref/templates"},
		{name: "regular trailing slash", input: "ref/metadata.yaml/", wantError: true},
		{name: "directory double trailing slash", input: "ref/templates//", typeflag: tar.TypeDir, wantError: true},
		{name: "directory interior empty component", input: "ref//templates/", typeflag: tar.TypeDir, wantError: true},
		{name: "dot prefix", input: "./ref/metadata.yaml", wantError: true},
		{name: "empty component", input: "ref//metadata.yaml", wantError: true},
		{name: "traversal", input: "../metadata.yaml", wantError: true},
		{name: "embedded traversal", input: "ref/../metadata.yaml", wantError: true},
		{name: "double slash root", input: "//server/share", wantError: true},
		{name: "backslash", input: `ref\metadata.yaml`, wantError: true},
		{name: "leading slash drive", input: "/C:/ref/metadata.yaml", wantError: true},
		{name: "alternate stream", input: "ref/metadata.yaml:stream", wantError: true},
		{name: "nul", input: "ref/meta\x00data.yaml", wantError: true},
		{name: "reserved", input: "ref/NUL", wantError: true},
		{name: "reserved extension", input: "ref/con.txt", wantError: true},
		{name: "reserved case", input: "ref/Com1.yaml", wantError: true},
		{name: "trailing dot", input: "ref/name.", wantError: true},
		{name: "trailing space", input: "ref/name ", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := normalizeArchivePath(test.input, test.typeflag, limits)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestNormalizeArchivePathLengthLimits(t *testing.T) {
	limits := testLimits()
	limits.maxPathBytes = 8
	_, err := normalizeArchivePath("ref/metadata.yaml", tar.TypeReg, limits)
	require.Error(t, err)

	limits = testLimits()
	limits.maxPathComponentBytes = 3
	_, err = normalizeArchivePath("ref/metadata.yaml", tar.TypeReg, limits)
	require.Error(t, err)
}

type tarEntry struct {
	name         string
	contents     string
	typeflag     byte
	linkname     string
	mode         int64
	pax          map[string]string
	preserveZero bool
}

func makeTar(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 && !entry.preserveZero {
			typeflag = tar.TypeReg
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o777
		}
		header := &tar.Header{
			Name:       entry.name,
			Typeflag:   typeflag,
			Linkname:   entry.linkname,
			Mode:       mode,
			Size:       int64(len(entry.contents)),
			PAXRecords: entry.pax,
		}
		if typeflag == tar.TypeDir || typeflag == tar.TypeSymlink || typeflag == tar.TypeLink ||
			typeflag == tar.TypeChar || typeflag == tar.TypeBlock || typeflag == tar.TypeFifo {
			header.Size = 0
		}
		require.NoError(t, writer.WriteHeader(header))
		if header.Size > 0 {
			_, err := io.WriteString(writer, entry.contents)
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func gzipBytes(t *testing.T, input []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	_, err := writer.Write(input)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func zstdBytes(t *testing.T, input []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer, err := zstd.NewWriter(&buffer, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	_, err = writer.Write(input)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func imageFromLayers(t *testing.T, layers ...v1.Layer) v1.Image {
	t.Helper()
	image, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)
	return image
}

func uncompressedLayer(t *testing.T, entries ...tarEntry) v1.Layer {
	t.Helper()
	return static.NewLayer(makeTar(t, entries...), types.OCIUncompressedLayer)
}

func testLimits() extractionLimits {
	return extractionLimits{
		maxLayers:                 32,
		maxDeclaredCompressed:     8 << 20,
		maxActualCompressed:       8 << 20,
		maxUncompressed:           8 << 20,
		maxRawHeaders:             1_000,
		maxStateEntries:           1_000,
		maxStateKeyBytes:          1 << 20,
		maxSelectedEntries:        100,
		maxSelectedNodes:          1_000,
		maxSelectedKeyBytes:       1 << 20,
		maxFileBytes:              1 << 20,
		maxSelectedBytes:          2 << 20,
		maxPathBytes:              4_096,
		maxPathComponentBytes:     255,
		maxZstdWindowBytes:        64 << 20,
		maxCredentialHelperOutput: 1 << 20,
	}
}

func testSelectedState(t *testing.T, limits extractionLimits) *selectedState {
	t.Helper()
	selected := &selectedState{
		nodes: make(map[string]extractedNode),
		nodeBudget: stateBudget{
			maxEntries:  limits.maxSelectedNodes,
			maxBytes:    limits.maxSelectedKeyBytes,
			entryErr:    errSelectedNodeLimit,
			keyBytesErr: errSelectedKeyBytesLimit,
		},
	}
	require.NoError(t, selected.nodeBudget.retain("."))
	selected.nodes["."] = extractedNode{directory: true, explicit: true}
	return selected
}

func TestApplyImageLayersWhiteoutsAndModes(t *testing.T) {
	lower := uncompressedLayer(t,
		tarEntry{name: "ref/metadata.yaml", contents: "old", mode: 0o777},
		tarEntry{name: "ref/replaced.yaml", contents: "lower"},
		tarEntry{name: "ref/deleted.yaml", contents: "remove"},
		tarEntry{name: "ref/opaque/lower.yaml", contents: "hidden"},
	)
	upper := uncompressedLayer(t,
		tarEntry{name: "ref/metadata.yaml", contents: "new", mode: 0o755},
		tarEntry{name: "ref/replaced.yaml", contents: "upper"},
		tarEntry{name: "ref/.wh.deleted.yaml"},
		tarEntry{name: "ref/opaque/.wh..wh..opq"},
		tarEntry{name: "ref/opaque/upper.yaml", contents: "visible"},
	)
	staging := t.TempDir()
	require.NoError(t, applyImageLayers(context.Background(), imageFromLayers(t, lower, upper), staging, "/ref/metadata.yaml", testLimits()))

	metadata, err := os.ReadFile(filepath.Join(staging, "metadata.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(metadata))
	replaced, err := os.ReadFile(filepath.Join(staging, "replaced.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "upper", string(replaced))
	_, err = os.Stat(filepath.Join(staging, "deleted.yaml"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(staging, "opaque", "lower.yaml"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	upperContents, err := os.ReadFile(filepath.Join(staging, "opaque", "upper.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "visible", string(upperContents))

	fileInfo, err := os.Stat(filepath.Join(staging, "metadata.yaml"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), fileInfo.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Join(staging, "opaque"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestApplyImageLayersRootMetadataAndTypeRegA(t *testing.T) {
	layer := uncompressedLayer(t,
		tarEntry{name: "metadata.yaml", contents: "root", preserveZero: true},
		tarEntry{name: "template.yaml", contents: "template"},
	)
	staging := t.TempDir()
	require.NoError(t, applyImageLayers(context.Background(), imageFromLayers(t, layer), staging, "/metadata.yaml", testLimits()))
	contents, err := os.ReadFile(filepath.Join(staging, "metadata.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "root", string(contents))
}

func TestApplyImageLayersAcceptsConventionalDirectoryHeaders(t *testing.T) {
	layer := uncompressedLayer(t,
		tarEntry{name: "etc/", typeflag: tar.TypeDir},
		tarEntry{name: "ref/", typeflag: tar.TypeDir},
		tarEntry{name: "ref/templates/", typeflag: tar.TypeDir},
		tarEntry{name: "ref/templates/example.yaml", contents: "template"},
		tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
	)
	staging := t.TempDir()
	require.NoError(t, applyImageLayers(context.Background(), imageFromLayers(t, layer), staging, "/ref/metadata.yaml", testLimits()))
	info, err := os.Stat(filepath.Join(staging, "templates"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestApplyImageLayersSelectedTypePolicy(t *testing.T) {
	tests := []tarEntry{
		{name: "ref/metadata.yaml", typeflag: tar.TypeSymlink, linkname: "target"},
		{name: "ref/metadata.yaml", typeflag: tar.TypeSymlink, linkname: "../../escape"},
		{name: "ref/metadata.yaml", typeflag: tar.TypeSymlink, linkname: "/absolute"},
		{name: "ref/metadata.yaml", typeflag: tar.TypeLink, linkname: "ref/target"},
		{name: "ref/metadata.yaml", typeflag: tar.TypeLink, linkname: "../../escape"},
		{name: "ref/metadata.yaml", typeflag: tar.TypeChar},
		{name: "ref/metadata.yaml", typeflag: tar.TypeBlock},
		{name: "ref/metadata.yaml", typeflag: tar.TypeFifo},
		{name: "ref/metadata.yaml", typeflag: 'V'},
		{name: "ref/metadata.yaml", contents: "x", pax: map[string]string{"SCHILY.realsize": "10"}},
	}
	for _, entry := range tests {
		t.Run(fmt.Sprintf("type-%d-link-%s", entry.typeflag, entry.linkname), func(t *testing.T) {
			staging := t.TempDir()
			err := applyImageLayers(context.Background(), imageFromLayers(t, uncompressedLayer(t, entry)), staging, "/ref/metadata.yaml", testLimits())
			require.Error(t, err)
			_, statErr := os.Stat(filepath.Join(staging, "metadata.yaml"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestApplyImageLayersIgnoresOutsideSpecialEntries(t *testing.T) {
	layer := uncompressedLayer(t,
		tarEntry{name: "outside/link", typeflag: tar.TypeSymlink, linkname: "/absolute"},
		tarEntry{name: "outside/device", typeflag: tar.TypeChar},
		tarEntry{name: "ref/metadata.yaml", contents: "ok"},
	)
	require.NoError(t, applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits()))
}

func TestApplyImageLayersRejectsInvalidWhiteoutTargets(t *testing.T) {
	for _, entryName := range []string{"ref/.wh.", "ref/.wh..", "ref/.wh..."} {
		t.Run(entryName, func(t *testing.T) {
			err := applyImageLayers(context.Background(), imageFromLayers(t, uncompressedLayer(t,
				tarEntry{name: entryName},
				tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
			)), t.TempDir(), "/ref/metadata.yaml", testLimits())
			require.Error(t, err)
		})
	}
}

func TestApplyImageLayersRejectsConflicts(t *testing.T) {
	layer := uncompressedLayer(t,
		tarEntry{name: "ref/dir/child.yaml", contents: "child"},
		tarEntry{name: "ref/dir", contents: "file"},
		tarEntry{name: "ref/metadata.yaml", contents: "ok"},
	)
	err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting")
}

func TestApplyImageLayersSuppressesDuplicateExplicitDirectory(t *testing.T) {
	layer := uncompressedLayer(t,
		tarEntry{name: "ref/dir", typeflag: tar.TypeDir},
		tarEntry{name: "ref/dir", typeflag: tar.TypeDir},
		tarEntry{name: "ref/metadata.yaml", contents: "ok"},
	)
	require.NoError(t, applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits()))
}

func TestApplyImageLayersRawBudgetsIncludeHiddenEntries(t *testing.T) {
	t.Run("raw headers hidden by whiteout", func(t *testing.T) {
		lower := uncompressedLayer(t,
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
			tarEntry{name: "ref/a", contents: "a"},
			tarEntry{name: "ref/b", contents: "b"},
		)
		upper := uncompressedLayer(t,
			tarEntry{name: "ref/.wh.a"},
			tarEntry{name: "ref/.wh.b"},
		)
		limits := testLimits()
		limits.maxRawHeaders = 3
		err := applyImageLayers(context.Background(), imageFromLayers(t, lower, upper), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errRawHeaderLimit)
	})

	t.Run("uncompressed bomb hidden by opaque whiteout", func(t *testing.T) {
		lowerTar := makeTar(t,
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
			tarEntry{name: "ref/hidden/bomb", contents: strings.Repeat("x", 32<<10)},
		)
		upperTar := makeTar(t,
			tarEntry{name: "ref/hidden/.wh..wh..opq"},
			tarEntry{name: "ref/metadata.yaml", contents: "upper"},
		)
		lower := static.NewLayer(gzipBytes(t, lowerTar), types.OCILayer)
		upper := static.NewLayer(gzipBytes(t, upperTar), types.OCILayer)
		limits := testLimits()
		limits.maxUncompressed = int64(len(upperTar) + 4_096)
		err := applyImageLayers(context.Background(), imageFromLayers(t, lower, upper), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errUncompressedLimit)
	})

	for testName, upperEntries := range map[string][]tarEntry{
		"replacement": {
			{name: "ref/hidden", contents: "replacement"},
			{name: "ref/metadata.yaml", contents: "upper"},
		},
		"normal whiteout": {
			{name: "ref/hidden/.wh.a"},
			{name: "ref/hidden/.wh.b"},
			{name: "ref/metadata.yaml", contents: "upper"},
		},
		"opaque whiteout": {
			{name: "ref/hidden/.wh..wh..opq"},
			{name: "ref/metadata.yaml", contents: "upper"},
		},
	} {
		t.Run("raw headers hidden by "+testName, func(t *testing.T) {
			lower := uncompressedLayer(t,
				tarEntry{name: "ref/hidden/a", contents: "a"},
				tarEntry{name: "ref/hidden/b", contents: "b"},
			)
			upper := uncompressedLayer(t, upperEntries...)
			limits := testLimits()
			limits.maxRawHeaders = len(upperEntries) + 1
			err := applyImageLayers(context.Background(), imageFromLayers(t, lower, upper), t.TempDir(), "/ref/metadata.yaml", limits)
			assert.ErrorIs(t, err, errRawHeaderLimit)
		})
	}

	t.Run("state entries", func(t *testing.T) {
		layer := uncompressedLayer(t,
			tarEntry{name: "outside/a", contents: "a"},
			tarEntry{name: "outside/b", contents: "b"},
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
		)
		limits := testLimits()
		limits.maxStateEntries = 1
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errStateEntryLimit)
	})

	t.Run("state entries from tombstones", func(t *testing.T) {
		layer := uncompressedLayer(t,
			tarEntry{name: "outside/.wh.a"},
			tarEntry{name: "outside/.wh.b"},
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
		)
		limits := testLimits()
		limits.maxStateEntries = 1
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errStateEntryLimit)
	})

	t.Run("state entries from opaque directories", func(t *testing.T) {
		layer := uncompressedLayer(t,
			tarEntry{name: "outside/a/.wh..wh..opq"},
			tarEntry{name: "outside/b/.wh..wh..opq"},
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
		)
		limits := testLimits()
		limits.maxStateEntries = 1
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errStateEntryLimit)
	})

	t.Run("state key bytes", func(t *testing.T) {
		layer := uncompressedLayer(t,
			tarEntry{name: "outside/long-name", contents: "a"},
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
		)
		limits := testLimits()
		limits.maxStateKeyBytes = 4
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errStateKeyBytesLimit)
	})
}

func TestApplyImageLayersBudgetBoundaries(t *testing.T) {
	archive := makeTar(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})
	layer := static.NewLayer(archive, types.OCIUncompressedLayer)
	image := imageFromLayers(t, layer)
	stateKeyBytes := int64(len("ref/metadata.yaml"))

	exact := testLimits()
	exact.maxDeclaredCompressed = int64(len(archive))
	exact.maxActualCompressed = int64(len(archive))
	exact.maxUncompressed = int64(len(archive))
	exact.maxRawHeaders = 1
	exact.maxStateEntries = 1
	exact.maxStateKeyBytes = stateKeyBytes
	exact.maxSelectedEntries = 1
	exact.maxSelectedNodes = 2
	exact.maxSelectedKeyBytes = int64(len(".") + len("metadata.yaml"))
	require.NoError(t, applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", exact))

	tests := []struct {
		name      string
		configure func(*extractionLimits)
		want      error
	}{
		{name: "declared compressed", configure: func(limits *extractionLimits) { limits.maxDeclaredCompressed-- }},
		{name: "actual compressed", configure: func(limits *extractionLimits) { limits.maxActualCompressed-- }, want: errActualCompressedLimit},
		{name: "uncompressed", configure: func(limits *extractionLimits) { limits.maxUncompressed-- }, want: errUncompressedLimit},
		{name: "raw headers", configure: func(limits *extractionLimits) { limits.maxRawHeaders-- }, want: errRawHeaderLimit},
		{name: "state entries", configure: func(limits *extractionLimits) { limits.maxStateEntries-- }, want: errStateEntryLimit},
		{name: "state key bytes", configure: func(limits *extractionLimits) { limits.maxStateKeyBytes-- }, want: errStateKeyBytesLimit},
		{name: "selected entries", configure: func(limits *extractionLimits) { limits.maxSelectedEntries-- }},
		{name: "selected nodes", configure: func(limits *extractionLimits) { limits.maxSelectedNodes-- }, want: errSelectedNodeLimit},
		{name: "selected key bytes", configure: func(limits *extractionLimits) { limits.maxSelectedKeyBytes-- }, want: errSelectedKeyBytesLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits := exact
			test.configure(&limits)
			err := applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", limits)
			require.Error(t, err)
			if test.want != nil {
				assert.ErrorIs(t, err, test.want)
			}
		})
	}
}

func TestApplyImageLayersSharedAggregateBudgets(t *testing.T) {
	upperArchive := makeTar(t, tarEntry{name: "ref/metadata.yaml", contents: "upper"})
	lowerArchive := makeTar(t, tarEntry{name: "outside/lower", contents: "lower"})
	image := imageFromLayers(t,
		static.NewLayer(lowerArchive, types.OCIUncompressedLayer),
		static.NewLayer(upperArchive, types.OCIUncompressedLayer),
	)
	total := int64(len(upperArchive) + len(lowerArchive))

	t.Run("actual compressed", func(t *testing.T) {
		limits := testLimits()
		limits.maxActualCompressed = total - 1
		err := applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errActualCompressedLimit)
	})

	t.Run("uncompressed", func(t *testing.T) {
		limits := testLimits()
		limits.maxUncompressed = total - 1
		err := applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errUncompressedLimit)
	})
}

type manifestOverrideImage struct {
	v1.Image
	alter func(*v1.Manifest)
}

func (image manifestOverrideImage) Manifest() (*v1.Manifest, error) {
	manifest, err := image.Image.Manifest()
	if err != nil {
		return nil, fmt.Errorf("reading wrapped manifest: %w", err)
	}
	copyManifest := *manifest
	copyManifest.Layers = append([]v1.Descriptor(nil), manifest.Layers...)
	image.alter(&copyManifest)
	return &copyManifest, nil
}

func TestApplyImageLayersRejectsDishonestDescriptorAndManifestLimits(t *testing.T) {
	layer := uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: strings.Repeat("x", 1_024)})
	base := imageFromLayers(t, layer)
	dishonest := manifestOverrideImage{Image: base, alter: func(manifest *v1.Manifest) {
		manifest.Layers[0].Size = 1
	}}
	limits := testLimits()
	limits.maxDeclaredCompressed = 10
	limits.maxActualCompressed = 100
	err := applyImageLayers(context.Background(), dishonest, t.TempDir(), "/ref/metadata.yaml", limits)
	assert.ErrorIs(t, err, errActualCompressedLimit)

	negative := manifestOverrideImage{Image: base, alter: func(manifest *v1.Manifest) {
		manifest.Layers[0].Size = -1
	}}
	err = applyImageLayers(context.Background(), negative, t.TempDir(), "/ref/metadata.yaml", testLimits())
	assert.Contains(t, err.Error(), "declared compressed")

	overflow := imageFromLayers(t, layer, layer)
	overflowImage := manifestOverrideImage{Image: overflow, alter: func(manifest *v1.Manifest) {
		manifest.Layers[0].Size = math.MaxInt64
		manifest.Layers[1].Size = 1
	}}
	limits = testLimits()
	limits.maxDeclaredCompressed = math.MaxInt64
	err = applyImageLayers(context.Background(), overflowImage, t.TempDir(), "/ref/metadata.yaml", limits)
	assert.Contains(t, err.Error(), "declared compressed")
}

func TestApplyImageLayersLayerCountLimit(t *testing.T) {
	layer := uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: "ok"})
	limits := testLimits()
	limits.maxLayers = 1
	err := applyImageLayers(context.Background(), imageFromLayers(t, layer, layer), t.TempDir(), "/ref/metadata.yaml", limits)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "layers")
}

type errorAfterReader struct {
	reader io.Reader
	err    error
	done   bool
}

func (reader *errorAfterReader) Read(data []byte) (int, error) {
	n, err := reader.reader.Read(data)
	if n > 0 {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}
		if err != nil {
			return n, fmt.Errorf("reading wrapped test stream: %w", err)
		}
		return n, nil
	}
	if !reader.done {
		reader.done = true
		return 0, reader.err
	}
	return 0, io.EOF
}

func (reader *errorAfterReader) Close() error { return nil }

type compressedErrorLayer struct {
	v1.Layer
	err error
}

type closeErrorReadCloser struct {
	io.ReadCloser
	err error
}

func (reader *closeErrorReadCloser) Close() error {
	return errors.Join(reader.ReadCloser.Close(), reader.err)
}

type compressedCloseErrorLayer struct {
	v1.Layer
	err error
}

func (layer compressedCloseErrorLayer) Compressed() (io.ReadCloser, error) {
	contents, err := layer.Layer.Compressed()
	if err != nil {
		return nil, fmt.Errorf("opening close-error test layer: %w", err)
	}
	return &closeErrorReadCloser{ReadCloser: contents, err: layer.err}, nil
}

func (layer compressedErrorLayer) Compressed() (io.ReadCloser, error) {
	contents, err := layer.Layer.Compressed()
	if err != nil {
		return nil, fmt.Errorf("opening wrapped compressed layer: %w", err)
	}
	data, err := io.ReadAll(contents)
	if err != nil {
		return nil, fmt.Errorf("reading wrapped compressed layer: %w", err)
	}
	_ = contents.Close()
	return &errorAfterReader{reader: bytes.NewReader(data), err: layer.err}, nil
}

type layersOverrideImage struct {
	v1.Image
	layers []v1.Layer
}

func (image layersOverrideImage) Layers() ([]v1.Layer, error) { return image.layers, nil }

type cancelingReadCloser struct {
	reader io.Reader
	cancel context.CancelFunc
	closed *bool
}

func (reader *cancelingReadCloser) Read(data []byte) (int, error) {
	n, err := reader.reader.Read(data)
	reader.cancel()
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	if err != nil {
		return n, fmt.Errorf("reading canceling test stream: %w", err)
	}
	return n, nil
}

func (reader *cancelingReadCloser) Close() error {
	*reader.closed = true
	return nil
}

type cancelingLayer struct {
	v1.Layer
	cancel context.CancelFunc
	closed *bool
}

func (layer cancelingLayer) Compressed() (io.ReadCloser, error) {
	contents, err := layer.Layer.Compressed()
	if err != nil {
		return nil, fmt.Errorf("opening canceling test layer: %w", err)
	}
	data, readErr := io.ReadAll(contents)
	closeErr := contents.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	return &cancelingReadCloser{reader: bytes.NewReader(data), cancel: layer.cancel, closed: layer.closed}, nil
}

func TestApplyImageLayersSurfacesCompressedVerificationError(t *testing.T) {
	verificationErr := errors.New("digest verification failed")
	baseLayer := uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})
	baseImage := imageFromLayers(t, baseLayer)
	image := layersOverrideImage{Image: baseImage, layers: []v1.Layer{compressedErrorLayer{Layer: baseLayer, err: verificationErr}}}
	err := applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", testLimits())
	assert.ErrorIs(t, err, verificationErr)
}

func TestApplyImageLayersReportsCompressedCloseErrors(t *testing.T) {
	closeErr := errors.New("compressed close failed")
	validLayer := uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})
	validImage := imageFromLayers(t, validLayer)
	image := layersOverrideImage{
		Image:  validImage,
		layers: []v1.Layer{compressedCloseErrorLayer{Layer: validLayer, err: closeErr}},
	}
	err := applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", testLimits())
	assert.ErrorIs(t, err, closeErr)

	primaryLayer := uncompressedLayer(t, tarEntry{name: "../unsafe", contents: "bad"})
	primaryImage := imageFromLayers(t, primaryLayer)
	image = layersOverrideImage{
		Image:  primaryImage,
		layers: []v1.Layer{compressedCloseErrorLayer{Layer: primaryLayer, err: closeErr}},
	}
	err = applyImageLayers(context.Background(), image, t.TempDir(), "/ref/metadata.yaml", testLimits())
	assert.ErrorIs(t, err, errUnsafeArchivePath)
	assert.ErrorIs(t, err, closeErr)
}

func TestApplyImageLayersRejectsTruncatedTar(t *testing.T) {
	archive := makeTar(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})
	truncated := archive[:516]
	layer := static.NewLayer(truncated, types.OCIUncompressedLayer)
	err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits())
	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestApplyImageLayersBoundsZstdExpansion(t *testing.T) {
	archive := makeTar(t,
		tarEntry{name: "outside/bomb", contents: strings.Repeat("z", 32<<10)},
		tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
	)
	layer := static.NewLayer(zstdBytes(t, archive), types.OCILayerZStd)
	limits := testLimits()
	limits.maxUncompressed = 4 << 10
	err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", limits)
	assert.ErrorIs(t, err, errUncompressedLimit)
}

func TestApplyImageLayersDecoderAndMediaTypes(t *testing.T) {
	t.Run("corrupt gzip", func(t *testing.T) {
		layer := static.NewLayer([]byte("not-gzip"), types.OCILayer)
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gzip")
	})

	t.Run("corrupt zstd", func(t *testing.T) {
		layer := static.NewLayer([]byte("not-zstd"), types.OCILayerZStd)
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits())
		require.Error(t, err)
	})

	t.Run("successful zstd", func(t *testing.T) {
		archive := makeTar(t, tarEntry{name: "ref/metadata.yaml", contents: "zstd-metadata"})
		layer := static.NewLayer(zstdBytes(t, archive), types.OCILayerZStd)
		staging := t.TempDir()
		require.NoError(t, applyImageLayers(context.Background(), imageFromLayers(t, layer), staging, "/ref/metadata.yaml", testLimits()))
		contents, err := os.ReadFile(filepath.Join(staging, "metadata.yaml"))
		require.NoError(t, err)
		assert.Equal(t, "zstd-metadata", string(contents))
	})

	t.Run("unsupported", func(t *testing.T) {
		layer := static.NewLayer([]byte("content"), types.MediaType("application/x-unsupported-layer"))
		err := applyImageLayers(context.Background(), imageFromLayers(t, layer), t.TempDir(), "/ref/metadata.yaml", testLimits())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported image layer media type")
	})
}

func TestApplyImageLayersSelectedAndOutsideLimits(t *testing.T) {
	t.Run("one file", func(t *testing.T) {
		limits := testLimits()
		limits.maxFileBytes = 2
		err := applyImageLayers(context.Background(), imageFromLayers(t, uncompressedLayer(t,
			tarEntry{name: "ref/metadata.yaml", contents: "too-large"},
		)), t.TempDir(), "/ref/metadata.yaml", limits)
		require.Error(t, err)
	})

	t.Run("selected total", func(t *testing.T) {
		limits := testLimits()
		limits.maxSelectedBytes = 5
		err := applyImageLayers(context.Background(), imageFromLayers(t, uncompressedLayer(t,
			tarEntry{name: "ref/metadata.yaml", contents: "abc"},
			tarEntry{name: "ref/other.yaml", contents: "def"},
		)), t.TempDir(), "/ref/metadata.yaml", limits)
		require.Error(t, err)
	})

	t.Run("outside payload counts globally", func(t *testing.T) {
		archive := makeTar(t,
			tarEntry{name: "outside/bomb", contents: strings.Repeat("x", 16<<10)},
			tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
		)
		limits := testLimits()
		limits.maxUncompressed = int64(len(archive) - 1)
		err := applyImageLayers(context.Background(), imageFromLayers(t,
			static.NewLayer(archive, types.OCIUncompressedLayer),
		), t.TempDir(), "/ref/metadata.yaml", limits)
		assert.ErrorIs(t, err, errUncompressedLimit)
	})
}

func TestSelectedNodeBudgetStopsDeepParentExpansion(t *testing.T) {
	image := imageFromLayers(t, uncompressedLayer(t,
		tarEntry{name: "ref/a/b/c/d/value.yaml", contents: "value"},
		tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
	))
	limits := testLimits()
	limits.maxSelectedNodes = 3 // staging root plus a and a/b
	staging := t.TempDir()
	err := applyImageLayers(context.Background(), image, staging, "/ref/metadata.yaml", limits)
	assert.ErrorIs(t, err, errSelectedNodeLimit)
	_, err = os.Stat(filepath.Join(staging, "a", "b"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(staging, "a", "b", "c"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	reference, err := parsePath("container://example/ref:v1:/ref/metadata.yaml")
	require.NoError(t, err)
	tempRoot := t.TempDir()
	reader := newRegistryReader()
	reader.limits = limits
	reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
		return image, nil
	}
	_, err = reader.extract(context.Background(), reference, tempRoot)
	assert.ErrorIs(t, err, errSelectedNodeLimit)
	entries, readErr := os.ReadDir(tempRoot)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestSelectedHeaderDeclaredSizeValidation(t *testing.T) {
	limits := testLimits()
	selected := testSelectedState(t, limits)
	header := &tar.Header{Name: "metadata.yaml", Typeflag: tar.TypeReg, Size: -1}
	err := materializeSelectedEntry(bytes.NewReader(nil), header, t.TempDir(), "metadata.yaml", limits, selected)
	require.Error(t, err)

	header.Size = limits.maxFileBytes + 1
	err = materializeSelectedEntry(bytes.NewReader(nil), header, t.TempDir(), "metadata.yaml", limits, selected)
	require.Error(t, err)

	header = &tar.Header{Name: "metadata.yaml", Typeflag: tar.TypeGNUSparse}
	err = materializeSelectedEntry(bytes.NewReader(nil), header, t.TempDir(), "metadata.yaml", limits, testSelectedState(t, limits))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sparse")

	header = &tar.Header{
		Name:       "metadata.yaml",
		Typeflag:   tar.TypeReg,
		PAXRecords: map[string]string{"GNU.sparse.size": "10"},
	}
	err = materializeSelectedEntry(bytes.NewReader(nil), header, t.TempDir(), "metadata.yaml", limits, testSelectedState(t, limits))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sparse")
}

func TestBudgetReaderExactBoundaryAndLimit(t *testing.T) {
	t.Run("exact boundary reaches EOF", func(t *testing.T) {
		budget := &byteBudget{limit: 4, err: errActualCompressedLimit}
		contents, err := io.ReadAll(&budgetReader{
			ctx: context.Background(), reader: strings.NewReader("four"), budget: budget,
		})
		require.NoError(t, err)
		assert.Equal(t, "four", string(contents))
		assert.Equal(t, int64(4), budget.used)
	})

	t.Run("limit plus one fails", func(t *testing.T) {
		budget := &byteBudget{limit: 3, err: errActualCompressedLimit}
		_, err := io.ReadAll(&budgetReader{
			ctx: context.Background(), reader: strings.NewReader("four"), budget: budget,
		})
		assert.ErrorIs(t, err, errActualCompressedLimit)
		assert.Equal(t, int64(4), budget.used)
	})
}

func TestZstdDecoderWindowLimit(t *testing.T) {
	assert.Equal(t, uint64(64<<20), defaultExtractionLimits.maxZstdWindowBytes)
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed, zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(64<<10))
	require.NoError(t, err)
	for index := range 256 {
		_, err := encoder.Write([]byte(strings.Repeat(string([]byte{byte(index)}), 1_024)))
		require.NoError(t, err)
	}
	require.NoError(t, encoder.Close())

	decoded, closeDecoder, err := openLayerDecoder(bytes.NewReader(compressed.Bytes()), types.OCILayerZStd, 32<<10)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, decoded)
	require.Error(t, err)
	require.NoError(t, closeDecoder())
}

func TestApplyImageLayersCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := applyImageLayers(ctx, imageFromLayers(t, uncompressedLayer(t,
		tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
	)), t.TempDir(), "/ref/metadata.yaml", testLimits())
	assert.ErrorIs(t, err, context.Canceled)
}

func TestApplyImageLayersCancellationDuringReadClosesLayer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	baseLayer := uncompressedLayer(t,
		tarEntry{name: "outside/large", contents: strings.Repeat("x", 32<<10)},
		tarEntry{name: "ref/metadata.yaml", contents: "metadata"},
	)
	baseImage := imageFromLayers(t, baseLayer)
	closed := false
	image := layersOverrideImage{
		Image: baseImage,
		layers: []v1.Layer{cancelingLayer{
			Layer:  baseLayer,
			cancel: cancel,
			closed: &closed,
		}},
	}
	err := applyImageLayers(ctx, image, t.TempDir(), "/ref/metadata.yaml", testLimits())
	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, closed)
}

func TestRegistryReaderWiringAndCleanup(t *testing.T) {
	reference, err := parsePath("container://example/ref:v1:/ref/metadata.yaml")
	require.NoError(t, err)
	tempRoot := t.TempDir()

	t.Run("wiring and publication", func(t *testing.T) {
		var gotPlatform v1.Platform
		var gotKeychain authn.Keychain
		var gotReference string
		var gotDeadline time.Time
		reader := newRegistryReader()
		reader.pull = func(ctx context.Context, ref name.Reference, keychain authn.Keychain, platform v1.Platform) (v1.Image, error) {
			require.NoError(t, ctx.Err())
			gotReference = ref.Name()
			var ok bool
			gotDeadline, ok = ctx.Deadline()
			require.True(t, ok)
			gotKeychain = keychain
			gotPlatform = platform
			return empty.Image, nil
		}
		reader.apply = func(_ context.Context, _ v1.Image, staging, _ string, _ extractionLimits) error {
			return os.WriteFile(filepath.Join(staging, "metadata.yaml"), []byte("ok"), 0o600)
		}
		result, err := reader.extract(context.Background(), reference, tempRoot)
		require.NoError(t, err)
		assert.IsType(t, &safeDefaultKeychain{}, gotKeychain)
		assert.Equal(t, reference.image.Name(), gotReference)
		assert.WithinDuration(t, time.Now().Add(registryReadTimeout), gotDeadline, 2*time.Second)
		assert.Equal(t, "linux", gotPlatform.OS)
		assert.Equal(t, runtime.GOARCH, gotPlatform.Architecture)
		_, err = os.Stat(filepath.Join(result, "metadata.yaml"))
		require.NoError(t, err)
		assert.NotContains(t, filepath.Base(result), ".partial")
	})

	t.Run("joins apply and cleanup errors", func(t *testing.T) {
		applyErr := errors.New("apply failed")
		cleanupErr := errors.New("cleanup failed")
		reader := newRegistryReader()
		reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
			return empty.Image, nil
		}
		reader.apply = func(context.Context, v1.Image, string, string, extractionLimits) error {
			return applyErr
		}
		reader.removeAll = func(path string) error {
			_ = os.RemoveAll(path)
			return cleanupErr
		}
		_, err := reader.extract(context.Background(), reference, t.TempDir())
		assert.ErrorIs(t, err, applyErr)
		assert.ErrorIs(t, err, cleanupErr)
	})

	t.Run("existing destination is not overwritten", func(t *testing.T) {
		reader := newRegistryReader()
		reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
			return empty.Image, nil
		}
		reader.apply = func(context.Context, v1.Image, string, string, extractionLimits) error { return nil }
		existing, err := os.Stat(tempRoot)
		require.NoError(t, err)
		reader.lstat = func(string) (fs.FileInfo, error) { return existing, nil }
		_, err = reader.extract(context.Background(), reference, tempRoot)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("destination created after check is not overwritten", func(t *testing.T) {
		root := t.TempDir()
		reader := newRegistryReader()
		reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
			return empty.Image, nil
		}
		reader.apply = func(context.Context, v1.Image, string, string, extractionLimits) error { return nil }
		var destination string
		reader.publish = func(staging, finalPath string) error {
			destination = finalPath
			require.NoError(t, os.Mkdir(finalPath, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(finalPath, "owner"), []byte("racer"), 0o600))
			return renameNoReplace(staging, finalPath)
		}
		_, err := reader.extract(context.Background(), reference, root)
		require.Error(t, err)
		contents, readErr := os.ReadFile(filepath.Join(destination, "owner"))
		require.NoError(t, readErr)
		assert.Equal(t, "racer", string(contents))
		entries, readErr := os.ReadDir(root)
		require.NoError(t, readErr)
		require.Len(t, entries, 1)
		assert.Equal(t, filepath.Base(destination), entries[0].Name())
	})

	t.Run("publication failure cleans staging", func(t *testing.T) {
		root := t.TempDir()
		publishErr := errors.New("publish failed")
		reader := newRegistryReader()
		reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
			return empty.Image, nil
		}
		reader.apply = func(context.Context, v1.Image, string, string, extractionLimits) error { return nil }
		reader.publish = func(string, string) error { return publishErr }
		_, err := reader.extract(context.Background(), reference, root)
		assert.ErrorIs(t, err, publishErr)
		entries, readErr := os.ReadDir(root)
		require.NoError(t, readErr)
		assert.Empty(t, entries)
	})

	t.Run("unexpected destination check error cleans staging", func(t *testing.T) {
		root := t.TempDir()
		checkErr := errors.New("lstat failed")
		reader := newRegistryReader()
		reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
			return empty.Image, nil
		}
		reader.apply = func(context.Context, v1.Image, string, string, extractionLimits) error { return nil }
		reader.lstat = func(string) (fs.FileInfo, error) { return nil, checkErr }
		_, err := reader.extract(context.Background(), reference, root)
		assert.ErrorIs(t, err, checkErr)
		entries, readErr := os.ReadDir(root)
		require.NoError(t, readErr)
		assert.Empty(t, entries)
	})
}

func TestRegistryReaderRejectsInvalidTempRootBeforePull(t *testing.T) {
	reference, err := parsePath("container://example/ref:v1:/ref/metadata.yaml")
	require.NoError(t, err)
	for testName, root := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "missing"),
		"file": func() string {
			file := filepath.Join(t.TempDir(), "not-a-directory")
			require.NoError(t, os.WriteFile(file, []byte("file"), 0o600))
			return file
		}(),
	} {
		t.Run(testName, func(t *testing.T) {
			pullCalled := false
			reader := newRegistryReader()
			reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
				pullCalled = true
				return empty.Image, nil
			}
			_, err := reader.extract(context.Background(), reference, root)
			require.Error(t, err)
			assert.False(t, pullCalled)
		})
	}
}

func TestRegistryReaderCleansCorruptAndMissingLayers(t *testing.T) {
	reference, err := parsePath("container://example/ref:v1:/ref/metadata.yaml")
	require.NoError(t, err)
	verificationErr := errors.New("verification failed")
	baseLayer := uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})
	baseImage := imageFromLayers(t, baseLayer)
	corruptImage := layersOverrideImage{
		Image:  baseImage,
		layers: []v1.Layer{compressedErrorLayer{Layer: baseLayer, err: verificationErr}},
	}

	for testName, image := range map[string]v1.Image{
		"corrupt": corruptImage,
		"missing metadata": imageFromLayers(t, uncompressedLayer(t,
			tarEntry{name: "ref/other.yaml", contents: "other"},
		)),
	} {
		t.Run(testName, func(t *testing.T) {
			tempRoot := t.TempDir()
			reader := newRegistryReader()
			reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
				return image, nil
			}
			_, err := reader.extract(context.Background(), reference, tempRoot)
			require.Error(t, err)
			entries, readErr := os.ReadDir(tempRoot)
			require.NoError(t, readErr)
			assert.Empty(t, entries)
		})
	}
}

func TestRegistryReaderCleansStagingForPolicyAndCancellationFailures(t *testing.T) {
	reference, err := parsePath("container://example/ref:v1:/ref/metadata.yaml")
	require.NoError(t, err)
	tests := []struct {
		name      string
		image     v1.Image
		configure func(*registryReader)
		context   func() context.Context
	}{
		{
			name:  "budget",
			image: imageFromLayers(t, uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})),
			configure: func(reader *registryReader) {
				reader.limits.maxRawHeaders = 0
			},
		},
		{
			name:  "path",
			image: imageFromLayers(t, uncompressedLayer(t, tarEntry{name: "../unsafe", contents: "bad"})),
		},
		{
			name: "type",
			image: imageFromLayers(t, uncompressedLayer(t,
				tarEntry{name: "ref/metadata.yaml", typeflag: tar.TypeSymlink, linkname: "/absolute"},
			)),
		},
		{
			name:  "cancellation",
			image: imageFromLayers(t, uncompressedLayer(t, tarEntry{name: "ref/metadata.yaml", contents: "metadata"})),
			context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			reader := newRegistryReader()
			if test.configure != nil {
				test.configure(&reader)
			}
			reader.pull = func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error) {
				return test.image, nil
			}
			ctx := context.Background()
			if test.context != nil {
				ctx = test.context()
			}
			_, err := reader.extract(ctx, reference, root)
			require.Error(t, err)
			entries, readErr := os.ReadDir(root)
			require.NoError(t, readErr)
			assert.Empty(t, entries)
		})
	}
}

func TestGetRefFSContextContainerAndCompatibility(t *testing.T) {
	tempRoot := t.TempDir()
	options := Options{
		ReferenceConfig: "container://example/ref:v1:/ref/metadata.yaml",
		TmpDir:          tempRoot,
		containerReferenceReader: func(ctx context.Context, raw, root string) (string, error) {
			require.NoError(t, ctx.Err())
			assert.Equal(t, "container://example/ref:v1:/ref/metadata.yaml", raw)
			assert.Equal(t, tempRoot, root)
			extracted := filepath.Join(root, "extracted")
			require.NoError(t, os.Mkdir(extracted, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(extracted, "metadata.yaml"), []byte("metadata"), 0o600))
			return extracted, nil
		},
	}
	referenceFS, err := options.GetRefFSContext(context.Background())
	require.NoError(t, err)
	contents, err := fs.ReadFile(referenceFS, "metadata.yaml")
	require.NoError(t, err)
	assert.Equal(t, "metadata", string(contents))

	local := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(local, "metadata.yaml"), []byte("local"), 0o600))
	localOptions := Options{ReferenceConfig: filepath.Join(local, "metadata.yaml")}
	_, err = localOptions.GetRefFS()
	require.NoError(t, err)
}

func isolatedCredentialEnvironment(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DOCKER_CONFIG", "")
	t.Setenv("DOCKER_AUTH_CONFIG", "")
	t.Setenv("REGISTRY_AUTH_FILE", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

func writeAuthFile(t *testing.T, path string, contents any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	data, err := json.Marshal(contents)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func encodedAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func resolveAuth(t *testing.T, keychain *safeDefaultKeychain) *authn.AuthConfig {
	t.Helper()
	resource, err := name.NewRegistry("example.test")
	require.NoError(t, err)
	authenticator, err := keychain.ResolveContext(context.Background(), resource)
	require.NoError(t, err)
	authConfig, err := authenticator.Authorization()
	require.NoError(t, err)
	return authConfig
}

func TestSafeDefaultKeychainFilesAndPrecedence(t *testing.T) {
	home := isolatedCredentialEnvironment(t)
	dockerPath := filepath.Join(home, ".docker", "config.json")
	podmanPath := filepath.Join(home, ".config", "containers", "auth.json")
	writeAuthFile(t, dockerPath, map[string]any{"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("docker-user", "docker-password")}}})
	writeAuthFile(t, podmanPath, map[string]any{"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("podman-user", "podman-password")}}})

	keychain := &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}}
	authConfig := resolveAuth(t, keychain)
	assert.Equal(t, "docker-user", authConfig.Username)
	assert.Equal(t, "docker-password", authConfig.Password)

	require.NoError(t, os.Remove(dockerPath))
	registryAuth := filepath.Join(t.TempDir(), "auth.json")
	writeAuthFile(t, registryAuth, map[string]any{"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("registry-user", "registry-password")}}})
	t.Setenv("REGISTRY_AUTH_FILE", registryAuth)
	authConfig = resolveAuth(t, keychain)
	assert.Equal(t, "registry-user", authConfig.Username)

	t.Setenv("REGISTRY_AUTH_FILE", "")
	authConfig = resolveAuth(t, keychain)
	assert.Equal(t, "podman-user", authConfig.Username)

	t.Setenv("DOCKER_AUTH_CONFIG", `{"auths":{"example.test":{"auth":"`+encodedAuth("env-user", "env-password")+`"}}}`)
	authConfig = resolveAuth(t, keychain)
	assert.Equal(t, "env-user", authConfig.Username)
	assert.Equal(t, "env-password", authConfig.Password)
}

func TestSafeDefaultKeychainCompatibilityCases(t *testing.T) {
	t.Run("explicit Docker config", func(t *testing.T) {
		isolatedCredentialEnvironment(t)
		dockerConfig := t.TempDir()
		t.Setenv("DOCKER_CONFIG", dockerConfig)
		writeAuthFile(t, filepath.Join(dockerConfig, "config.json"), map[string]any{
			"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("explicit-user", "explicit-password")}},
		})
		authConfig := resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
		assert.Equal(t, "explicit-user", authConfig.Username)
	})

	t.Run("Podman home fallback", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		t.Setenv("XDG_CONFIG_HOME", "")
		writeAuthFile(t, filepath.Join(home, ".config", "containers", "auth.json"), map[string]any{
			"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("home-user", "home-password")}},
		})
		authConfig := resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
		assert.Equal(t, "home-user", authConfig.Username)
	})

	t.Run("registry fallback after repository target", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
			"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("registry-user", "registry-password")}},
		})
		repository, err := name.NewRepository("example.test/team/image")
		require.NoError(t, err)
		authenticator, err := (&safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}}).ResolveContext(context.Background(), repository)
		require.NoError(t, err)
		authConfig, err := authenticator.Authorization()
		require.NoError(t, err)
		assert.Equal(t, "registry-user", authConfig.Username)
	})

	t.Run("Docker Hub historical key", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
			"auths": map[string]any{authn.DefaultAuthKey: map[string]string{"auth": encodedAuth("hub-user", "hub-password")}},
		})
		repository, err := name.NewRepository("index.docker.io/library/alpine")
		require.NoError(t, err)
		authenticator, err := (&safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}}).ResolveContext(context.Background(), repository)
		require.NoError(t, err)
		authConfig, err := authenticator.Authorization()
		require.NoError(t, err)
		assert.Equal(t, "hub-user", authConfig.Username)
	})

	t.Run("invalid environment falls back without output", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
			"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("fallback-user", "fallback-password")}},
		})
		t.Setenv("DOCKER_AUTH_CONFIG", `{"auths":{"example.test":{"auth":"environment-secret-sentinel"}`)
		var authConfig *authn.AuthConfig
		captured, captureErr := captureProcessOutput(t, func() error {
			authConfig = resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
			return nil
		})
		require.NoError(t, captureErr)
		assert.Equal(t, "fallback-user", authConfig.Username)
		assert.NotContains(t, captured, "environment-secret-sentinel")
	})

	t.Run("valid non-matching environment falls back", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
			"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("fallback-user", "fallback-password")}},
		})
		t.Setenv("DOCKER_AUTH_CONFIG", `{"auths":{"other.test":{"auth":"`+encodedAuth("other-user", "other-password")+`"}}}`)
		authConfig := resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
		assert.Equal(t, "fallback-user", authConfig.Username)
	})

	t.Run("non-matching file is anonymous", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
			"auths": map[string]any{"other.test": map[string]string{"auth": encodedAuth("other-user", "other-password")}},
		})
		authConfig := resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
		assert.Empty(t, authConfig.Username)
		assert.Empty(t, authConfig.Password)
	})
}

type recordingHelperRunner struct {
	calledSuffix string
}

func (runner *recordingHelperRunner) get(_ context.Context, suffix, serverURL string) (helperCredential, bool, error) {
	runner.calledSuffix = suffix
	return helperCredential{serverURL: serverURL, username: suffix + "-user", secret: "synthetic-password"}, false, nil
}

func TestSafeDefaultKeychainPrefersRegistryHelperOverGlobalStore(t *testing.T) {
	home := isolatedCredentialEnvironment(t)
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
		"credsStore":  "global",
		"credHelpers": map[string]string{"example.test": "specific"},
	})
	runner := &recordingHelperRunner{}
	authConfig := resolveAuth(t, &safeDefaultKeychain{runner: runner})
	assert.Equal(t, "specific", runner.calledSuffix)
	assert.Equal(t, "specific-user", authConfig.Username)
}

func installCredentialHelper(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	source, err := os.Open(executable)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, source.Close())
	}()

	directory := t.TempDir()
	name := "docker-credential-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destinationPath := filepath.Join(directory, name)
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	require.NoError(t, err)
	_, err = io.Copy(destination, source)
	require.NoError(t, err)
	require.NoError(t, destination.Close())
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func helperKeychain(t *testing.T, mode string) *safeDefaultKeychain {
	t.Helper()
	home := isolatedCredentialEnvironment(t)
	installCredentialHelper(t)
	t.Setenv(credentialHelperModeEnv, mode)
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{
		"credHelpers": map[string]string{"example.test": "test"},
	})
	return &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}}
}

func captureProcessOutput(t *testing.T, operation func() error) (string, error) {
	t.Helper()
	capture, err := os.CreateTemp(t.TempDir(), "captured-output-")
	require.NoError(t, err)

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	originalLogWriter := log.Writer()
	originalWarnWriter := gcrlogs.Warn.Writer()
	originalProgressWriter := gcrlogs.Progress.Writer()
	originalDebugWriter := gcrlogs.Debug.Writer()
	os.Stdout = capture
	os.Stderr = capture
	log.SetOutput(capture)
	gcrlogs.Warn.SetOutput(capture)
	gcrlogs.Progress.SetOutput(capture)
	gcrlogs.Debug.SetOutput(capture)
	operationErr := operation()
	os.Stdout = originalStdout
	os.Stderr = originalStderr
	log.SetOutput(originalLogWriter)
	gcrlogs.Warn.SetOutput(originalWarnWriter)
	gcrlogs.Progress.SetOutput(originalProgressWriter)
	gcrlogs.Debug.SetOutput(originalDebugWriter)

	require.NoError(t, capture.Sync())
	_, err = capture.Seek(0, io.SeekStart)
	require.NoError(t, err)
	contents, err := io.ReadAll(capture)
	require.NoError(t, err)
	require.NoError(t, capture.Close())
	return string(contents), operationErr
}

func TestSafeDefaultKeychainCredentialHelper(t *testing.T) {
	authConfig := resolveAuth(t, helperKeychain(t, "success"))
	assert.Equal(t, "helper-user", authConfig.Username)
	assert.Equal(t, "helper-password", authConfig.Password)
}

func TestSafeDefaultKeychainGlobalHelperTokenAndNotFound(t *testing.T) {
	t.Run("global credentials store", func(t *testing.T) {
		home := isolatedCredentialEnvironment(t)
		installCredentialHelper(t)
		t.Setenv(credentialHelperModeEnv, "success")
		writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), map[string]any{"credsStore": "test"})
		authConfig := resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
		assert.Equal(t, "helper-user", authConfig.Username)
	})

	t.Run("identity token", func(t *testing.T) {
		authConfig := resolveAuth(t, helperKeychain(t, "token"))
		assert.Equal(t, "identity-token", authConfig.IdentityToken)
		assert.Empty(t, authConfig.Password)
	})

	t.Run("not found becomes anonymous", func(t *testing.T) {
		authConfig := resolveAuth(t, helperKeychain(t, "not-found"))
		assert.Empty(t, authConfig.Username)
		assert.Empty(t, authConfig.Password)
	})
}

func TestSafeDefaultKeychainPodmanRuntimeFallback(t *testing.T) {
	isolatedCredentialEnvironment(t)
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	writeAuthFile(t, filepath.Join(runtimeDir, "containers", "auth.json"), map[string]any{
		"auths": map[string]any{"example.test": map[string]string{"auth": encodedAuth("runtime-user", "runtime-password")}},
	})
	authConfig := resolveAuth(t, &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}})
	assert.Equal(t, "runtime-user", authConfig.Username)
	assert.Equal(t, "runtime-password", authConfig.Password)
}

func TestCredentialHelperFailureRedactionAndBounds(t *testing.T) {
	t.Run("stdout and stderr are contained", func(t *testing.T) {
		keychain := helperKeychain(t, "failure")
		resource, err := name.NewRegistry("example.test")
		require.NoError(t, err)
		var resolveErr error
		captured, captureErr := captureProcessOutput(t, func() error {
			_, resolveErr = keychain.ResolveContext(context.Background(), resource)
			return nil
		})
		require.NoError(t, captureErr)
		require.Error(t, resolveErr)
		assert.NotContains(t, resolveErr.Error(), "stdout-secret-sentinel")
		assert.NotContains(t, resolveErr.Error(), "stderr-secret-sentinel")
		assert.NotContains(t, captured, "stdout-secret-sentinel")
		assert.NotContains(t, captured, "stderr-secret-sentinel")
	})

	t.Run("production registry boundary is redacted", func(t *testing.T) {
		keychain := helperKeychain(t, "failure")
		reference, err := name.NewTag("example.test/team/reference:v1")
		require.NoError(t, err)
		var pullErr error
		captured, captureErr := captureProcessOutput(t, func() error {
			_, pullErr = pullRemoteImageWithTransport(
				context.Background(),
				reference,
				keychain,
				v1.Platform{OS: "linux", Architecture: runtime.GOARCH},
				roundTripperFunc(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("network should not be reached")
				}),
			)
			return nil
		})
		require.NoError(t, captureErr)
		require.Error(t, pullErr)
		assert.Contains(t, pullErr.Error(), "pulling image")
		for _, sentinel := range []string{"stdout-secret-sentinel", "stderr-secret-sentinel"} {
			assert.NotContains(t, pullErr.Error(), sentinel)
			assert.NotContains(t, captured, sentinel)
		}
	})

	t.Run("malformed output is redacted", func(t *testing.T) {
		keychain := helperKeychain(t, "malformed")
		resource, err := name.NewRegistry("example.test")
		require.NoError(t, err)
		_, err = keychain.ResolveContext(context.Background(), resource)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "malformed-secret-sentinel")
	})

	t.Run("oversized output is bounded and redacted", func(t *testing.T) {
		keychain := helperKeychain(t, "oversized")
		keychain.runner = execCredentialHelperRunner{maxOutput: 128}
		resource, err := name.NewRegistry("example.test")
		require.NoError(t, err)
		_, err = keychain.ResolveContext(context.Background(), resource)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "oversized-secret-sentinel")
	})

	t.Run("oversized not-found output remains an error", func(t *testing.T) {
		keychain := helperKeychain(t, "oversized-not-found")
		keychain.runner = execCredentialHelperRunner{maxOutput: 128}
		resource, err := name.NewRegistry("example.test")
		require.NoError(t, err)
		_, err = keychain.ResolveContext(context.Background(), resource)
		assert.ErrorIs(t, err, errCredentialHelper)
	})
}

func TestCredentialHelperCancellationTerminatesProcess(t *testing.T) {
	keychain := helperKeychain(t, "hang")
	marker := filepath.Join(t.TempDir(), "pid")
	t.Setenv("KUBE_COMPARE_HELPER_MARKER", marker)
	resource, err := name.NewRegistry("example.test")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, resolveErr := keychain.ResolveContext(ctx, resource)
		result <- resolveErr
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("credential helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	started := time.Now()
	cancel()
	err = <-result
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(started), 3*time.Second)

	pidBytes, err := os.ReadFile(marker)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(pidBytes))
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		process, err := os.FindProcess(pid)
		require.NoError(t, err)
		assert.Error(t, process.Signal(syscall.Signal(0)), "credential helper process is still alive")
	}
}

func TestSafeDefaultKeychainAnonymousWhenMissing(t *testing.T) {
	isolatedCredentialEnvironment(t)
	keychain := &safeDefaultKeychain{runner: execCredentialHelperRunner{maxOutput: 1 << 20}}
	authConfig := resolveAuth(t, keychain)
	assert.Empty(t, authConfig.Username)
	assert.Empty(t, authConfig.Password)
}
