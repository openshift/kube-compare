// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// defaultListLimit is the page size for server-side List pagination against the API server.
const defaultListLimit int64 = 500

// Fetcher fetches resources from a cluster or must-gather directory.
type Fetcher interface {
	FetchResources(ctx context.Context, spec *ResourceSpec) ([]*unstructured.Unstructured, error)
}

// ClusterFetcher fetches resources from a live Kubernetes cluster.
type ClusterFetcher struct {
	dynamicClient dynamic.Interface
	mapper        meta.RESTMapper
}

// NewClusterFetcher creates a ClusterFetcher using the given factory.
func NewClusterFetcher(f kcmdutil.Factory) (*ClusterFetcher, error) {
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	mapper, err := f.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST mapper: %w", err)
	}
	return &ClusterFetcher{
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}, nil
}

// FetchResources fetches all resources matching the given specification from the cluster.
func (f *ClusterFetcher) FetchResources(ctx context.Context, spec *ResourceSpec) ([]*unstructured.Unstructured, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fetch resources: %w", err)
	}
	gv, err := schema.ParseGroupVersion(spec.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %q: %w", spec.APIVersion, err)
	}
	gvk := gv.WithKind(spec.Kind)

	mapping, err := f.mapper.RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find API for %s (%s): %w", spec.Kind, spec.APIVersion, err)
	}

	gvr := mapping.Resource
	var ri dynamic.ResourceInterface
	if spec.Namespace != "" {
		ri = f.dynamicClient.Resource(gvr).Namespace(spec.Namespace)
	} else {
		ri = f.dynamicClient.Resource(gvr)
	}

	var merged []unstructured.Unstructured
	if len(spec.Names) == 0 {
		merged, err = listClusterResourcePages(ctx, ri, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", spec.Kind, err)
		}
	} else {
		seen := make(map[string]struct{}, len(spec.Names))
		for _, name := range spec.Names {
			if name == "" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			base := metav1.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(metav1.ObjectNameField, name).String(),
			}
			batch, err := listClusterResourcePages(ctx, ri, base)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch %s %q: %w", spec.Kind, name, err)
			}
			merged = append(merged, batch...)
		}
	}
	return unstructuredPtrSlice(merged), nil
}

func listClusterResourcePages(ctx context.Context, ri dynamic.ResourceInterface, base metav1.ListOptions) ([]unstructured.Unstructured, error) {
	opts := base
	opts.Limit = defaultListLimit
	var accumulated []unstructured.Unstructured
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list cluster resources: %w", err)
		}
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list cluster resources: %w", err)
		}
		accumulated = append(accumulated, list.Items...)
		if list.GetContinue() == "" {
			break
		}
		opts = metav1.ListOptions{
			Limit:         defaultListLimit,
			Continue:      list.GetContinue(),
			FieldSelector: base.FieldSelector,
			LabelSelector: base.LabelSelector,
		}
	}
	return accumulated, nil
}

func unstructuredPtrSlice(items []unstructured.Unstructured) []*unstructured.Unstructured {
	out := make([]*unstructured.Unstructured, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}

// MustGatherFetcher fetches resources from a must-gather directory.
type MustGatherFetcher struct {
	rootDir string
	cache   []*unstructured.Unstructured
}

// NewMustGatherFetcher creates a MustGatherFetcher for the given directory.
func NewMustGatherFetcher(mustGatherDir string) (*MustGatherFetcher, error) {
	absPath, err := filepath.Abs(mustGatherDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("must-gather directory does not exist: %s", mustGatherDir)
		}
		return nil, fmt.Errorf("failed to stat must-gather directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("must-gather path is not a directory: %s", mustGatherDir)
	}
	return &MustGatherFetcher{rootDir: absPath}, nil
}

// FetchResources fetches all resources matching the given specification from must-gather.
func (f *MustGatherFetcher) FetchResources(ctx context.Context, spec *ResourceSpec) ([]*unstructured.Unstructured, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fetch from must-gather: %w", err)
	}
	resources, err := f.loadAllResources()
	if err != nil {
		return nil, err
	}
	var matched []*unstructured.Unstructured
	for _, r := range resources {
		if r.GetKind() != spec.Kind || r.GetAPIVersion() != spec.APIVersion {
			continue
		}
		if spec.Namespace != "" && r.GetNamespace() != spec.Namespace {
			continue
		}
		if len(spec.Names) > 0 {
			found := false
			for _, n := range spec.Names {
				if r.GetName() == n {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		matched = append(matched, r)
	}
	return matched, nil
}

func (f *MustGatherFetcher) loadAllResources() ([]*unstructured.Unstructured, error) {
	if f.cache != nil {
		return f.cache, nil
	}
	roots, err := f.findDataRoots()
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no must-gather data found under %s (expected cluster-scoped-resources/ or namespaces/)", f.rootDir)
	}
	seen := make(map[string]bool)
	var loaded []*unstructured.Unstructured
	for _, root := range roots {
		for _, subdir := range []string{"cluster-scoped-resources", "namespaces"} {
			base := filepath.Join(root, subdir)
			if _, err := os.Stat(base); os.IsNotExist(err) {
				continue
			}
			err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
					return nil
				}
				objs, err := loadResourcesFromFile(path)
				if err != nil {
					klog.V(2).Infof("Skipping %s: %v", path, err)
					return nil
				}
				for _, obj := range objs {
					key := fmt.Sprintf("%s/%s/%s/%s", obj.GetAPIVersion(), obj.GetKind(), obj.GetNamespace(), obj.GetName())
					if seen[key] {
						continue
					}
					seen[key] = true
					loaded = append(loaded, obj)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to walk must-gather: %w", err)
			}
		}
	}
	if loaded == nil {
		loaded = []*unstructured.Unstructured{}
	}
	return loaded, nil
}

func (f *MustGatherFetcher) findDataRoots() ([]string, error) {
	var roots []string
	seen := make(map[string]bool)
	err := filepath.Walk(f.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (filepath.Base(path) == "cluster-scoped-resources" || filepath.Base(path) == "namespaces") {
			parent := filepath.Dir(path)
			if !seen[parent] {
				seen[parent] = true
				roots = append(roots, parent)
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking must-gather root %q for data directories: %w", f.rootDir, err)
	}
	return roots, nil
}

func loadResourcesFromFile(path string) ([]*unstructured.Unstructured, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	dec := yamlv3.NewDecoder(bytes.NewReader(data))
	var result []*unstructured.Unstructured
	for {
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("yaml decode %s: %w", path, err)
		}
		if raw == nil {
			continue
		}
		if raw["items"] != nil {
			kindStr, _ := raw["kind"].(string)
			if kindStr == "List" || strings.HasSuffix(kindStr, "List") {
				items, ok := raw["items"].([]any)
				if !ok {
					continue
				}
				for _, item := range items {
					itemMap, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if itemMap["kind"] == nil || itemMap["apiVersion"] == nil {
						continue
					}
					obj := &unstructured.Unstructured{Object: itemMap}
					result = append(result, obj)
				}
				continue
			}
		}
		if raw["kind"] != nil && raw["apiVersion"] != nil {
			obj := &unstructured.Unstructured{Object: raw}
			result = append(result, obj)
		}
	}
	return result, nil
}
