package convert

import (
	"bytes"
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
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
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
		{
			name:           "Lookup Substitution",
			passValuesFile: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewCmd()
			dirName, err := os.MkdirTemp("", strings.ReplaceAll(test.name, " ", ""))
			defer func() {
				if err := os.RemoveAll(dirName); err != nil {
					t.Errorf("failed to clean up temporary directory: %v", err)
				}
			}()
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
func copyFile(srcFile, dstFile string) (err error) {
	// Open source file
	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close source file: %w", cerr)
		}
	}()

	out, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close destination file: %w", cerr)
		}
	}()

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

func checkLookups(t *testing.T, expected, result []Lookup) {
	assert.Equal(t, len(expected), len(result), "Lookup count")
	if len(result) > 0 {
		for i, e := range expected {
			r := result[i]
			assert.Equal(t, e.Key, r.Key, "Lookup.Key")
			assert.Equal(t, e.Array, r.Array, "Lookup.Array")
		}
	}
}

func TestFindLookups(t *testing.T) {
	tests := []struct {
		inputs   []string
		expected []Lookup
	}{
		{
			inputs: []string{
				"",
				" nothing here ",
				"lookupCR",
				"lookupCR incomplete",
				"lookupCR incomplete three arguments",
				`lookupCR "two words" (Unterminated parentheses" "arg(" "end"`,
				`lookupCR "two words" "Unterminated quoted string\" with some text`,
			},
			expected: []Lookup{},
		},
		{
			inputs: []string{
				"lookupCR a b c d",
				`lookupCR a b
					c
					d`,
				`lookupCR "a" (b) "c" (d)`,
				"Text before lookupCR a b c d and after",
			},
			expected: []Lookup{
				{
					Key: "lookupCR_a_b_c_d",
				},
			},
		},
		{
			inputs: []string{
				"lookupCRs a b c d",
				`lookupCRs a b
					c
					d`,
				`lookupCRs "a" (b) "c" (d)`,
				"Text before lookupCRs a b c d and after",
			},
			expected: []Lookup{
				{
					Key:   "lookupCRs_a_b_c_d",
					Array: true,
				},
			},
		},
		{
			inputs: []string{
				`lookupCR "two words" (template function "with args") "arg(" ")end"`,
				`lookupCR "two words" (template (function) "with \"args") "arg(" ")end\""`,
			},
			expected: []Lookup{
				{
					Key: "lookupCR_two_words_template_function_with_args_arg_end",
				},
			},
		},
		{
			inputs: []string{
				`{{- $objlist := lookupCRs
						"apps/v1"
						"ConfigMap"
						"default"
						"*"
				}}`,
			},
			expected: []Lookup{
				{
					Key:   "lookupCRs_apps_v1_ConfigMap_default",
					Array: true,
				},
			},
		},
		{
			inputs: []string{
				`{{-$obj:=lookupCR "apps/v1" "ConfigMap" "default" "cm1"}}`,
			},
			expected: []Lookup{
				{
					Key: "lookupCR_apps_v1_ConfigMap_default_cm1",
				},
			},
		},
	}
	for _, test := range tests {
		for _, input := range test.inputs {
			t.Run(fmt.Sprintf("Testing lookup for %q", input), func(t *testing.T) {
				result := findLookups(input)
				checkLookups(t, test.expected, result)
			})
		}
	}
}

// splitYAMLDocuments splits a rendered manifest into its non-empty YAML
// documents. Only a line that is exactly "---" is treated as a separator, so a
// resource glued onto the previous resource's final line (e.g. "value---")
// would not be split off and the document count would be wrong.
func splitYAMLDocuments(manifest string) []string {
	var docs []string
	var cur []string
	flush := func() {
		doc := strings.Join(cur, "\n")
		if strings.TrimSpace(doc) != "" {
			docs = append(docs, doc)
		}
		cur = nil
	}
	for _, line := range strings.Split(manifest, "\n") {
		if line == "---" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return docs
}

// TestMultiEntryRendering guards the range wrapper emitted by
// convertToHelmTemplate: when .Values.<component> holds several entries each one
// must render as its own, cleanly separated YAML document, and a single entry
// must not leave a trailing blank line. Helm executes chart templates with
// text/template (plus Sprig helpers), so rendering the generated template with a
// text/template engine reproduces Helm's whitespace handling faithfully; the
// wrapper only relies on the list/dict helpers, which we supply below.
func TestMultiEntryRendering(t *testing.T) {
	cmd := NewCmd()
	dirName, err := os.MkdirTemp("", "MultipleCRInstances")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(dirName); err != nil {
			t.Errorf("failed to clean up temporary directory: %v", err)
		}
	}()

	chartDir := path.Join(dirName, "chart")
	require.NoError(t, cmd.Flags().Set("helm-name", chartDir))
	require.NoError(t, cmd.Flags().Set("reference", path.Join(testDirs, "MultipleCRInstances", refYamlLocation)))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	tmplBytes, err := os.ReadFile(path.Join(chartDir, helmTemplatesDir, "cm.yaml"))
	require.NoError(t, err)

	funcs := template.FuncMap{
		"list": func(items ...any) []any { return items },
		"dict": func(pairs ...any) map[string]any {
			d := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				d[fmt.Sprint(pairs[i])] = pairs[i+1]
			}
			return d
		},
	}
	tmpl, err := template.New("cm").Funcs(funcs).Parse(string(tmplBytes))
	require.NoError(t, err)

	render := func(names ...string) string {
		instances := make([]any, 0, len(names))
		for _, n := range names {
			instances = append(instances, map[string]any{
				"metadata": map[string]any{"name": n, "namespace": "ns-" + n},
			})
		}
		var buf bytes.Buffer
		require.NoError(t, tmpl.Execute(&buf, map[string]any{
			"Values": map[string]any{"cm": instances},
		}))
		return buf.String()
	}

	t.Run("multiple entries render as separate documents", func(t *testing.T) {
		out := render("first", "second", "third")

		// Every "---" separator must start on its own line so a resource is never
		// glued onto the previous resource's final line (which would be invalid
		// YAML).
		assert.NotRegexp(t, `\S---`, out, "found content glued to a --- separator")
		assert.NotContains(t, out, "\n\n---", "found a blank line before a --- separator")

		docs := splitYAMLDocuments(out)
		require.Len(t, docs, 3, "expected one document per entry")
		for i, name := range []string{"first", "second", "third"} {
			var cm map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(docs[i]), &cm))
			assert.Equal(t, "ConfigMap", cm["kind"])
			meta, ok := cm["metadata"].(map[string]any)
			require.True(t, ok, "document %d has no metadata map", i)
			assert.Equal(t, name, meta["name"])
			assert.Equal(t, "ns-"+name, meta["namespace"])
		}
	})

	t.Run("single entry has no trailing blank line", func(t *testing.T) {
		out := render("only")

		docs := splitYAMLDocuments(out)
		require.Len(t, docs, 1)

		// The wrapper trims its own trailing whitespace, so the rendered output
		// must not end with a blank line (issue #308).
		assert.False(t, strings.HasSuffix(out, "\n\n"), "unexpected trailing blank line")
		assert.NotContains(t, out, "\n \n", "unexpected whitespace-only line")
	})
}
