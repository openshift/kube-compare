package convert

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update .golden files")

var testDirs = "testdata"
var refYamlLocation = "reference/metadata.yaml"
var valuesFile = "values.yaml"
var defaultsDir = "defaults"
var resultDirName = "result"

type Test struct {
	name           string
	passDefaultDir bool
	passValuesFile bool
	helmVersion    string
	description    string
}

func (test *Test) getRefPath() string {
	return path.Join(testDirs, strings.ReplaceAll(test.name, " ", ""), refYamlLocation)
}

func (test *Test) getTestPath() string {
	return path.Join(testDirs, strings.ReplaceAll(test.name, " ", ""))
}

func (test *Test) getValuesPath() string {
	return path.Join(test.getTestPath(), valuesFile)
}

func (test *Test) getDefaultsPath() string {
	return path.Join(test.getTestPath(), defaultsDir)
}

func TestConvert(t *testing.T) {
	tests := []Test{
		{
			name: "Values Creation If Clause",
		},
		{
			name: "Values Creation Index",
		},
		{
			name: "Values Creation Range",
		},
		{
			name:        "Templates Are Created As Expected",
			helmVersion: "2",
			description: "Templates Are Created As Expected Test",
		},
		{
			name:        "Odd Filenames",
			helmVersion: "2",
			description: "Test escaping of odd or unexpected characters in reference filenames",
		},
		{
			name:           "Default Values Addition",
			passDefaultDir: true,
		},
		{
			name:           "Use Values File",
			passValuesFile: true,
		},
		{
			name:           "Values Contain Keys With Dots",
			passDefaultDir: true,
		},
		{
			name:           "Capturegroup Defaults",
			passValuesFile: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewCmd()
			dirName, err := os.MkdirTemp("", strings.ReplaceAll(test.name, " ", ""))
			defer os.RemoveAll(dirName)
			chartDir := path.Join(dirName, test.name)
			require.NoError(t, err)

			require.NoError(t, cmd.Flags().Set("helm-name", chartDir))
			require.NoError(t, cmd.Flags().Set("reference", test.getRefPath()))
			if test.passDefaultDir {
				require.NoError(t, cmd.Flags().Set("defaults", test.getDefaultsPath()))
			}
			if test.passValuesFile {
				require.NoError(t, cmd.Flags().Set("values", test.getValuesPath()))
			}
			if test.helmVersion != "" {
				require.NoError(t, cmd.Flags().Set("helm-version", test.helmVersion))
			}
			if test.description != "" {
				require.NoError(t, cmd.Flags().Set("description", test.description))
			}

			err = cmd.RunE(cmd, []string{})
			if err != nil {
				t.Fatalf("unexpected error occurred in test %s, error: %s", test.name, err)
			}

			resultDir := path.Join(test.getTestPath(), resultDirName)
			if *update {
				require.NoError(t, os.RemoveAll(resultDir))
				err = CopyDir(chartDir, resultDir)
				if err != nil {
					t.Fatalf("unexpected error occurred in test %s, error: %s", test.name, err)
				}
			}
			require.NoError(t, diffDirs(chartDir, resultDir))
		})
	}
}

// CopyDir recursively copies files from source to destination directory
func CopyDir(src, dst string) error {
	err := os.MkdirAll(dst, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	err = filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk path %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// Create subdirectories in the destination
			return os.MkdirAll(dstPath, os.ModePerm)
		}
		return copyFile(path, dstPath)
	})
	if err != nil {
		return fmt.Errorf("failed to walk source directory: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(srcFile, dstFile string) error {
	// Open source file
	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}
func diffDirs(dir1, dir2 string) error {
	cmd := exec.Command("diff", "-r", dir1, dir2)
	output, err := cmd.CombinedOutput()
	var exitError *exec.ExitError
	if err != nil {
		if errors.As(err, &exitError) {
			if exitError.ExitCode() == 1 {
				// Directories are different
				return fmt.Errorf("directories differ\n%s", output)
			}
		}
		return fmt.Errorf("failed to execute diff command: %w", err)
	}
	return nil
}
