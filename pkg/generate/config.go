// SPDX-License-Identifier:Apache-2.0

package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// validAPIVersions lists supported RefgenConfig.apiVersion values.
var validAPIVersions = []string{
	"refgen/v1",
}

// RefgenConfig is the root configuration for reference generation.
type RefgenConfig struct {
	APIVersion string `json:"apiVersion"`
	OutputDir  string `json:"outputDir"`
	// OmitAnnotations lists metadata.annotation keys stripped from captured manifests
	// and added to generated metadata.yaml fieldsToOmit (in addition to built-in defaults).
	OmitAnnotations []string `json:"omitAnnotations,omitempty"`
	// OmitLabels lists metadata.labels keys stripped from captured manifests and
	// added to fieldsToOmit defaults (in addition to built-in defaults).
	OmitLabels []string       `json:"omitLabels,omitempty"`
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
	allowedAPIVersion := false
	for _, v := range validAPIVersions {
		if config.APIVersion == v {
			allowedAPIVersion = true
			break
		}
	}
	if !allowedAPIVersion {
		return nil, fmt.Errorf("configuration apiVersion %q is invalid; must be one of: %s", config.APIVersion, strings.Join(validAPIVersions, ", "))
	}
	if config.OutputDir == "" {
		config.OutputDir = "./generated-reference"
	}
	if len(config.Resources) == 0 {
		return nil, fmt.Errorf("configuration must specify at least one resource")
	}
	for i, k := range config.OmitAnnotations {
		if err := validateOmitKey(k, "omitAnnotations", i); err != nil {
			return nil, err
		}
	}
	for i, k := range config.OmitLabels {
		if err := validateOmitKey(k, "omitLabels", i); err != nil {
			return nil, err
		}
	}
	return &config, nil
}

func validateOmitKey(key, field string, index int) error {
	if key == "" {
		return fmt.Errorf("%s[%d]: key must not be empty", field, index)
	}
	if strings.Contains(key, `"`) {
		return fmt.Errorf("%s[%d]: key must not contain double quotes", field, index)
	}
	return nil
}
