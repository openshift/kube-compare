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
	"github.com/openshift/kube-compare/pkg/testutils"
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
			name:         "Missing CRs test suite creation when CRS are Missing",
			referenceDir: "OnlyRequiredResourcesOfRequiredComponentAreReportedMissing(OptionalResourcesNotReported)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkCompatibilityWithCompareOutput(t, test, *update)
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
			value := testutils.GetFile(t, path.Join(TestDirs, fmt.Sprintf("%s.golden", strings.ReplaceAll(test.name, " ", ""))), removeInconsistentInfoFromReport(actualOutput), *update)
			require.Equal(t, removeInconsistentInfoFromReport(actualOutput), value)

		})
	}
}
func checkCompatibilityWithCompareOutput(t *testing.T, test Test, update bool) {
	cmdutil.BehaviorOnFatal(func(str string, code int) {
		if str != "" && str != compare.DiffsFoundMsg {
			t.Fatalf("kube-compare run failed; msg: '%s', code: %d", str, code)
		}
	})

	tf := cmdtesting.NewTestFactory()
	IOStream, _, out, _ := genericiooptions.NewTestIOStreams()
	cmpCmd := compare.NewCmd(tf, IOStream)
	require.NoError(t, cmpCmd.Flags().Set("reference", path.Join(compareTestRefsDir, test.referenceDir, "reference/metadata.yaml")))
	require.NoError(t, cmpCmd.Flags().Set("filename", path.Join(compareTestRefsDir, test.referenceDir, "resources")))
	require.NoError(t, cmpCmd.Flags().Set("recursive", "true"))
	require.NoError(t, cmpCmd.Flags().Set("output", compare.Json))
	cmpCmd.Run(cmpCmd, []string{})
	result := testutils.GetFile(t, test.getJSONPath(), testutils.RemoveInconsistentInfo(t, out.String()), update)
	require.Equal(t, result, testutils.RemoveInconsistentInfo(t, out.String()))
}

func removeInconsistentInfoFromReport(text []byte) string {
	re := regexp.MustCompile("(?:time|timestamp)=\"(\\S*)\"")
	return string(re.ReplaceAll(text, []byte("TIME")))
}
