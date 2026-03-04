// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// RefgenConfig is the root configuration for reference generation.
type RefgenConfig struct {
	APIVersion string         `json:"apiVersion"`
	OutputDir  string         `json:"outputDir"`
	Resources  []ResourceSpec `json:"resources"`
}

// ResourceSpec specifies a Kubernetes resource type to capture.
type ResourceSpec struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Required   bool     `json:"required"`
	Namespace  string   `json:"namespace,omitempty"`
	Names      []string `json:"names,omitempty"`
}

// LoadConfig loads and validates a refgen configuration file.
func LoadConfig(configPath string) (*RefgenConfig, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", configPath, err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuration file not found: %s", configPath)
		}
		return nil, fmt.Errorf("failed to read configuration: %w", err)
	}
	var config RefgenConfig
	if err := yaml.UnmarshalStrict(data, &config); err != nil {
		return nil, fmt.Errorf("invalid YAML in configuration file: %w", err)
	}
	if config.APIVersion == "" {
		config.APIVersion = "refgen/v1"
	}
	if config.OutputDir == "" {
		config.OutputDir = "./generated-reference"
	}
	if len(config.Resources) == 0 {
		return nil, fmt.Errorf("configuration must specify at least one resource")
	}
	return &config, nil
}
