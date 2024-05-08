package report

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/openshift/kube-compare/addon-tools/report-creator/junit"
	"github.com/openshift/kube-compare/pkg/compare"
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

// createDiffsSuite generates a JUnit test suite representing all differences found between cluster resources
// and expected reference CRs.
// The suite includes individual test cases for each cluster resource (CR) that exhibits differences.
// If differences are detected in a CR, a failure message is included in the test case including the full diff output.
func createDiffsSuite(output compare.Output) junit.TestSuite {
	diffSuite := junit.TestSuite{
		Name:      "Detected Differences Between Cluster CRs and Expected CRs",
		Timestamp: time.Now().Format(time.RFC3339),
		Time:      time.Now().Format(time.RFC3339),
		Tests:     len(*output.Diffs),
		Failures:  output.Summary.NumDiffCRs,
	}

	for _, diff := range *output.Diffs {
		testcase := junit.TestCase{
			Name:      fmt.Sprintf("CR: %s", diff.CRName),
			Classname: fmt.Sprintf("Matching Reference CR: %s", diff.CorrelatedTemplate),
		}

		if diff.DiffOutput != "None" {
			testcase.Failure = &junit.Failure{
				Type:     "Difference",
				Message:  fmt.Sprintf("Differences found in CR: %s, Compared To Refernce CR: %s", diff.CRName, diff.CorrelatedTemplate),
				Contents: diff.DiffOutput,
			}
		}

		diffSuite.TestCases = append(diffSuite.TestCases, testcase)
	}

	return diffSuite
}

// createMissingCRsSuite generates a JUnit test suite that ensures that all the expected CRs appear in the cluster.
// The suite includes test cases for each missing CR, categorized by their respective components and namespaces.
// If no CRs are missing, a single test case indicating that all expected CRs exist in the cluster is included.
func createMissingCRsSuite(summary compare.Summary) junit.TestSuite {
	suite := junit.TestSuite{
		Name:      "Missing Cluster Resources",
		Timestamp: time.Now().Format(time.RFC3339),
		Time:      time.Now().Format(time.RFC3339),
	}

	// Iterate over parts and components to add missing CRs as test cases
	for partName, partCRs := range summary.RequiredCRS {
		for componentName, componentCRs := range partCRs {
			for _, cr := range componentCRs {
				suite.TestCases = append(suite.TestCases, junit.TestCase{
					Name:      fmt.Sprintf("Missing CR: %s", cr),
					Classname: fmt.Sprintf("Part:%s Compomnet: %s", partName, componentName),
					Failure: &junit.Failure{
						Type:    "Missing Cluster CR",
						Message: fmt.Sprintf("Required CR by the reference %q is missing from cluster", cr),
					},
				})
			}
		}
	}
	sort.Slice(suite.TestCases, func(i, j int) bool {
		return suite.TestCases[i].Classname < suite.TestCases[j].Classname
	})

	// If no missing CRs are found, include a single test case indicating all expected CRs exist in the cluster
	if summary.NumMissing == 0 {
		suite.TestCases = append(suite.TestCases, junit.TestCase{
			Name: "All expected CRs exist in the cluster"})
		suite.Tests = 1
		return suite
	}
	suite.Tests = summary.NumMissing
	suite.Failures = summary.NumMissing

	return suite
}

// createUnmatchedSuite generates a JUnit test suite for representing unmatched cluster resources.
// The suite includes individual test cases for each unmatched CR.
// If no CRs are unmatched, a single test case indicating that all CRs are matched is included.
func createUnmatchedSuite(summary compare.Summary) junit.TestSuite {
	unmatchedSuite := junit.TestSuite{
		Name:      "Unmatched Cluster Resources",
		Timestamp: time.Now().Format(time.RFC3339),
		Time:      time.Now().Format(time.RFC3339),
	}

	// Iterate over unmatched CRs to add them as test cases
	for _, cr := range summary.UnmatchedCRS {
		unmatchedSuite.TestCases = append(unmatchedSuite.TestCases, junit.TestCase{
			Name: cr,
			Failure: &junit.Failure{
				Type:    "Unmatched CR",
				Message: fmt.Sprintf("Cluster resource '%s' is unmatched.", cr),
			},
		})
	}

	// If no unmatched CRs are found, include a single test case indicating all CRs are matched
	if len(summary.UnmatchedCRS) == 0 {
		unmatchedSuite.TestCases = append(unmatchedSuite.TestCases, junit.TestCase{
			Name: "All CLuster CRs are matched to reference CRs ",
		})
		unmatchedSuite.Tests = 1
		return unmatchedSuite
	}
	unmatchedSuite.Tests = len(summary.UnmatchedCRS)
	unmatchedSuite.Failures = len(summary.UnmatchedCRS)

	return unmatchedSuite
}

func createReport(output compare.Output) *junit.TestSuites {
	suites := junit.TestSuites{Name: "Comparison results of known valid reference configuration and a set of specific cluster CRs", Time: time.Now().Format(time.RFC3339), Suites: []junit.TestSuite{
		createDiffsSuite(output), createMissingCRsSuite(*output.Summary), createUnmatchedSuite(*output.Summary)}}
	for _, suite := range suites.Suites {
		suites.Tests += suite.Tests
		suites.Failures += suite.Failures
	}
	return &suites
}

func getParsed(raw string) (compare.Output, error) {
	output := compare.Output{}
	err := json.Unmarshal([]byte(raw), &output)
	if err != nil {
		return output, fmt.Errorf("failed to unmarshal json, %s", err)
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
				return err
			}
			compareOutput, err := getParsed(string(jsonInput))
			if err != nil {
				return err
			}
			f, err := os.Create(options.outputFile)
			if err != nil {
				return err

			}
			defer f.Close()
			err = junit.Write(f, *createReport(compareOutput))
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&options.compareOutputPath, "json", "j", "", "Path to the file including the json output of the cluster-compare command")
	cmd.Flags().StringVarP(&options.outputFile, "output", "o", "report.xml", "Path to save the report")
	return cmd
}
