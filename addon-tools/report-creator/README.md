# report-creator add-on

report-creator is a CLI tool that allows creating a JUnit test report from the
output of the 'kubectl cluster-compare' plugin. The command uses the JSON
format output of the 'kubectl cluster-compare' plugin. This tol can be handy in
automatic test environments.

The tool divides the result of the cluster compare into 3 test suites:

1. Diff test suite - Each test in the suite represents a CR that is matched and
   diffed to a reference CR. The test will be reported as failed if there are
   differences between the cluster cr and the expected CR. The full diff will
   be included in the test case failure message. In case there are no
   differences for the CR, the test will be marked as successful.
2. Missing CRs test suite - Each test in this suite represents a missing CR
   from the cluster that appeared in the reference and was expected to appear
   in the cluster but wasn't found/identified. If there are no missing CRs,
   this test suite will contain one successful test indicating that the cluster
   contains all the expected CRs.
3. Unmatched CRs test suite - This suite includes all the CRs in the cluster
   that were not matched to any reference. Each unmatched CR will be
   represented as a test case that failed. If there are no unmatched CRs, then
   this suite will include one successful test case representing that there are
   no unmatched CRs.

## Usage

```txt
report-creator -j <COMPARE_JSON_OUTPUT_PATH> [flags]

Flags
  -h, --help            help for report-creator
  -j, --json string     Path to the file including the json output of the cluster-compare command
  -o, --output string   Path to save the report (default "report.xml")
```
