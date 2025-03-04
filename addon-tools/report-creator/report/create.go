package report

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/openshift/kube-compare/pkg/compare"
	"github.com/openshift/kube-compare/pkg/junit"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	longDesc = templates.LongDesc(`
report-creator is a CLI tool that allows creating a JUnit test report from the output of the 'kubectl
cluster-compare' plugin. The command uses the JSON format output of the 'kubectl cluster-compare'
plugin. This tol can be handy in automatic test environments.

The tool divides the result of the cluster compare into 3 test suites:

1. Diff test suite - Each test in the suite represents a CR that is matched and diffed to a reference CR. The test will
be reported as failed if there are differences between the cluster cr and the expected CR.
The full diff will be included in the test case failure message. In case there are no differences
for the CR, the test will be marked as successful.

2. Missing CRs test suite - Each test in this suite represents a missing CR from the cluster that appeared
in the reference and was expected to appear in the cluster but wasn't found/identified.
If there are no missing CRs, this test suite will contain one successful test indicating that the cluster
contains all the expected CRs.

3. Unmatched CRs test suite - This suite includes all the CRs in the cluster that were not matched
to any reference. Each unmatched CR will be represented as a test case that failed.
If there are no unmatched CRs, then
this suite will include one successful test case representing that there are no unmatched CRs.
`)
)

func getParsed(raw string) (compare.Output, error) {
	output := compare.Output{}
	err := json.Unmarshal([]byte(raw), &output)
	if err != nil {
		return output, fmt.Errorf("failed to unmarshal json: %w", err)
	}
	return output, nil
}

type Options struct {
	compareOutputPath string
	outputFile        string
}

func NewCmd() *cobra.Command {
	options := Options{}
	cmd := &cobra.Command{
		Use:   "create-report -j <COMPARE_JSON_OUTPUT_PATH>",
		Short: "report-creator: A CLI tool for generating JUnit test reports from kubectl cluster-compare plugin output, categorizing results into diff, missing CRs, and unmatched CRs test suites.",
		Long:  longDesc,

		RunE: func(cmd *cobra.Command, args []string) error {
			jsonInput, err := os.ReadFile(options.compareOutputPath)
			if err != nil {
				return fmt.Errorf("failed to read comparison file: %w", err)
			}
			compareOutput, err := getParsed(string(jsonInput))
			if err != nil {
				return err
			}
			f, err := os.Create(options.outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)

			}
			defer f.Close()
			err = junit.Write(f, *compareOutput.JunitReport())
			if err != nil {
				return fmt.Errorf("failed to write junit report: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.compareOutputPath, "json", "j", "", "Path to the file including the json output of the cluster-compare command")
	cmd.Flags().StringVarP(&options.outputFile, "output", "o", "report.xml", "Path to save the report")
	return cmd
}
