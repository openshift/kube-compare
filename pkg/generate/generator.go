// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

var (
	sanitizePathChars = regexp.MustCompile(`[^\w\-.]`)
	sanitizePathDashes = regexp.MustCompile(`-+`)
)

// defaultFieldsToOmit returns the standard fieldsToOmit configuration for generated metadata.
func defaultFieldsToOmit() map[string]any {
	return map[string]any{
		"defaultOmitRef": "all",
		"items": map[string]any{
			"defaults": []map[string]string{
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
				{"pathToKey": "metadata.labels.\"pod-security.kubernetes.io\""},
				{"pathToKey": "metadata.labels.\"operators.coreos.com/\""},
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

// omitAnnotationAndLabelKeys returns annotation and label keys referenced by defaultFieldsToOmit
// defaults entries (metadata.annotations."key" / metadata.labels."key").
func omitAnnotationAndLabelKeys() (annotations, labels []string) {
	fto := defaultFieldsToOmit()
	items, _ := fto["items"].(map[string]any)
	if items == nil {
		return nil, nil
	}
	defaults, _ := items["defaults"].([]map[string]string)
	if defaults == nil {
		return nil, nil
	}
	const annP = `metadata.annotations."`
	const lblP = `metadata.labels."`
	for _, m := range defaults {
		p := m["pathToKey"]
		if strings.HasPrefix(p, annP) && strings.HasSuffix(p, `"`) && len(p) > len(annP)+1 {
			annotations = append(annotations, p[len(annP):len(p)-1])
			continue
		}
		if strings.HasPrefix(p, lblP) && strings.HasSuffix(p, `"`) && len(p) > len(lblP)+1 {
			labels = append(labels, p[len(lblP):len(p)-1])
		}
	}
	return annotations, labels
}

// sanitizeFilename converts a resource name to a safe filename.
func sanitizeFilename(name string) string {
	safe := sanitizePathChars.ReplaceAllString(name, "-")
	safe = sanitizePathDashes.ReplaceAllString(safe, "-")
	safe = strings.Trim(safe, "-")
	if safe == "" {
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

// cleanResource returns a copy of the object with runtime-managed fields removed.
func cleanResource(obj *unstructured.Unstructured) map[string]any {
	result := make(map[string]any)
	for k, v := range obj.Object {
		result[k] = v
	}
	if metadata, ok := result["metadata"].(map[string]any); ok {
		for _, key := range []string{"resourceVersion", "uid", "creationTimestamp", "generation", "managedFields", "selfLink"} {
			delete(metadata, key)
		}
		annKeys, labelKeys := omitAnnotationAndLabelKeys()
		if ann, ok := metadata["annotations"].(map[string]any); ok {
			for _, k := range annKeys {
				delete(ann, k)
			}
			if len(ann) == 0 {
				delete(metadata, "annotations")
			}
		}
		if lbl, ok := metadata["labels"].(map[string]any); ok {
			for _, k := range labelKeys {
				delete(lbl, k)
			}
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
	config    *RefgenConfig
	outputDir string // absolute, cleaned root after Generate begins
	files     map[string][]fileEntry
}

type fileEntry struct {
	spec *ResourceSpec
	path string
}

// NewGenerator creates a new Generator.
func NewGenerator(config *RefgenConfig, outputDir string) *Generator {
	if outputDir == "" {
		outputDir = config.OutputDir
	}
	return &Generator{
		config:    config,
		outputDir: outputDir,
		files:     make(map[string][]fileEntry),
	}
}

// Generate writes the reference directory with metadata.yaml and CR files.
func (g *Generator) Generate(resourcesBySpec map[*ResourceSpec][]*unstructured.Unstructured) (string, error) {
	outputAbs, err := filepath.Abs(filepath.Clean(g.outputDir))
	if err != nil {
		return "", fmt.Errorf("failed to resolve output directory: %w", err)
	}
	g.outputDir = outputAbs
	if err := os.MkdirAll(g.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}
	for spec, resources := range resourcesBySpec {
		if len(resources) == 0 {
			continue
		}
		if err := g.writeCRFiles(spec, resources); err != nil {
			return "", err
		}
	}
	if err := g.writeMetadata(); err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(g.outputDir)
	if err != nil {
		return g.outputDir, nil
	}
	return absPath, nil
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

func (g *Generator) writeCRFiles(spec *ResourceSpec, resources []*unstructured.Unstructured) error {
	safeKind := sanitizePathSegment(spec.Kind)
	kindDir := filepath.Join(g.outputDir, safeKind)
	if err := g.pathWithinOutput(kindDir); err != nil {
		return err
	}
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", kindDir, err)
	}
	g.files[safeKind] = nil
	for _, r := range resources {
		filename := sanitizeFilename(r.GetName()) + ".yaml"
		crPath := filepath.Join(kindDir, filename)
		counter := 1
		for {
			if _, err := os.Stat(crPath); os.IsNotExist(err) {
				break
			}
			filename = fmt.Sprintf("%s-%d.yaml", sanitizeFilename(r.GetName()), counter)
			crPath = filepath.Join(kindDir, filename)
			counter++
		}
		clean := cleanResource(r)
		data, err := yaml.Marshal(clean)
		if err != nil {
			return fmt.Errorf("failed to marshal %s: %w", r.GetName(), err)
		}
		if err := g.pathWithinOutput(crPath); err != nil {
			return err
		}
		if err := os.WriteFile(crPath, data, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", crPath, err)
		}
		relativePath := safeKind + "/" + filepath.Base(crPath)
		g.files[safeKind] = append(g.files[safeKind], fileEntry{spec: spec, path: relativePath})
	}
	return nil
}

func (g *Generator) writeMetadata() error {
	metadata := map[string]any{
		"apiVersion":   "v2",
		"parts":        []map[string]any{},
		"fieldsToOmit": defaultFieldsToOmit(),
	}
	for kind, entries := range g.files {
		if len(entries) == 0 {
			continue
		}
		spec := entries[0].spec
		paths := make([]map[string]string, 0, len(entries))
		for _, e := range entries {
			paths = append(paths, map[string]string{"path": e.path})
		}
		component := map[string]any{"name": strings.ToLower(kind)}
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
			"name":        fmt.Sprintf("%s-%s", reqStr, strings.ToLower(kind)),
			"description": fmt.Sprintf("%s %s resources", reqTitle, spec.Kind),
			"components":  []map[string]any{component},
		}
		metadata["parts"] = append(metadata["parts"].([]map[string]any), part)
	}
	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	metadataPath := filepath.Join(g.outputDir, "metadata.yaml")
	if err := g.pathWithinOutput(metadataPath); err != nil {
		return err
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write metadata.yaml: %w", err)
	}
	return nil
}
