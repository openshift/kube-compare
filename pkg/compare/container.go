package compare

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	containerScheme      = "container://"
	containerPathDivider = ":/"
	registryReadTimeout  = 5 * time.Minute
)

// isContainer reports whether path uses the container reference scheme.
func isContainer(path string) bool {
	return strings.HasPrefix(path, containerScheme)
}

type containerReference struct {
	image        name.Reference
	metadataPath string
}

// parsePath parses container://<image-reference>:/<absolute-metadata-path>.
func parsePath(raw string) (containerReference, error) {
	if !isContainer(raw) {
		return containerReference{}, containerPathError()
	}

	remainder := strings.TrimPrefix(raw, containerScheme)
	divider := strings.LastIndex(remainder, containerPathDivider)
	if divider <= 0 {
		return containerReference{}, containerPathError()
	}

	imageName := remainder[:divider]
	metadataPath := remainder[divider+1:]
	if imageName == "" || metadataPath == "" || !path.IsAbs(metadataPath) ||
		strings.HasSuffix(metadataPath, "/") || strings.ContainsAny(metadataPath, "\x00\\:") {
		return containerReference{}, containerPathError()
	}
	for _, component := range strings.Split(metadataPath, "/") {
		if component == ".." {
			return containerReference{}, containerPathError()
		}
	}

	metadataPath = path.Clean(metadataPath)
	if metadataPath == "/" || path.Base(metadataPath) == "." {
		return containerReference{}, containerPathError()
	}

	image, err := name.ParseReference(imageName)
	if err != nil {
		// The parser can repeat its input. Do not include or wrap it because a
		// malformed reference may contain credential-like material.
		return containerReference{}, containerPathError()
	}

	return containerReference{image: image, metadataPath: metadataPath}, nil
}

func containerPathError() error {
	return fmt.Errorf(
		"incorrect path passed to -r, it must follow this format: %s<IMAGE>:/absolute/path/to/metadata.yaml",
		containerScheme,
	)
}

type extractionLimits struct {
	maxLayers                 int
	maxDeclaredCompressed     int64
	maxActualCompressed       int64
	maxUncompressed           int64
	maxRawHeaders             int
	maxStateEntries           int
	maxStateKeyBytes          int64
	maxSelectedEntries        int
	maxSelectedNodes          int
	maxSelectedKeyBytes       int64
	maxFileBytes              int64
	maxSelectedBytes          int64
	maxPathBytes              int
	maxPathComponentBytes     int
	maxZstdWindowBytes        uint64
	maxCredentialHelperOutput int
}

var defaultExtractionLimits = extractionLimits{
	maxLayers:                 128,
	maxDeclaredCompressed:     2 << 30,
	maxActualCompressed:       2 << 30,
	maxUncompressed:           2 << 30,
	maxRawHeaders:             100_000,
	maxStateEntries:           100_000,
	maxStateKeyBytes:          32 << 20,
	maxSelectedEntries:        10_000,
	maxSelectedNodes:          100_000,
	maxSelectedKeyBytes:       32 << 20,
	maxFileBytes:              32 << 20,
	maxSelectedBytes:          256 << 20,
	maxPathBytes:              4_096,
	maxPathComponentBytes:     255,
	maxZstdWindowBytes:        64 << 20,
	maxCredentialHelperOutput: 1 << 20,
}

type imagePuller func(context.Context, name.Reference, authn.Keychain, v1.Platform) (v1.Image, error)
type imageApplier func(context.Context, v1.Image, string, string, extractionLimits) error

type registryReader struct {
	pull      imagePuller
	apply     imageApplier
	keychain  authn.Keychain
	platform  v1.Platform
	limits    extractionLimits
	removeAll func(string) error
	publish   func(string, string) error
	lstat     func(string) (fs.FileInfo, error)
}

func newRegistryReader() registryReader {
	limits := defaultExtractionLimits
	return registryReader{
		pull:  pullRemoteImage,
		apply: applyImageLayers,
		keychain: &safeDefaultKeychain{
			runner: execCredentialHelperRunner{maxOutput: limits.maxCredentialHelperOutput},
		},
		platform:  v1.Platform{OS: "linux", Architecture: runtime.GOARCH},
		limits:    limits,
		removeAll: os.RemoveAll,
		publish:   renameNoReplace,
		lstat:     os.Lstat,
	}
}

type httpsOnlyTransport struct {
	delegate http.RoundTripper
}

func (transport httpsOnlyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		return nil, errors.New("registry request requires HTTPS")
	}
	response, err := transport.delegate.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("sending HTTPS registry request: %w", err)
	}
	return response, nil
}

func pullRemoteImage(
	ctx context.Context,
	ref name.Reference,
	keychain authn.Keychain,
	platform v1.Platform,
) (v1.Image, error) {
	return pullRemoteImageWithTransport(ctx, ref, keychain, platform, remote.DefaultTransport)
}

func pullRemoteImageWithTransport(
	ctx context.Context,
	ref name.Reference,
	keychain authn.Keychain,
	platform v1.Platform,
	transport http.RoundTripper,
) (v1.Image, error) {
	image, err := remote.Image(
		ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(keychain),
		remote.WithPlatform(platform),
		remote.WithTransport(httpsOnlyTransport{delegate: transport}),
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("pulling image %q from registry: %w", ref.Context().Name(), ctxErr)
		}
		// Registry response bodies are untrusted and may reflect authorization
		// data. Keep the externally returned error operation-only.
		return nil, fmt.Errorf("pulling image %q from registry failed", ref.Context().Name())
	}
	return image, nil
}

// getReferencesFromContainer pulls and extracts the reference directory into tempRoot.
func getReferencesFromContainer(ctx context.Context, raw, tempRoot string) (string, error) {
	reference, err := parsePath(raw)
	if err != nil {
		return "", err
	}
	return newRegistryReader().extract(ctx, reference, tempRoot)
}

func (reader registryReader) extract(
	ctx context.Context,
	reference containerReference,
	tempRoot string,
) (result string, resultErr error) {
	info, err := os.Stat(tempRoot)
	if err != nil {
		return "", fmt.Errorf("temporary directory could not be accessed: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("temporary path is not a directory: %s", tempRoot)
	}

	ctx, cancel := context.WithTimeout(ctx, registryReadTimeout)
	defer cancel()

	image, err := reader.pull(ctx, reference.image, reader.keychain, reader.platform)
	if err != nil {
		return "", err
	}

	staging, err := os.MkdirTemp(tempRoot, ".container-reference-*.partial")
	if err != nil {
		return "", fmt.Errorf("creating reference staging directory: %w", err)
	}
	if err := os.Chmod(staging, 0o700); err != nil {
		return "", errors.Join(
			fmt.Errorf("securing reference staging directory: %w", err),
			removeTemporaryPath(staging, reader.removeAll),
		)
	}
	published := false
	defer func() {
		if !published {
			resultErr = errors.Join(resultErr, removeTemporaryPath(staging, reader.removeAll))
		}
	}()

	if err := reader.apply(ctx, image, staging, reference.metadataPath, reader.limits); err != nil {
		return "", err
	}

	finalPath := strings.TrimSuffix(staging, ".partial")
	if _, err := reader.lstat(finalPath); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return "", fmt.Errorf("reference destination already exists: %s", finalPath)
		}
		return "", fmt.Errorf("checking reference destination: %w", err)
	}
	if err := reader.publish(staging, finalPath); err != nil {
		return "", fmt.Errorf("publishing extracted reference: %w", err)
	}
	published = true
	return finalPath, nil
}

func removeTemporaryPath(path string, removeAll func(string) error) error {
	if err := removeAll(path); err != nil {
		return fmt.Errorf("removing partial reference directory: %w", err)
	}
	return nil
}
