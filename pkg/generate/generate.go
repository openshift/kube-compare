// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// Options holds options for the generate command.
type Options struct {
	GenerateConfig string
	OutputDir      string
	MustGatherDir  string
	Verbose        bool

	Factory kcmdutil.Factory
	Streams genericiooptions.IOStreams
}

// Run executes the generate command: fetches resources and writes reference files.
func (o *Options) Run(ctx context.Context) error {
	config, err := LoadConfig(o.GenerateConfig)
	if err != nil {
		return err
	}
	klog.V(1).Infof("Loaded configuration from %s", o.GenerateConfig)
	klog.V(1).Infof("  Resources to capture: %d", len(config.Resources))

	outputDir := o.OutputDir
	if outputDir == "" {
		outputDir = config.OutputDir
	}

	var fetcher Fetcher
	if o.MustGatherDir != "" {
		klog.V(1).Infof("Using must-gather directory: %s", o.MustGatherDir)
		fetcher, err = NewMustGatherFetcher(o.MustGatherDir)
		if err != nil {
			return err
		}
	} else {
		klog.V(1).Infof("Connected to Kubernetes cluster")
		fetcher, err = NewClusterFetcher(o.Factory)
		if err != nil {
			return err
		}
	}

	resourcesBySpec := make(map[*ResourceSpec][]*unstructured.Unstructured)
	var totalResources int
	var missingSpecs []*ResourceSpec

	for i := range config.Resources {
		spec := &config.Resources[i]
		nsInfo := ""
		if spec.Namespace != "" {
			nsInfo = fmt.Sprintf(" in namespace %s", spec.Namespace)
		}
		klog.V(1).Infof("Fetching %s (%s)%s...", spec.Kind, spec.APIVersion, nsInfo)
		resources, err := fetcher.FetchResources(ctx, spec)
		if err != nil {
			if !spec.Required {
				klog.Warningf("failed to fetch optional resource %s (%s): %v", spec.Kind, spec.APIVersion, err)
				resourcesBySpec[spec] = nil
				missingSpecs = append(missingSpecs, spec)
				continue
			}
			return fmt.Errorf("failed to fetch %s: %w", spec.Kind, err)
		}
		resourcesBySpec[spec] = resources
		totalResources += len(resources)
		if len(resources) == 0 {
			missingSpecs = append(missingSpecs, spec)
		}
		klog.V(1).Infof("  Found %d resource(s)", len(resources))
	}

	generator := NewGenerator(config, outputDir)
	outputPath, err := generator.Generate(resourcesBySpec)
	if err != nil {
		return err
	}

	fmt.Fprintf(o.Streams.Out, "Generated reference at: %s\n", outputPath)
	fmt.Fprintf(o.Streams.Out, "  Total resources captured: %d\n", totalResources)
	capturedTypes := 0
	for _, resources := range resourcesBySpec {
		if len(resources) > 0 {
			capturedTypes++
		}
	}
	fmt.Fprintf(o.Streams.Out, "  Resource types: %d\n", capturedTypes)
	if len(missingSpecs) > 0 {
		fmt.Fprintf(o.Streams.ErrOut, "Warning: No resources found for:\n")
		for _, spec := range missingSpecs {
			details := fmt.Sprintf("%s (%s)", spec.Kind, spec.APIVersion)
			if spec.Namespace != "" {
				details = fmt.Sprintf("%s in namespace %s", details, spec.Namespace)
			}
			if len(spec.Names) > 0 {
				details = fmt.Sprintf("%s with names %v", details, spec.Names)
			}
			fmt.Fprintf(o.Streams.ErrOut, "  - %s\n", details)
		}
	}
	return nil
}
