package report

import (
	"flag"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/kube-compare/pkg/compare"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var update = flag.Bool("update", false, "update .golden files")

var TestDirs = "testdata"
var compareTestRefsDir = "../../../pkg/compare/testdata"

type Test struct {
	name         string
	referenceDir string
}

func (test *Test) getJSONPath() string {
	return path.Join(TestDirs, strings.ReplaceAll(test.name, " ", ""))
}

// TestCompareRun ensures that Run command calls the right actions
// and returns the expected result.
// The tests use the test references used for the compare command tests to make sure the reporter is up-to-date with
// the compare command reference and output format
func TestCompareRun(t *testing.T) {
	tests := []Test{
		{
			name:         "Diff Test Suite Creation When There Are Diffs",
			referenceDir: "RefWithTemplateFunctionsRendersAsExpected",
		},
		{
			name:         "Creation Of Missing CRs And Unmatched CRs And Diff Tests Suites When No Diffs",
			referenceDir: "AllRequiredTemplatesExistAndThereAreNoDiffs",
		},
		{
			name:         "Unmatched CRs Test Suite When CRs Are Unmatched",
			referenceDir: "OnlyResourcesThatWereNotMatchedBecauseMultipleMatchesAppearInSummary",
		},
		{
			name:         "Missing CRs test suite creation when CRS are Missing",
			referenceDir: "OnlyRequiredResourcesOfRequiredComponentAreReportedMissing(OptionalResourcesNotReported)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if *update {
				// The JSON output is regenerated from the references used in the compare tests:
				updateCompareOutput(t, test)
			}
			// crete temp dir to save report created by test
			cmd := NewCmd()
			dirName, err := os.MkdirTemp("", test.name)
			require.NoError(t, err)
			outputPath := path.Join(dirName, test.name)
			require.NoError(t, cmd.Flags().Set("output", outputPath))

			require.NoError(t, cmd.Flags().Set("json", test.getJSONPath()))

			err = cmd.RunE(cmd, []string{})
			if err != nil {
				t.Fatalf("unexpected error occurred in test %s, error: %s", test.name, err)
			}

			actualOutput, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("test %s failed reading the created report: %s", test.name, err)
			}
			getGoldenValue(t, path.Join(TestDirs, fmt.Sprintf("%s.golden", strings.ReplaceAll(" ", "", test.name))), removeInconsistentInfo(actualOutput))

		})
	}
}
func updateCompareOutput(t *testing.T, test Test) {
	t.Log("update test input to match current version of compare command")
	cmdutil.BehaviorOnFatal(func(str string, code int) {})

	tf := cmdtesting.NewTestFactory()
	IOStream, _, out, _ := genericiooptions.NewTestIOStreams()
	cmpCmd := compare.NewCmd(tf, IOStream)
	require.NoError(t, cmpCmd.Flags().Set("reference", path.Join(compareTestRefsDir, test.referenceDir, "reference")))
	require.NoError(t, cmpCmd.Flags().Set("filename", path.Join(compareTestRefsDir, test.referenceDir, "resources")))
	require.NoError(t, cmpCmd.Flags().Set("recursive", "true"))
	require.NoError(t, cmpCmd.Flags().Set("output", compare.Json))
	cmpCmd.Run(cmpCmd, []string{})
	if err := os.WriteFile(test.getJSONPath(), out.Bytes(), 0644); err != nil { // nolint:gocritic,gosec
		t.Fatalf("test %s failed to update test file: %s", test.getJSONPath(), err)
	}
}

func getGoldenValue(t *testing.T, fileName string, value []byte) {
	if *update {
		t.Log("update golden file")
		if err := os.WriteFile(fileName, value, 0644); err != nil { // nolint:gocritic,gosec
			t.Fatalf("test %s failed to update golden file: %s", fileName, err)
		}
	}
	expected, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("test %s failed reading .golden file: %s", fileName, err)
	}
	require.Equal(t, string(expected), string(value))
}

func removeInconsistentInfo(text []byte) []byte {
	// remove time and timestamp
	re := regexp.MustCompile("(?:time|timestamp)=\"(\\S*)\"")
	return re.ReplaceAll(text, []byte("TIME"))
}
