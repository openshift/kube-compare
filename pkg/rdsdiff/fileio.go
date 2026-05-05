// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"fmt"
	"os"
	"path/filepath"
)

// readCleanFile reads a file after cleaning the path to prevent path traversal.
func readCleanFile(name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Clean(name)) // #nosec G304 -- path cleaned
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return data, nil
}

// writeCleanFile writes data to a file after cleaning the path to prevent path traversal.
func writeCleanFile(name string, data []byte) error {
	if err := os.WriteFile(filepath.Clean(name), data, 0o600); err != nil { // #nosec G304 -- path cleaned
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// readCleanDir reads a directory after cleaning the path to prevent path traversal.
func readCleanDir(name string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(filepath.Clean(name))
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", name, err)
	}
	return entries, nil
}
