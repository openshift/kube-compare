package compare

import (
	"testing"

	"github.com/openshift/kube-compare/pkg/junit"
	"github.com/stretchr/testify/assert"
)

type testExpectation struct {
	tests    int
	skips    int
	failures []string
}

func (expected testExpectation) matches(t *testing.T, actual junit.TestSuite) {
	assert.Equal(t, expected.tests, actual.Tests)
	assert.Equal(t, expected.tests, len(actual.TestCases))
	assert.Equal(t, len(expected.failures), actual.Failures)
	actualFailCount := 0
	for _, f := range actual.TestCases {
		if f.Failure != nil {
			assert.Contains(t, expected.failures, f.Failure.Contents)
			actualFailCount += 1
		}
	}
	assert.Equal(t, len(expected.failures), actualFailCount)
	assert.Equal(t, expected.skips, actual.Skipped)
	actualSkipCount := 0
	for _, s := range actual.TestCases {
		if s.SkipMessage != nil {
			actualSkipCount += 1
		}
	}
	assert.Equal(t, expected.skips, actualSkipCount)
}

func TestJunitDiffSuite(t *testing.T) {
	tests := []struct {
		name     string
		output   Output
		expected testExpectation
	}{
		{
			name: "Empty Output",
			output: Output{
				Summary: &Summary{},
				Diffs:   &[]DiffSum{},
			},
			expected: testExpectation{
				tests:    0,
				failures: []string{},
			},
		},
		{
			name: "No differences",
			output: Output{
				Summary: &Summary{},
				Diffs: &[]DiffSum{
					{
						CRName:             "crname",
						CorrelatedTemplate: "template",
						DiffOutput:         "",
					},
				},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{},
			},
		},
		{
			name: "Patched difference",
			output: Output{
				Summary: &Summary{},
				Diffs: &[]DiffSum{
					{
						CRName:             "crname",
						CorrelatedTemplate: "template",
						DiffOutput:         "",
						Patched:            "patched",
						OverrideReasons:    []string{"reason"},
					},
				},
			},
			expected: testExpectation{
				tests:    1,
				skips:    1,
				failures: []string{},
			},
		},
		{
			name: "One difference",
			output: Output{
				Summary: &Summary{},
				Diffs: &[]DiffSum{
					{
						CRName:             "crname",
						CorrelatedTemplate: "template",
						DiffOutput:         "difference",
					},
				},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{"difference"},
			},
		},
		{
			name: "One of each",
			output: Output{
				Summary: &Summary{},
				Diffs: &[]DiffSum{
					{
						CRName:             "crname",
						CorrelatedTemplate: "template",
						DiffOutput:         "",
					},
					{
						CRName:             "crname",
						CorrelatedTemplate: "template",
						DiffOutput:         "",
						Patched:            "patched",
						OverrideReasons:    []string{"reason"},
					},
					{
						CRName:             "crname",
						CorrelatedTemplate: "template",
						DiffOutput:         "difference",
					},
				},
			},
			expected: testExpectation{
				tests:    3,
				skips:    1,
				failures: []string{"difference"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.output.junitDiffSuite()
			test.expected.matches(t, actual)
		})
	}
}

func TestJunitValidationIssueSuite(t *testing.T) {
	tests := []struct {
		name     string
		output   Output
		expected testExpectation
	}{
		{
			name: "Empty input",
			output: Output{
				Summary: &Summary{},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{},
			},
		},
		{
			name: "Missing CR",
			output: Output{
				Summary: &Summary{
					ValidationIssues: map[string]map[string]ValidationIssue{
						"group": {
							"part": {
								Msg: "message",
								CRs: []string{"crname"},
							},
						},
					},
					NumMissing: 1,
				},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{""},
			},
		},
		{
			name: "Validation issue that is not a missing CR",
			output: Output{
				Summary: &Summary{
					ValidationIssues: map[string]map[string]ValidationIssue{
						"group": {
							"part": {
								Msg: "message",
								CRs: []string{"crname"},
							},
						},
					},
					NumMissing: 0,
				},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{""},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.output.junitValidationIssueSuite()
			test.expected.matches(t, actual)
		})
	}
}

func TestJunitUnmatchedCRsSuite(t *testing.T) {
	tests := []struct {
		name     string
		output   Output
		expected testExpectation
	}{
		{
			name: "Empty input",
			output: Output{
				Summary: &Summary{},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{},
			},
		},
		{
			name: "One unmatched CR",
			output: Output{
				Summary: &Summary{
					UnmatchedCRS: []string{"one"},
				},
			},
			expected: testExpectation{
				tests:    1,
				failures: []string{""},
			},
		},
		{
			name: "Two unmatched CRs",
			output: Output{
				Summary: &Summary{
					UnmatchedCRS: []string{"one", "two"},
				},
			},
			expected: testExpectation{
				tests:    2,
				failures: []string{"", ""},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.output.junitUnmatchedCRsSuite()
			test.expected.matches(t, actual)
		})
	}
}
