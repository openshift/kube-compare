// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"fmt"
	"strings"
	"unicode"
)

// InlineDiffOption controls reference-only preprocessing before an inline diff.
type InlineDiffOption string

const (
	// IgnoreReferenceHashCommentLines ignores reference lines whose first
	// non-whitespace character is '#'.
	IgnoreReferenceHashCommentLines InlineDiffOption = "ignoreReferenceHashCommentLines"
	// IgnoreReferenceSlashCommentLines ignores reference lines whose first
	// non-whitespace characters are '//'.
	IgnoreReferenceSlashCommentLines InlineDiffOption = "ignoreReferenceSlashCommentLines"
)

var inlineDiffOptionOrder = []InlineDiffOption{
	IgnoreReferenceHashCommentLines,
	IgnoreReferenceSlashCommentLines,
}

// ValidateInlineDiffOptions validates the configured inline-diff options.
func ValidateInlineDiffOptions(options []InlineDiffOption) error {
	known := make(map[InlineDiffOption]bool, len(inlineDiffOptionOrder))
	for _, option := range inlineDiffOptionOrder {
		known[option] = true
	}

	seen := make(map[InlineDiffOption]bool, len(options))
	for index, option := range options {
		if !known[option] {
			return fmt.Errorf("inlineDiffOptions[%d] has unknown option %q", index, option)
		}
		if seen[option] {
			return fmt.Errorf("inlineDiffOptions[%d] duplicates option %q", index, option)
		}
		seen[option] = true
	}
	return nil
}

// NormalizeInlineDiffReference applies inline-diff options to the reference
// value only. It preserves all non-comment content and its original line ends.
func NormalizeInlineDiffReference(value string, options []InlineDiffOption) string {
	enabled := make(map[InlineDiffOption]bool, len(options))
	for _, option := range options {
		enabled[option] = true
	}

	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, line := range strings.SplitAfter(value, "\n") {
		trimmedLine := strings.TrimLeftFunc(line, unicode.IsSpace)
		ignore := false
		for _, option := range inlineDiffOptionOrder {
			if !enabled[option] {
				continue
			}
			switch option {
			case IgnoreReferenceHashCommentLines:
				ignore = strings.HasPrefix(trimmedLine, "#")
			case IgnoreReferenceSlashCommentLines:
				ignore = strings.HasPrefix(trimmedLine, "//")
			}
			if ignore {
				break
			}
		}
		if !ignore {
			normalized.WriteString(line)
		}
	}
	result := normalized.String()
	if !strings.HasSuffix(value, "\n") && strings.HasSuffix(result, "\n") {
		result = strings.TrimSuffix(result, "\n")
		result = strings.TrimSuffix(result, "\r")
	}
	return result
}
