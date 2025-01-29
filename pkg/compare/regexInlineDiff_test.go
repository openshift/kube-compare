package compare

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type RegexTestDiff struct {
	regex      string
	input      string
	expected   string
	initialCg  CapturedValues
	expectedCg CapturedValues
}

func TestInlineRegexDiff(t *testing.T) {
	tests := []RegexTestDiff{
		{
			regex:    "Hello",
			input:    "Hello",
			expected: "Hello",
		},
		{
			regex:    "H[e|i]llo",
			input:    "Hello",
			expected: "Hello",
		},
		{
			regex:    "Hello",
			input:    "bye",
			expected: "Hello",
		},
		{
			regex:    "He(llo)",
			input:    "Hello",
			expected: "Hello",
		},
		{
			regex:    "He(llo)",
			input:    "bye",
			expected: "He(llo)",
		},
		{
			regex:    "He(?<simple>llo)",
			input:    "Hello",
			expected: "Hello",
			expectedCg: CapturedValues{
				caps: map[string][]string{
					"simple": {"llo"},
				},
			},
		},
		{
			regex:    "He(?<simple>llo)",
			input:    "Bye",
			expected: "He(?<simple>llo)",
		},
		{
			regex:    "He(?<simple>llo), World",
			input:    "Hello, World",
			expected: "Hello, World",
			expectedCg: CapturedValues{
				caps: map[string][]string{
					"simple": {"llo"},
				},
			},
		},
		{
			regex: "He(?<simple>llo), World",
			input: "Hello, World",
			expected: "He(?<simple>=othermatch), World\n" +
				"WARNING: Capturegroup (?<simple>…) matched multiple values: « othermatch | llo »",
			initialCg: CapturedValues{
				caps: map[string][]string{
					"simple": {"othermatch"},
				},
			},
			expectedCg: CapturedValues{
				caps: map[string][]string{
					"simple": {"othermatch", "llo"},
				},
			},
		},
		{
			regex: "(?<simple>Hello), (?<simple>World)",
			input: "Hello, World",
			expected: "(?<simple>=Hello), (?<simple>=Hello)\n" +
				"WARNING: Capturegroup (?<simple>…) matched multiple values: « Hello | World »",
			expectedCg: CapturedValues{
				caps: map[string][]string{
					"simple": {"Hello", "World"},
				},
			},
		},
		{
			regex:    "Hello, (World)",
			input:    "Hello, Bob",
			expected: "Hello, (World)",
		},
		{
			regex:    "(Hello, (World))",
			input:    "Hello, World",
			expected: "Hello, World",
		},
		{
			regex:    "(World)",
			input:    "Hello World",
			expected: "World",
		},
		{
			regex:    "(Hello)",
			input:    "Hello World",
			expected: "Hello",
		},
		{
			regex: "(?<hello>H(?<nested>[a-z]+)) (?<world>W(?<nested>[a-z]+))",
			input: "Hello World",
			expected: "Hello World\n" +
				"WARNING: Capturegroup (?<nested>…) matched multiple values: « ello | orld »",
			expectedCg: CapturedValues{
				caps: map[string][]string{
					"hello":  {"Hello"},
					"world":  {"World"},
					"nested": {"ello", "orld"},
				},
			},
		},
	}

	inlineFunc := InlineDiffs["regex"]
	for _, test := range tests {
		t.Run(test.regex, func(t *testing.T) {
			actual, resultingCg := inlineFunc.Diff(test.regex, test.input, test.initialCg)
			require.Equal(t, test.expected, actual)
			require.Equal(t, test.expectedCg, resultingCg)
		})
	}
}
