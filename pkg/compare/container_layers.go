package compare

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

const (
	whiteoutPrefix = ".wh."
	opaqueWhiteout = ".wh..wh..opq"
)

var (
	errActualCompressedLimit = errors.New("image actual compressed data exceeds limit")
	errUncompressedLimit     = errors.New("image uncompressed data exceeds limit")
	errRawHeaderLimit        = errors.New("image raw tar header count exceeds limit")
	errStateEntryLimit       = errors.New("image layer state entry count exceeds limit")
	errStateKeyBytesLimit    = errors.New("image layer state key bytes exceed limit")
	errSelectedNodeLimit     = errors.New("selected reference node count exceeds limit")
	errSelectedKeyBytesLimit = errors.New("selected reference node key bytes exceed limit")
	errUnsafeArchivePath     = errors.New("unsafe archive path")
)

type byteBudget struct {
	limit int64
	used  int64
	err   error
}

type budgetReader struct {
	ctx    context.Context
	reader io.Reader
	budget *byteBudget
}

func (reader *budgetReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, fmt.Errorf("bounded stream context: %w", err)
	}
	if len(data) == 0 {
		return 0, nil
	}
	if reader.budget.used > reader.budget.limit {
		return 0, reader.budget.err
	}

	remaining := reader.budget.limit - reader.budget.used
	if int64(len(data)) > remaining+1 {
		data = data[:remaining+1]
	}
	n, err := reader.reader.Read(data)
	reader.budget.used += int64(n)
	if reader.budget.used > reader.budget.limit {
		return n, reader.budget.err
	}
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	if err != nil {
		return n, fmt.Errorf("reading bounded stream: %w", err)
	}
	return n, nil
}

type headerBudget struct {
	limit int
	used  int
}

func (budget *headerBudget) charge() error {
	budget.used++
	if budget.used > budget.limit {
		return errRawHeaderLimit
	}
	return nil
}

type stateBudget struct {
	maxEntries  int
	maxBytes    int64
	entries     int
	bytes       int64
	entryErr    error
	keyBytesErr error
}

func (budget *stateBudget) retain(key string) error {
	keyBytes := int64(len(key))
	if budget.entries >= budget.maxEntries {
		return budget.entryErr
	}
	if keyBytes > budget.maxBytes-budget.bytes {
		return budget.keyBytesErr
	}
	budget.entries++
	budget.bytes += keyBytes
	return nil
}

func (budget *stateBudget) release(key string) {
	budget.entries--
	budget.bytes -= int64(len(key))
}

type visibilityState struct {
	files       map[string]bool
	opaqueDirs  map[string]struct{}
	stateBudget stateBudget
	headerCount headerBudget
}

type extractedNode struct {
	directory bool
	explicit  bool
}

type selectedState struct {
	parent        string
	metadataName  string
	entries       int
	selectedBytes int64
	nodes         map[string]extractedNode
	nodeBudget    stateBudget
}

func applyImageLayers(
	ctx context.Context,
	image v1.Image,
	staging string,
	metadataPath string,
	limits extractionLimits,
) (resultErr error) {
	manifest, layers, err := validatedImageLayers(image, limits)
	if err != nil {
		return err
	}
	extractionRoot, err := os.OpenRoot(staging)
	if err != nil {
		return fmt.Errorf("opening reference staging directory: %w", err)
	}
	defer func() {
		if err := extractionRoot.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("closing reference staging directory: %w", err))
		}
	}()

	visibility := visibilityState{
		files:      make(map[string]bool),
		opaqueDirs: make(map[string]struct{}),
		stateBudget: stateBudget{
			maxEntries:  limits.maxStateEntries,
			maxBytes:    limits.maxStateKeyBytes,
			entryErr:    errStateEntryLimit,
			keyBytesErr: errStateKeyBytesLimit,
		},
		headerCount: headerBudget{limit: limits.maxRawHeaders},
	}
	selectedParent := strings.TrimPrefix(path.Dir(metadataPath), "/")
	if selectedParent == "" {
		selectedParent = "."
	}
	selected := selectedState{
		parent:       selectedParent,
		metadataName: path.Base(metadataPath),
		nodes:        make(map[string]extractedNode),
		nodeBudget: stateBudget{
			maxEntries:  limits.maxSelectedNodes,
			maxBytes:    limits.maxSelectedKeyBytes,
			entryErr:    errSelectedNodeLimit,
			keyBytesErr: errSelectedKeyBytesLimit,
		},
	}
	if err := selected.nodeBudget.retain("."); err != nil {
		return err
	}
	selected.nodes["."] = extractedNode{directory: true, explicit: true}
	compressedBudget := byteBudget{
		limit: limits.maxActualCompressed,
		err:   errActualCompressedLimit,
	}
	uncompressedBudget := byteBudget{
		limit: limits.maxUncompressed,
		err:   errUncompressedLimit,
	}

	for layerIndex := len(layers) - 1; layerIndex >= 0; layerIndex-- {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("applying image context: %w", err)
		}
		if err := applyImageLayer(
			ctx,
			layers[layerIndex],
			manifest.Layers[layerIndex].MediaType,
			extractionRoot,
			limits,
			&compressedBudget,
			&uncompressedBudget,
			&visibility,
			&selected,
		); err != nil {
			return fmt.Errorf("applying image layer %d: %w", layerIndex, err)
		}
	}

	metadataInfo, err := extractionRoot.Lstat(selected.metadataName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("requested metadata file was not found in image")
		}
		return fmt.Errorf("checking requested metadata file: %w", err)
	}
	if !metadataInfo.Mode().IsRegular() {
		return fmt.Errorf("requested metadata path is not a regular file")
	}
	return nil
}

func validatedImageLayers(image v1.Image, limits extractionLimits) (*v1.Manifest, []v1.Layer, error) {
	manifest, err := image.Manifest()
	if err != nil {
		return nil, nil, fmt.Errorf("reading image manifest: %w", err)
	}
	if len(manifest.Layers) > limits.maxLayers {
		return nil, nil, fmt.Errorf("image contains %d layers, limit is %d", len(manifest.Layers), limits.maxLayers)
	}

	var declaredCompressed int64
	for _, descriptor := range manifest.Layers {
		if descriptor.Size < 0 || descriptor.Size > limits.maxDeclaredCompressed-declaredCompressed {
			return nil, nil, fmt.Errorf("image declared compressed layer data exceeds %d bytes", limits.maxDeclaredCompressed)
		}
		declaredCompressed += descriptor.Size
	}

	layers, err := image.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("retrieving image layers: %w", err)
	}
	if len(layers) != len(manifest.Layers) {
		return nil, nil, fmt.Errorf("image layer count does not match manifest")
	}
	for index, layer := range layers {
		digest, err := layer.Digest()
		if err != nil {
			return nil, nil, fmt.Errorf("reading image layer digest: %w", err)
		}
		if digest != manifest.Layers[index].Digest {
			return nil, nil, fmt.Errorf("image layer order does not match manifest")
		}
	}
	return manifest, layers, nil
}

func applyImageLayer(
	ctx context.Context,
	layer v1.Layer,
	mediaType types.MediaType,
	extractionRoot *os.Root,
	limits extractionLimits,
	compressedBudget *byteBudget,
	uncompressedBudget *byteBudget,
	visibility *visibilityState,
	selected *selectedState,
) (resultErr error) {
	compressed, err := layer.Compressed()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("opening compressed layer: %w", ctxErr)
		}
		// A registry controls HTTP error response bodies. Do not propagate a
		// response that could reflect authorization data.
		return errors.New("opening compressed image layer failed")
	}
	compressedClosed := false
	defer func() {
		if !compressedClosed {
			resultErr = errors.Join(resultErr, closeWithContext("compressed layer", compressed))
		}
	}()

	boundedCompressed := &budgetReader{ctx: ctx, reader: compressed, budget: compressedBudget}
	decoded, closeDecoder, err := openLayerDecoder(boundedCompressed, mediaType, limits.maxZstdWindowBytes)
	if err != nil {
		return err
	}
	decoderClosed := false
	defer func() {
		if !decoderClosed {
			resultErr = errors.Join(resultErr, closeDecoder())
		}
	}()

	boundedUncompressed := &budgetReader{ctx: ctx, reader: decoded, budget: uncompressedBudget}
	if err := applyLayerTar(ctx, tar.NewReader(boundedUncompressed), extractionRoot, limits, visibility, selected); err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, boundedUncompressed); err != nil {
		return fmt.Errorf("draining uncompressed layer: %w", err)
	}
	if err := closeDecoder(); err != nil {
		decoderClosed = true
		return err
	}
	decoderClosed = true

	if _, err := io.Copy(io.Discard, boundedCompressed); err != nil {
		return fmt.Errorf("verifying compressed layer: %w", err)
	}
	if err := compressed.Close(); err != nil {
		compressedClosed = true
		return fmt.Errorf("closing compressed layer: %w", err)
	}
	compressedClosed = true
	return nil
}

func openLayerDecoder(
	compressed io.Reader,
	mediaType types.MediaType,
	maxZstdWindow uint64,
) (io.Reader, func() error, error) {
	switch mediaType {
	case types.OCILayer, types.OCIRestrictedLayer, types.DockerLayer, types.DockerForeignLayer:
		reader, err := gzip.NewReader(compressed)
		if err != nil {
			return nil, nil, fmt.Errorf("opening gzip layer: %w", err)
		}
		return reader, func() error {
			if err := reader.Close(); err != nil {
				return fmt.Errorf("closing gzip layer: %w", err)
			}
			return nil
		}, nil
	case types.OCILayerZStd:
		reader, err := zstd.NewReader(
			compressed,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderLowmem(true),
			zstd.WithDecoderMaxWindow(maxZstdWindow),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("opening zstd layer: %w", err)
		}
		return reader, func() error {
			reader.Close()
			return nil
		}, nil
	case types.OCIUncompressedLayer, types.OCIUncompressedRestrictedLayer, types.DockerUncompressedLayer:
		return compressed, func() error { return nil }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported image layer media type %q", mediaType)
	}
}

func closeWithContext(name string, closer io.Closer) error {
	if err := closer.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", name, err)
	}
	return nil
}

func applyLayerTar(
	ctx context.Context,
	tarReader *tar.Reader,
	extractionRoot *os.Root,
	limits extractionLimits,
	visibility *visibilityState,
	selected *selectedState,
) error {
	layerOpaque := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("reading layer context: %w", err)
		}
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading layer tar: %w", err)
		}
		if err := visibility.headerCount.charge(); err != nil {
			return err
		}

		archiveName, err := normalizeArchivePath(header.Name, header.Typeflag, limits)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		basename := path.Base(archiveName)
		dirname := path.Dir(archiveName)
		if basename == opaqueWhiteout {
			if _, exists := layerOpaque[dirname]; !exists {
				if err := visibility.stateBudget.retain(dirname); err != nil {
					return err
				}
				layerOpaque[dirname] = struct{}{}
			}
			continue
		}

		tombstone := strings.HasPrefix(basename, whiteoutPrefix)
		if tombstone {
			basename = strings.TrimPrefix(basename, whiteoutPrefix)
			if basename == "" || basename == "." || basename == ".." {
				return fmt.Errorf("invalid whiteout entry")
			}
		}

		stateName := path.Join(dirname, basename)
		if header.Typeflag == tar.TypeDir {
			stateName = archiveName
		}
		if _, exists := visibility.files[stateName]; exists && !tombstone {
			continue
		}
		if inWhiteoutDir(visibility.files, stateName) || inOpaqueDir(visibility.opaqueDirs, stateName) {
			continue
		}

		stateValue := tombstone || header.Typeflag != tar.TypeDir
		if _, exists := visibility.files[stateName]; !exists {
			if err := visibility.stateBudget.retain(stateName); err != nil {
				return err
			}
		}
		visibility.files[stateName] = stateValue
		if tombstone {
			continue
		}

		relative, selectedEntry := selectedRelativePath(archiveName, selected.parent)
		if !selectedEntry {
			continue
		}
		if err := validatePortableSelectedPath(relative); err != nil {
			return err
		}
		selected.entries++
		if selected.entries > limits.maxSelectedEntries {
			return fmt.Errorf("selected reference contains more than %d entries", limits.maxSelectedEntries)
		}
		if err := materializeSelectedEntry(tarReader, header, extractionRoot, relative, limits, selected); err != nil {
			return err
		}
	}

	for opaqueDir := range layerOpaque {
		delete(layerOpaque, opaqueDir)
		if _, exists := visibility.opaqueDirs[opaqueDir]; exists {
			visibility.stateBudget.release(opaqueDir)
			continue
		}
		visibility.opaqueDirs[opaqueDir] = struct{}{}
	}
	return nil
}

func selectedRelativePath(archiveName, selectedParent string) (string, bool) {
	if selectedParent == "." {
		return archiveName, true
	}
	if archiveName == selectedParent {
		return ".", true
	}
	prefix := selectedParent + "/"
	if strings.HasPrefix(archiveName, prefix) {
		return strings.TrimPrefix(archiveName, prefix), true
	}
	return "", false
}

func normalizeArchivePath(name string, typeflag byte, limits extractionLimits) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return "", errUnsafeArchivePath
	}
	if strings.HasSuffix(name, "/") {
		if typeflag != tar.TypeDir || strings.HasSuffix(name, "//") {
			return "", errUnsafeArchivePath
		}
		name = strings.TrimSuffix(name, "/")
	}
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.HasPrefix(name, "/") {
		return "", errUnsafeArchivePath
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return "", errUnsafeArchivePath
		}
	}

	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", errUnsafeArchivePath
	}
	if len(cleaned) > limits.maxPathBytes {
		return "", fmt.Errorf("archive path exceeds %d bytes", limits.maxPathBytes)
	}
	for _, component := range strings.Split(cleaned, "/") {
		if component == "" || len(component) > limits.maxPathComponentBytes {
			return "", errUnsafeArchivePath
		}
	}
	return cleaned, nil
}

func validatePortableSelectedPath(name string) error {
	if name == "." {
		return nil
	}
	if strings.ContainsAny(name, "\\:") {
		return errUnsafeArchivePath
	}
	for _, component := range strings.Split(name, "/") {
		if strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") ||
			isWindowsReservedName(component) {
			return errUnsafeArchivePath
		}
	}
	return nil
}

func isWindowsReservedName(component string) bool {
	trimmed := strings.TrimRight(component, " .")
	stem, _, _ := strings.Cut(trimmed, ".")
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) {
		return stem[3] >= '1' && stem[3] <= '9'
	}
	return false
}

func inOpaqueDir(opaqueDirs map[string]struct{}, file string) bool {
	for file != "" {
		dirname := path.Dir(file)
		if file == dirname {
			break
		}
		if _, found := opaqueDirs[dirname]; found {
			return true
		}
		file = dirname
	}
	return false
}

func inWhiteoutDir(files map[string]bool, file string) bool {
	for file != "" {
		dirname := path.Dir(file)
		if file == dirname {
			break
		}
		if hidesChildren, found := files[dirname]; found && hidesChildren {
			return true
		}
		file = dirname
	}
	return false
}

func materializeSelectedEntry(
	tarReader io.Reader,
	header *tar.Header,
	extractionRoot *os.Root,
	relative string,
	limits extractionLimits,
	selected *selectedState,
) error {
	if hasSparseMetadata(header) {
		return fmt.Errorf("sparse files are not supported in the selected reference")
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return createExtractedDirectory(extractionRoot, relative, selected)
	case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // TypeRegA is required for old-style tar compatibility.
		if relative == "." {
			return fmt.Errorf("selected reference root is not a directory")
		}
		if header.Size < 0 || header.Size > limits.maxFileBytes {
			return fmt.Errorf("selected file exceeds %d bytes", limits.maxFileBytes)
		}
		if header.Size > limits.maxSelectedBytes-selected.selectedBytes {
			return fmt.Errorf("selected reference files exceed %d bytes", limits.maxSelectedBytes)
		}
		if err := createExtractedFile(extractionRoot, relative, header.Size, tarReader, selected); err != nil {
			return err
		}
		selected.selectedBytes += header.Size
		return nil
	case tar.TypeLink:
		return fmt.Errorf("hard links are not supported in the selected reference")
	case tar.TypeSymlink:
		return fmt.Errorf("symbolic links are not supported in the selected reference")
	case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		return fmt.Errorf("special files are not supported in the selected reference")
	case tar.TypeGNUSparse:
		return fmt.Errorf("sparse files are not supported in the selected reference")
	default:
		return fmt.Errorf("unsupported tar entry type in the selected reference")
	}
}

func hasSparseMetadata(header *tar.Header) bool {
	for key := range header.PAXRecords {
		if strings.HasPrefix(key, "GNU.sparse.") || key == "SCHILY.realsize" {
			return true
		}
	}
	return false
}

func createExtractedDirectory(extractionRoot *os.Root, relative string, selected *selectedState) error {
	if relative == "." {
		return nil
	}
	if existing, found := selected.nodes[relative]; found {
		if !existing.directory {
			return fmt.Errorf("selected path has conflicting file and directory entries")
		}
		if existing.explicit {
			return fmt.Errorf("selected reference contains a duplicate directory")
		}
		existing.explicit = true
		selected.nodes[relative] = existing
		return nil
	}
	if err := ensureExtractedParents(extractionRoot, relative, selected); err != nil {
		return err
	}
	if err := selected.nodeBudget.retain(relative); err != nil {
		return err
	}
	if err := extractionRoot.Mkdir(relative, 0o700); err != nil {
		selected.nodeBudget.release(relative)
		return fmt.Errorf("creating extracted directory: %w", err)
	}
	selected.nodes[relative] = extractedNode{directory: true, explicit: true}
	return nil
}

func createExtractedFile(
	extractionRoot *os.Root,
	relative string,
	size int64,
	contents io.Reader,
	selected *selectedState,
) (resultErr error) {
	if _, found := selected.nodes[relative]; found {
		return fmt.Errorf("selected path has duplicate or conflicting entries")
	}
	if err := ensureExtractedParents(extractionRoot, relative, selected); err != nil {
		return err
	}
	if err := selected.nodeBudget.retain(relative); err != nil {
		return err
	}

	file, err := extractionRoot.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		selected.nodeBudget.release(relative)
		return fmt.Errorf("creating extracted file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("closing extracted file: %w", err))
		}
	}()
	if _, err := io.CopyN(file, contents, size); err != nil {
		return fmt.Errorf("writing extracted file: %w", err)
	}
	selected.nodes[relative] = extractedNode{explicit: true}
	return nil
}

func ensureExtractedParents(extractionRoot *os.Root, relative string, selected *selectedState) error {
	parent := path.Dir(relative)
	if parent == "." {
		return nil
	}
	components := strings.Split(parent, "/")
	current := ""
	for _, component := range components {
		if current == "" {
			current = component
		} else {
			current = path.Join(current, component)
		}
		if existing, found := selected.nodes[current]; found {
			if !existing.directory {
				return fmt.Errorf("selected path parent is a file")
			}
			continue
		}
		if err := selected.nodeBudget.retain(current); err != nil {
			return err
		}
		if err := extractionRoot.Mkdir(current, 0o700); err != nil {
			selected.nodeBudget.release(current)
			return fmt.Errorf("creating extracted parent: %w", err)
		}
		selected.nodes[current] = extractedNode{directory: true}
	}
	return nil
}
