package main_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	main "github.com/openshift/kube-compare/addon-tools/generate-metadata"
	logging "github.com/openshift/kube-compare/addon-tools/generate-metadata/pkg"
	"github.com/stretchr/testify/require"
)

type Level string

const (
	Error Level = "error"
	Fatal Level = "fatal"
	Exit  Level = "exit"
)

type TestBuffers struct {
	Error *bytes.Buffer
	Fatal *bytes.Buffer
	Exit  *bytes.Buffer
}

func (tb *TestBuffers) getBufferForLevel(level Level) *bytes.Buffer {
	switch level {
	case Error:
		return tb.Error
	case Fatal:
		return tb.Fatal
	case Exit:
		return tb.Exit
	}
	return nil
}

func testLogger(buffers TestBuffers) *logging.Logger {
	return &logging.Logger{
		Error: func(args ...any) {
			fmt.Fprint(buffers.Error, args)
		},
		Errorf: func(format string, args ...any) {
			fmt.Fprintf(buffers.Error, format, args)
		},
		Fatal: func(args ...any) {
			fmt.Fprint(buffers.Fatal, args)
		},
		Fatalf: func(format string, args ...any) {
			fmt.Fprintf(buffers.Fatal, format, args)
		},
		Exit: func(args ...any) {
			fmt.Fprint(buffers.Exit, args)
		},
		Exitf: func(format string, args ...any) {
			fmt.Fprintf(buffers.Exit, format, args)
		},
	}
}

type check interface {
	CheckLogs(t *testing.T, tb TestBuffers)
	CheckManifest(t *testing.T, out []byte)
	WithDir(dir string)
}

type CheckErrorLog struct {
	strValue string
	// re       *regexp.Regexp
	level Level
}

func (c *CheckErrorLog) WithDir(dir string)                        {}
func (c *CheckErrorLog) CheckManifest(t *testing.T, actual []byte) {}
func (c *CheckErrorLog) CheckLogs(t *testing.T, tb TestBuffers) {
	if c.strValue != "" {
		buf := tb.getBufferForLevel(c.level)
		require.Contains(t, buf.String(), c.strValue)
	}
}

type CheckExpectedManifest struct {
	path string
}

func (c *CheckExpectedManifest) CheckLogs(t *testing.T, tb TestBuffers) {}
func (c *CheckExpectedManifest) WithDir(dir string) {
	c.path = filepath.Join(dir, "expected.yaml")
}
func (c *CheckExpectedManifest) CheckManifest(t *testing.T, actual []byte) {
	expected, err := os.ReadFile(c.path)
	require.NoError(t, err, "Failed to load expected file %s", c.path)
	require.YAMLEq(t, string(expected), string(actual), "Failed to match expected and actual output")
}

func testRun(t *testing.T, dir string, exitOnError bool, checks []check) {

	out := new(bytes.Buffer)
	tb := TestBuffers{
		Error: new(bytes.Buffer),
		Fatal: new(bytes.Buffer),
		Exit:  new(bytes.Buffer),
	}
	logging.SetLogger(testLogger(tb))
	main.InitLogger()
	main.GenerateMataData(out, filepath.Join(dir, "reference"), exitOnError)
	for _, c := range checks {
		c.WithDir(dir)
		c.CheckLogs(t, tb)
		c.CheckManifest(t, out.Bytes())
	}
}

func TestDifferentInputs(t *testing.T) {
	checkExpectedManifest := &CheckExpectedManifest{}
	tests := []struct {
		name        string
		dir         string
		exitOnError bool
		checks      []check
	}{
		{
			name:        "Check exit and error when conflicting component found with exitOnError",
			dir:         "testdata/all_labels_conflicting_component_requirment",
			exitOnError: true,
			checks: []check{
				&CheckErrorLog{
					level:    Fatal,
					strValue: "failed to generate valid metadata file: conflicting component required status",
				},
			},
		},
		{
			name:   "Check exit and error when conflicting component found with exitOnError",
			dir:    "testdata/all_labels_conflicting_component_requirment",
			checks: []check{checkExpectedManifest},
		},
		{
			name:   "All labels All required",
			dir:    "testdata/all_labels_all_required",
			checks: []check{checkExpectedManifest},
		},
		{
			name:   "All labels required and optional templates",
			dir:    "testdata/all_labels_required_and_optional",
			checks: []check{checkExpectedManifest},
		},
		{
			name:   "all_labels_all_required_subdir",
			dir:    "testdata/all_labels_all_required_subdir",
			checks: []check{checkExpectedManifest},
		},
		{
			name:   "no_labels",
			dir:    "testdata/no_labels",
			checks: []check{checkExpectedManifest},
		},
		{
			name:   "no_labels_split_groups",
			dir:    "testdata/no_labels_split_groups",
			checks: []check{checkExpectedManifest},
		},
		{
			name:   "no_labels_split_parts",
			dir:    "testdata/no_labels_split_parts",
			checks: []check{checkExpectedManifest},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testRun(t, test.dir, test.exitOnError, test.checks)
		})
	}
}
