// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

var (
	sanitizePathChars  = regexp.MustCompile(`[^\w\-.]`)
	sanitizePathDashes = regexp.MustCompile(`-+`)
)

// defaultFieldsToOmit returns the standard fieldsToOmit configuration for generated metadata.
func defaultFieldsToOmit() map[string]any {
	return map[string]any{
		"defaultOmitRef": "all",
		"items": map[string]any{
			"defaults": []map[string]any{
				{"pathToKey": "metadata.annotations.\"kubernetes.io/metadata.name\""},
				{"pathToKey": "metadata.annotations.\"openshift.io/sa.scc.uid-range\""},
				{"pathToKey": "metadata.annotations.\"openshift.io/sa.scc.mcs\""},
				{"pathToKey": "metadata.annotations.\"openshift.io/sa.scc.supplemental-groups\""},
				{"pathToKey": "metadata.annotations.\"machineconfiguration.openshift.io/mc-name-suffix\""},
				{"pathToKey": "metadata.annotations.\"kubectl.kubernetes.io/last-applied-configuration\""},
				{"pathToKey": "metadata.annotations.\"nmstate.io/webhook-mutating-timestamp\""},
				{"pathToKey": "metadata.annotations.\"ran.openshift.io/ztp-gitops-generated\""},
				{"pathToKey": "metadata.annotations.\"include.release.openshift.io/ibm-cloud-managed\""},
				{"pathToKey": "metadata.annotations.\"include.release.openshift.io/self-managed-high-availability\""},
				{"pathToKey": "metadata.annotations.\"include.release.openshift.io/single-node-developer\""},
				{"pathToKey": "metadata.annotations.\"release.openshift.io/create-only\""},
				{"pathToKey": "metadata.annotations.\"capability.openshift.io/name\""},
				{"pathToKey": "metadata.annotations.\"olm.providedAPIs\""},
				{"pathToKey": "metadata.annotations.\"operator.sriovnetwork.openshift.io/last-network-namespace\""},
				{"pathToKey": "metadata.annotations.\"k8s.v1.cni.cncf.io/resourceName\""},
				{"pathToKey": "metadata.annotations.\"security.openshift.io/MinimallySufficientPodSecurityStandard\""},
				{"pathToKey": "metadata.labels.\"kubernetes.io/metadata.name\""},
				// OLM and PSA inject multiple label keys under these prefixes; omit by prefix so
				// metadata.yaml matches kube-compare ManifestPathV1 isPrefix semantics.
				{"pathToKey": `metadata.labels."pod-security.kubernetes.io/"`, "isPrefix": true},
				{"pathToKey": `metadata.labels."operators.coreos.com/"`, "isPrefix": true},
				{"pathToKey": "metadata.labels.\"security.openshift.io/scc.podSecurityLabelSync\""},
				{"pathToKey": "metadata.labels.\"lca.openshift.io/target-ocp-version\""},
				{"pathToKey": "metadata.labels.\"olm.operatorgroup.uid\""},
				{"pathToKey": "metadata.resourceVersion"},
				{"pathToKey": "metadata.uid"},
				{"pathToKey": "metadata.creationTimestamp"},
				{"pathToKey": "metadata.generation"},
				{"pathToKey": "metadata.finalizers"},
				{"pathToKey": "metadata.ownerReferences"},
				{"pathToKey": "spec.finalizers"},
				{"pathToKey": "spec.ownerReferences"},
				{"pathToKey": "spec.clusterID"},
				{"pathToKey": "spec.filters"},
			},
			"all": []map[string]any{
				{"include": "defaults"},
				{"pathToKey": "status"},
			},
		},
	}
}

const (
	annotationPathPrefix = `metadata.annotations."`
	labelPathPrefix      = `metadata.labels."`
)

func pathToKeyForAnnotation(key string) string {
	return annotationPathPrefix + key + `"`
}

func pathToKeyForLabel(key string) string {
	return labelPathPrefix + key + `"`
}

// mergeFieldsToOmit returns fieldsToOmit metadata: built-in defaults plus any
// omitAnnotations / omitLabels from the refgen config.
func mergeFieldsToOmit(cfg *RefgenConfig) map[string]any {
	fto := defaultFieldsToOmit()
	if cfg == nil || (len(cfg.OmitAnnotations) == 0 && len(cfg.OmitLabels) == 0) {
		return fto
	}
	itemsAny, itemsPresent := fto["items"]
	if !itemsPresent {
		panic(fmt.Sprintf(
			"internal: defaultFieldsToOmit changed: missing top-level \"items\" (mergeFieldsToOmit RefgenConfig OmitAnnotations=%d OmitLabels=%d)",
			len(cfg.OmitAnnotations), len(cfg.OmitLabels)))
	}
	items, itemsOK := itemsAny.(map[string]any)
	if !itemsOK {
		panic(fmt.Sprintf(
			"internal: defaultFieldsToOmit changed: expected items to be map[string]any but was %T (mergeFieldsToOmit RefgenConfig OmitAnnotations=%d OmitLabels=%d)",
			itemsAny, len(cfg.OmitAnnotations), len(cfg.OmitLabels)))
	}
	if items == nil {
		panic(fmt.Sprintf(
			"internal: defaultFieldsToOmit changed: items map is nil (mergeFieldsToOmit RefgenConfig OmitAnnotations=%d OmitLabels=%d)",
			len(cfg.OmitAnnotations), len(cfg.OmitLabels)))
	}
	defaultsAny := items["defaults"]
	orig, defaultsOK := defaultsAny.([]map[string]any)
	if !defaultsOK {
		panic(fmt.Sprintf(
			"internal: defaultFieldsToOmit changed: expected items.defaults to be []map[string]any but was %T (mergeFieldsToOmit RefgenConfig OmitAnnotations=%d OmitLabels=%d)",
			defaultsAny, len(cfg.OmitAnnotations), len(cfg.OmitLabels)))
	}
	merged := make([]map[string]any, 0, len(orig)+len(cfg.OmitAnnotations)+len(cfg.OmitLabels))
	merged = append(merged, orig...)
	for _, k := range cfg.OmitAnnotations {
		merged = append(merged, map[string]any{"pathToKey": pathToKeyForAnnotation(k)})
	}
	for _, k := range cfg.OmitLabels {
		merged = append(merged, map[string]any{"pathToKey": pathToKeyForLabel(k)})
	}
	items["defaults"] = merged
	return fto
}

func defaultsEntryIsPrefix(m map[string]any) bool {
	v, ok := m["isPrefix"]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true")
	default:
		return false
	}
}

func forEachDefaultsEntry(defaultsAny any, fn func(pathToKey string, isPrefix bool)) {
	switch defaults := defaultsAny.(type) {
	case []map[string]any:
		for _, m := range defaults {
			p, _ := m["pathToKey"].(string)
			if p == "" {
				continue
			}
			fn(p, defaultsEntryIsPrefix(m))
		}
	case []any:
		for _, elem := range defaults {
			m, ok := elem.(map[string]any)
			if !ok {
				continue
			}
			pv, ok := m["pathToKey"].(string)
			if !ok || pv == "" {
				continue
			}
			fn(pv, defaultsEntryIsPrefix(m))
		}
	}
}

// omitAnnotationAndLabelKeys returns exact and prefix keys for annotations and labels
// described by fieldsToOmit defaults (metadata.annotations."key" / metadata.labels."key",
// optional isPrefix for prefix removal on the underlying map keys).
func omitAnnotationAndLabelKeys(fto map[string]any) (
	annExact, annPrefix, lblExact, lblPrefix []string,
) {
	items, _ := fto["items"].(map[string]any)
	if items == nil {
		return nil, nil, nil, nil
	}
	seenAnn := make(map[string]struct{})
	seenAnnPref := make(map[string]struct{})
	seenLbl := make(map[string]struct{})
	seenLblPref := make(map[string]struct{})

	forEachDefaultsEntry(items["defaults"], func(p string, isPrefix bool) {
		if strings.HasPrefix(p, annotationPathPrefix) && strings.HasSuffix(p, `"`) && len(p) > len(annotationPathPrefix)+1 {
			k := p[len(annotationPathPrefix) : len(p)-1]
			if isPrefix {
				if _, ok := seenAnnPref[k]; !ok {
					seenAnnPref[k] = struct{}{}
					annPrefix = append(annPrefix, k)
				}
				return
			}
			if _, ok := seenAnn[k]; !ok {
				seenAnn[k] = struct{}{}
				annExact = append(annExact, k)
			}
			return
		}
		if strings.HasPrefix(p, labelPathPrefix) && strings.HasSuffix(p, `"`) && len(p) > len(labelPathPrefix)+1 {
			k := p[len(labelPathPrefix) : len(p)-1]
			if isPrefix {
				if _, ok := seenLblPref[k]; !ok {
					seenLblPref[k] = struct{}{}
					lblPrefix = append(lblPrefix, k)
				}
				return
			}
			if _, ok := seenLbl[k]; !ok {
				seenLbl[k] = struct{}{}
				lblExact = append(lblExact, k)
			}
		}
	})
	return annExact, annPrefix, lblExact, lblPrefix
}

func deleteMapKeysByPrefix(m map[string]any, prefixes []string) {
	if len(m) == 0 || len(prefixes) == 0 {
		return
	}
	for k := range m {
		for _, pref := range prefixes {
			if strings.HasPrefix(k, pref) {
				delete(m, k)
				break
			}
		}
	}
}

// sanitizeFilename converts a resource name to a safe filename.
func sanitizeFilename(name string) string {
	safe := sanitizePathChars.ReplaceAllString(name, "-")
	safe = sanitizePathDashes.ReplaceAllString(safe, "-")
	safe = strings.Trim(safe, "-.")
	if safe == "" || safe == "." || safe == ".." || strings.Contains(safe, "..") {
		return "unnamed"
	}
	return safe
}

// sanitizePathSegment maps user-controlled strings (e.g. Kind) to a single directory name
// that cannot contain path separators or traverse outside the output directory when joined.
func sanitizePathSegment(s string) string {
	s = strings.ReplaceAll(s, string(filepath.Separator), "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, `\`, "-")
	safe := sanitizePathChars.ReplaceAllString(s, "-")
	safe = sanitizePathDashes.ReplaceAllString(safe, "-")
	safe = strings.Trim(safe, "-.")
	if safe == "" || safe == "." || safe == ".." || strings.Contains(safe, "..") {
		return "resource"
	}
	return safe
}

// cleanResource returns a deep copy of the object (via DeepCopy) with runtime-managed fields removed.
// fto is the merged fieldsToOmit map (defaults entries drive annotation/label key removal).
func cleanResource(obj *unstructured.Unstructured, fto map[string]any) map[string]any {
	result := obj.DeepCopy().Object
	if metadata, ok := result["metadata"].(map[string]any); ok {
		for _, key := range []string{"resourceVersion", "uid", "creationTimestamp", "generation", "managedFields", "selfLink"} {
			delete(metadata, key)
		}
		annExact, annPrefix, lblExact, lblPrefix := omitAnnotationAndLabelKeys(fto)
		if ann, ok := metadata["annotations"].(map[string]any); ok {
			for _, k := range annExact {
				delete(ann, k)
			}
			deleteMapKeysByPrefix(ann, annPrefix)
			if len(ann) == 0 {
				delete(metadata, "annotations")
			}
		}
		if lbl, ok := metadata["labels"].(map[string]any); ok {
			for _, k := range lblExact {
				delete(lbl, k)
			}
			deleteMapKeysByPrefix(lbl, lblPrefix)
			if len(lbl) == 0 {
				delete(metadata, "labels")
			}
		}
	}
	delete(result, "status")
	return result
}

// Generator generates kube-compare reference files.
type Generator struct {
	config       *RefgenConfig
	outputDir    string              // absolute, cleaned root after Generate begins
	files        map[int][]fileEntry // config.Resources index: one slice per ResourceSpec row (same Kind allowed)
	fieldsToOmit map[string]any
}

type fileEntry struct {
	path string
}

// NewGenerator creates a new Generator.
func NewGenerator(config *RefgenConfig, outputDir string) *Generator {
	if outputDir == "" {
		outputDir = config.OutputDir
	}
	return &Generator{
		config:       config,
		outputDir:    outputDir,
		files:        make(map[int][]fileEntry),
		fieldsToOmit: mergeFieldsToOmit(config),
	}
}

// Generate writes the reference directory with metadata.yaml and CR files.
// resourcesBySpec must use the same pointers as the configured rows, i.e.
// &config.Resources[i] for each index i (see Options.Run); each row gets its
// own metadata part even when Kind matches another row.
func (g *Generator) Generate(resourcesBySpec map[*ResourceSpec][]*unstructured.Unstructured) (string, error) {
	outputAbs, err := filepath.Abs(filepath.Clean(g.outputDir))
	if err != nil {
		return "", fmt.Errorf("failed to resolve output directory: %w", err)
	}
	g.outputDir = outputAbs
	if err := os.MkdirAll(g.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}
	g.files = make(map[int][]fileEntry)
	for i := range g.config.Resources {
		spec := &g.config.Resources[i]
		resources := resourcesBySpec[spec]
		if len(resources) == 0 {
			continue
		}
		if err := g.writeCRFiles(i, spec, resources); err != nil {
			return "", err
		}
	}

	if err := g.writeMetadata(); err != nil {
		return "", err
	}
	return g.outputDir, nil
}

func (g *Generator) pathWithinOutput(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	rel, err := filepath.Rel(g.outputDir, abs)
	if err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes output directory: %s", path)
	}
	return nil
}

func (g *Generator) writeCRFiles(specIndex int, spec *ResourceSpec, resources []*unstructured.Unstructured) error {
	safeKind := sanitizePathSegment(spec.Kind)
	kindDir := filepath.Join(g.outputDir, safeKind)
	if err := g.pathWithinOutput(kindDir); err != nil {
		return err
	}
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", kindDir, err)
	}
	for _, r := range resources {
		filename := sanitizeFilename(r.GetName()) + ".yaml"
		crPath := filepath.Join(kindDir, filename)
		counter := 1
		for {
			if err := g.pathWithinOutput(crPath); err != nil {
				return err
			}
			f, err := os.OpenFile(crPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err == nil {
				if err := f.Close(); err != nil {
					return fmt.Errorf("failed to close %s: %w", crPath, err)
				}
				break
			}
			if errors.Is(err, fs.ErrExist) {
				filename = fmt.Sprintf("%s-%d.yaml", sanitizeFilename(r.GetName()), counter)
				crPath = filepath.Join(kindDir, filename)
				counter++
				continue
			}
			return fmt.Errorf("failed to reserve output path %s: %w", crPath, err)
		}
		clean := cleanResource(r, g.fieldsToOmit)
		data, err := yaml.Marshal(clean)
		if err != nil {
			return fmt.Errorf("failed to marshal %s: %w", r.GetName(), err)
		}
		if err := os.WriteFile(crPath, data, 0o600); err != nil {
			return fmt.Errorf("failed to write %s: %w", crPath, err)
		}
		relativePath := safeKind + "/" + filepath.Base(crPath)
		g.files[specIndex] = append(g.files[specIndex], fileEntry{path: relativePath})
	}
	return nil
}

func (g *Generator) writeMetadata() error {
	metadata := map[string]any{
		"apiVersion":   "v2",
		"parts":        []map[string]any{},
		"fieldsToOmit": g.fieldsToOmit,
	}
	parts := metadata["parts"].([]map[string]any)
	for i := range g.config.Resources {
		spec := &g.config.Resources[i]
		entries := g.files[i]
		if len(entries) == 0 {
			continue
		}
		safeKind := sanitizePathSegment(spec.Kind)
		paths := make([]map[string]string, 0, len(entries))
		for _, e := range entries {
			paths = append(paths, map[string]string{"path": e.path})
		}
		component := map[string]any{"name": strings.ToLower(safeKind)}
		if spec.Required {
			component["allOf"] = paths
		} else {
			component["anyOf"] = paths
		}
		reqStr := "optional"
		reqTitle := "Optional"
		if spec.Required {
			reqStr = "required"
			reqTitle = "Required"
		}
		part := map[string]any{
			"name":        fmt.Sprintf("%s-%s", reqStr, strings.ToLower(safeKind)),
			"description": fmt.Sprintf("%s %s resources", reqTitle, spec.Kind),
			"components":  []map[string]any{component},
		}
		parts = append(parts, part)
	}
	metadata["parts"] = parts
	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	metadataPath := filepath.Join(g.outputDir, "metadata.yaml")
	if err := g.pathWithinOutput(metadataPath); err != nil {
		return err
	}
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write metadata.yaml: %w", err)
	}
	return nil
}
