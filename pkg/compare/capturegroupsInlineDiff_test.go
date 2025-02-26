package compare

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapturegroupIndex(t *testing.T) {
	tests := []struct {
		pattern  string
		expected []string
	}{
		{"", []string{}},
		{"Text with no capture groups!", []string{}},
		{"[a-z]+(looks)?(like)?(regex)?", []string{}},
		{"Incomplete (?<bad_name", []string{}},
		{"Incomplete (?<name_escape\\>bad)", []string{}},
		{"Incomplete (?<no_end>[a-z]+", []string{}},
		{"Incomplete (?<false end>[0-9()]+", []string{}},
		{"Incomplete (?<escaped end>here\\)", []string{}},
		{"(?<simple_group>.*)", []string{"(?<simple_group>.*)"}},
		{"  (?<simple_group>.*)  ", []string{"(?<simple_group>.*)"}},
		{"(?<tricky_group>[^)(]*)", []string{"(?<tricky_group>[^)(]*)"}},
		{"(?<escape_group>.*\\).*)", []string{"(?<escape_group>.*\\).*)"}},
		{"(?<cclass_group>[[:alpha:]]+)", []string{"(?<cclass_group>[[:alpha:]]+)"}},
		{"[text before]((?<hidden_group>.*))[text after]", []string{"(?<hidden_group>.*)"}},
		{"(?<group_with_groups>(?<inner1>.*(?<inner2>.*))?)", []string{"(?<group_with_groups>(?<inner1>.*(?<inner2>.*))?)"}},
		{"(?<one>.*)(?<two>.*)", []string{"(?<one>.*)", "(?<two>.*)"}},
		{"Two groups (?<first>.*) in a (?<second>.*) string", []string{"(?<first>.*)", "(?<second>.*)"}},
	}
	for _, c := range tests {
		t.Run(fmt.Sprintf("Pattern %q", c.pattern), func(t *testing.T) {
			matches := CapturegroupIndex(c.pattern)
			require.Equal(t, len(c.expected), len(matches), fmt.Sprintf("Result length match for %q", c.pattern))
			for i, m := range matches {
				t.Run(fmt.Sprintf("Group %d:%d", i+1, len(matches)), func(t *testing.T) {
					expected := c.expected[i]
					assert.Equal(t, expected, c.pattern[m.Start:m.End], fmt.Sprintf("Expected capture group %d", i))
					nameEnd := strings.Index(expected, ">")
					expectedName := expected[3:nameEnd]
					assert.Equal(t, expectedName, m.Name, fmt.Sprintf("Expected capture group %d name match", i))
					assert.Equal(t, expected, m.Full, fmt.Sprintf("Expected full capturegroup %d", i))
				})
			}
		})
	}
}

func TestCapturegroupQuoteMeta(t *testing.T) {
	tests := []struct {
		pattern  string
		expected string
	}{
		{"", ""},
		{"plain text", "plain text"},
		{"[a-z]+(looks)?(like)?(regex)?", "\\[a-z\\]\\+\\(looks\\)\\?\\(like\\)\\?\\(regex\\)\\?"},
		{"(?<simple_group>.*)", "^(?<simple_group>.*)$"},
		{"[(?<group_in_brackets>[^\\]]+)]", "\\[(?<group_in_brackets>[^\\]]+)\\]"},
		{" (?<simple_group>.*)", " \\b(?<simple_group>.*)$"},
		{"(?<simple_group>.*) ", "^(?<simple_group>.*)\\b "},
		{"Text around a (?<simple_group>.*) with another (?<end_group>.*)", "Text around a \\b(?<simple_group>.*)\\b with another \\b(?<end_group>.*)$"},
	}
	for _, c := range tests {
		t.Run(fmt.Sprintf("Pattern %q", c.pattern), func(t *testing.T) {
			actual := CapturegroupQuoteMeta(c.pattern, CapturegroupIndex(c.pattern))
			assert.Equal(t, c.expected, actual)
		})
	}
}

func mlString(lines []string) string {
	return strings.Join(lines, "\n")
}

func TestCapturegroupsDiff(t *testing.T) {
	type Case struct {
		message  string
		value    []string
		expected []string
	}
	suites := []struct {
		message string
		pattern []string
		cases   []Case
	}{
		{
			message: "Empty pattern",
			pattern: []string{""},
			cases: []Case{
				{
					message:  "empty value",
					expected: []string{""},
				},
				{
					message:  "nonempty value",
					value:    []string{"something"},
					expected: []string{""},
				},
			},
		},
		{
			message: "No capturegroups",
			pattern: []string{"one", "two", "three"},
			cases: []Case{
				{
					message:  "empty value",
					expected: []string{"one", "two", "three"},
				},
				{
					message:  "matching value",
					value:    []string{"one", "two", "three"},
					expected: []string{"one", "two", "three"},
				},
				{
					message:  "mismatched value",
					value:    []string{"phone", "a two is here", "threenager"},
					expected: []string{"one", "two", "three"},
				},
			},
		},
		{
			message: "One capturegroup",
			pattern: []string{"one", "(?<g1>[a-z]+)", "three"},
			cases: []Case{
				{
					message:  "empty value",
					expected: []string{"one", "(?<g1>[a-z]+)", "three"},
				},
				{
					message:  "mismatches pattern",
					value:    []string{"one", "2two2", "three"},
					expected: []string{"one", "(?<g1>[a-z]+)", "three"},
				},
				{
					message:  "matching pattern",
					value:    []string{"one", "two", "three"},
					expected: []string{"one", "two", "three"},
				},
			},
		},
		{
			message: "One capturegroup allowing spaces",
			pattern: []string{"one", "(?<g1>[a-z\\s]+)", "three"},
			cases: []Case{
				{
					message:  "empty value",
					expected: []string{"one", "(?<g1>[a-z\\s]+)", "three"},
				},
				{
					message:  "mismatches pattern",
					value:    []string{"one", "2two2", "three"},
					expected: []string{"one", "(?<g1>[a-z\\s]+)", "three"},
				},
				{
					message:  "matching pattern",
					value:    []string{"one", "two point five", "three"},
					expected: []string{"one", "two point five", "three"},
				},
			},
		},
		{
			message: "Two different capturegroups",
			pattern: []string{"Line one", "Line (?<g1>[a-z\\s]+) two (?<g2>[a-z]+)", "Line three"},
			cases: []Case{
				{
					message:  "empty value",
					expected: []string{"Line one", "Line (?<g1>[a-z\\s]+) two (?<g2>[a-z]+)", "Line three"},
				},
				{
					message:  "mismatches pattern",
					value:    []string{"one", "two", "three"},
					expected: []string{"Line one", "Line (?<g1>[a-z\\s]+) two (?<g2>[a-z]+)", "Line three"},
				},
				{
					message:  "mismatches only regexes",
					value:    []string{"Line one", "Line two", "Line three"},
					expected: []string{"Line one", "Line (?<g1>[a-z\\s]+) two (?<g2>[a-z]+)", "Line three"},
				},
				{
					message: "mismatches 1/2 pattern",
					value:   []string{"Line one", "Line a two 42", "Line three"},
					// TODO: Perhaps we could engineer a way to match the first 'a'?
					expected: []string{"Line one", "Line (?<g1>[a-z\\s]+) two (?<g2>[a-z]+)", "Line three"},
				},
				{
					message:  "matching pattern",
					value:    []string{"Line one", "Line a two b", "Line three"},
					expected: []string{"Line one", "Line a two b", "Line three"},
				},
				{
					message:  "matching pattern with spaces",
					value:    []string{"Line one", "Line a a a two b", "Line three"},
					expected: []string{"Line one", "Line a a a two b", "Line three"},
				},
			},
		},
		{
			message: "Two identical capturegroups",
			pattern: []string{"Line one", "Line (?<g1>[a-z\\s]+) two (?<g1>[a-z\\s]+)", "Line (?<g1>[a-z\\s]+)"},
			cases: []Case{
				{
					message:  "matching pattern identically",
					value:    []string{"Line one", "Line a a two a a", "Line a a"},
					expected: []string{"Line one", "Line a a two a a", "Line a a"},
				},
				{
					message: "matching pattern differently each time",
					value:   []string{"Line one", "Line a a two b", "Line three"},
					expected: []string{"Line one", "Line (?<g1>=a a) two (?<g1>=a a)", "Line (?<g1>=a a)",
						"WARNING: Capturegroup (?<g1>…) matched multiple values: « a a | b | three »",
					},
				},
			},
		},
		{
			message: "Nested capture groups",
			pattern: []string{"(?<hello>H(?<nested>[a-z]+)) (?<world>W(?<nested>[a-z]+))"},
			cases: []Case{
				{
					message:  "matching sub pattern",
					value:    []string{"Hello Wello"},
					expected: []string{"Hello Wello"},
				},
				{
					message:  "different sub pattern",
					value:    []string{"Hello World"},
					expected: []string{"Hello World", "WARNING: Capturegroup (?<nested>…) matched multiple values: « ello | orld »"},
				},
			},
		},
	}
	for _, s := range suites {
		t.Run(s.message, func(t *testing.T) {
			for _, c := range s.cases {
				t.Run(c.message, func(t *testing.T) {
					cg := CapturegroupsInlineDiff{}
					actual := cg.Diff(mlString(s.pattern), mlString(c.value))
					assert.Equal(t, mlString(c.expected), actual)
				})
			}
		})
	}
}
