package compare

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"k8s.io/klog/v2"
)

const (
	capturegroups inlineDiffType = "capturegroups"
)

type CapturegroupsInlineDiff struct{}

type diffInfo struct {
	dmp   *diffmatchpatch.DiffMatchPatch
	diffs []diffmatchpatch.Diff
	caps  map[string][]string
}

func (id *diffInfo) addCapture(name, value string) {
	if id.caps == nil {
		id.caps = make(map[string][]string)
	}
	if !slices.Contains(id.caps[name], value) {
		id.caps[name] = append(id.caps[name], value)
	}
}

type CgInfo struct {
	Name  string
	Start int
	End   int
}

// Options for development purposes to test alternative implementations

// If true, use a line-granular diff.
// Otherwise, do a word-granular diff.
var diffByLines = false

// If true, add string-end anchors to the entire pattern when quoted.
// Otherwise only do so when a capture group begins or ends the string.
var quoteEscapeFull = false

// Return a list of the valid-looking capturegroup indices within the given pattern string.
// Each inner list is a tuple of start:end indices that can be used to extract a capture group.
// For example:
//
//	groups := CaptureGroupIndex(pattern)
//	loc := groups[0]
//	cg := pattern[loc[0],loc[1]]
func CapturegroupIndex(pattern string) []CgInfo {
	result := make([]CgInfo, 0)
	// The outer loop finds the beginning of the next named capturegroup
	for i := 0; i < len(pattern); i++ {
		idx := strings.Index(pattern[i:], "(?<")
		if idx == -1 {
			break
		}
		cg := CgInfo{
			Start: idx + i,
		}
		i = cg.Start + 3
		// Find the end of the capturegroup name
	CgName:
		for ; i < len(pattern); i++ {
			switch pattern[i] {
			case '\\':
				// Escape next character
				i++
			case '>':
				cg.Name = pattern[(cg.Start + 3):i]
				i++
				break CgName
			}
		}
		pDepth := 0
		cDepth := 0
		// Find the end of this capturegroup
		for ; i < len(pattern); i++ {
			switch pattern[i] {
			case '\\':
				// Escape next character
				i++
			case '(':
				if cDepth > 0 {
					continue
				}
				pDepth++
			case ')':
				if cDepth > 0 {
					continue
				}
				pDepth--
			case '[':
				cDepth++
			case ']':
				cDepth--
			}
			if pDepth < 0 {
				// Exited this capture group; record it
				cg.End = i + 1
				result = append(result, cg)
				break
			}
		}
	}
	return result
}

// Transforms all non-capturegroup text in the pattern via Regex.QuoteMeta(),
// reusing previously-computed group indices Additionally this will add
// appropriate word or end-of-string anchors to capturegroups and/or the whole
// pattern according to the global 'quoteEscapeFull' option
func CapturegroupQuoteMeta(pattern string, groups []CgInfo) string {
	results := make([]string, 0, len(groups)*2)
	last := 0
	if quoteEscapeFull {
		results = append(results, "^")
	}
	for _, group := range groups {
		if last < group.Start {
			// Escape everything up to the capturegroup
			results = append(results, regexp.QuoteMeta(pattern[last:group.Start]))
		}
		if group.Start == 0 && !quoteEscapeFull {
			// If the capturegroup begins the string, prepend a start-string anchor
			results = append(results, "^")
		}
		if group.Start > 0 && pattern[group.Start-1] == ' ' {
			// If the capturegroup is after a space, prepend a start-word anchor
			results = append(results, "\\b")
		}
		// Append the capturegroup verbatim
		results = append(results, pattern[group.Start:group.End])
		if group.End == len(pattern) && !quoteEscapeFull {
			// If the capturegroup ends the string, append an end-string anchor
			results = append(results, "$")
		}
		if group.End < len(pattern) && pattern[group.End] == ' ' {
			// If the capturegroup is followed by a space, append an end-word anchor
			results = append(results, "\\b")
		}
		last = group.End
	}
	if last < len(pattern) {
		// Escape everything after the last capturegroup
		results = append(results, regexp.QuoteMeta(pattern[last:]))
	}
	if quoteEscapeFull {
		results = append(results, "$")
	}
	return strings.Join(results, "")
}

// Using the 'deletion' side as the pattern, record all matching capturegroups
func (id *diffInfo) captureAllGroups(deletion, insertion diffmatchpatch.Diff) error {
	// Quick sanity check
	if deletion.Type != diffmatchpatch.DiffDelete || insertion.Type != diffmatchpatch.DiffInsert {
		return fmt.Errorf("deletion.Type %s!=DiffDelete or insertion.Type %s!=DiffInsert", deletion.Type.String(), insertion.Type.String())
	}

	// The delete side is always the pattern
	pattern := deletion.Text
	// The insert side is the value we're matching against
	value := insertion.Text

	// Find all capturegroups in the pattern
	groups := CapturegroupIndex(pattern)
	if len(groups) == 0 {
		// No groups to match
		return nil
	}

	// Quote all text that surrounds the capturegroups
	quotedPattern := CapturegroupQuoteMeta(pattern, groups)

	// Attempt a match
	re, err := regexp.Compile(quotedPattern)
	if err != nil {
		// Note: Should not usually be possible, because of the 'validate' function below, but:
		return fmt.Errorf("template %q regex compilation failed: %w", pattern, err)
	}
	if matches := re.FindStringSubmatch(value); matches != nil {
		// Record all named subgroups for substitution later
		for i, cgName := range re.SubexpNames() {
			if i == 0 {
				continue
			}
			if cgName == "" {
				continue
			}
			id.addCapture(cgName, matches[i])
		}
	}
	return nil
}

// Perform the diff, ensuring the diff parts are on word-boundaries, and
// recording the parts in id.diffs
func (id *diffInfo) doWordDiff(pattern, value string) {
	id.dmp = diffmatchpatch.New()
	diffs := id.dmp.DiffMain(pattern, value, false)
	// Note: This DiffCleanupSemantic() helper will ensure we don't split any
	// capture groups into peices provided there is no space in any of them
	// (which is why we enforce this in 'Validate' below)
	// TODO: If we implemented an alternative to this that respected full
	// capture groups and not just word boundaries, that would allow spaces in
	// capture groups.
	id.diffs = id.dmp.DiffCleanupSemantic(diffs)
}

// Perform the diff, ensuring the diff parts are on line-boundaries, and
// recording the parts in id.diffs
func (id *diffInfo) doLineDiff(pattern, value string) {
	id.dmp = diffmatchpatch.New()
	patternLines, valueLines, lineStrings := id.dmp.DiffLinesToChars(pattern, value)
	diffs := id.dmp.DiffMain(patternLines, valueLines, true)
	id.diffs = id.dmp.DiffCharsToLines(diffs, lineStrings)
}

// Return the potentially-comparable diff pair to id.diffs[i] (ie, if
// id.diffs[i+1] represents an insert-then-delete pair or delete-then-insert
// pair), or nil if i+1 is out of bounds or does not constitute a proper
// potentially-comparable pair.
func (id *diffInfo) comparableDiffPair(i int) (*diffmatchpatch.Diff, *diffmatchpatch.Diff) {
	a := id.diffs[i]
	if i+1 < len(id.diffs) {
		b := id.diffs[i+1]
		if a.Type == diffmatchpatch.DiffInsert && b.Type == diffmatchpatch.DiffDelete {
			return &a, &b
		}
		if a.Type == diffmatchpatch.DiffDelete && b.Type == diffmatchpatch.DiffInsert {
			return &b, &a
		}
	}
	return nil, nil
}

// Main entrypoint called by compare.go
func (id CapturegroupsInlineDiff) Diff(pattern, value string) string {
	// General approach:
	//  - Match all relevant capturegroups
	//  - Substitute in the values for all matched capturegroups to the pattern

	cgDiff := diffInfo{}

	// Doing a word-wise diff shrinks the probleset by avoiding any text that
	// is identical or an obvious plain deletion or addition.
	if diffByLines {
		cgDiff.doLineDiff(pattern, value)
	} else {
		// First do a word-wise diff to isolate only those whole words that differ
		cgDiff.doWordDiff(pattern, value)
	}

	// Next, look for any interesting insert-then-delete or delete-then-insert
	// adjacent sections, and try to match any capturegroups we find.
	for i := 0; i < len(cgDiff.diffs); i++ {
		if insertion, deletion := cgDiff.comparableDiffPair(i); insertion != nil && deletion != nil {
			// Records any matching capturegroups in the cgDiff.caps structure
			err := cgDiff.captureAllGroups(*deletion, *insertion)
			if err != nil {
				klog.Warningf("capturegroup error: %s", err)
				// Errors are intentionally nonfatal at this time.
				// Preferably these would be caught in the 'validate'
				// function below.
			}
		}
	}

	// Copy the original pattern string from the template, interpolating in the
	// first matched value from the captures above. This will cause the
	// higher-level diff to show:
	// - missed matches as different
	// - proper matches as identical
	// - any different values matched to the same-named capturegroups as different
	reconciledString := ""
	idx := 0
	for _, group := range CapturegroupIndex(pattern) {
		if idx < group.Start {
			reconciledString += pattern[idx:group.Start]
		}
		if matches, ok := cgDiff.caps[group.Name]; ok {
			if len(matches) == 1 {
				reconciledString += matches[0]
			} else {
				// Multiple matches detected, so call attention to them
				reconciledString += fmt.Sprintf("(?<%s>=%s)", group.Name, matches[0])
			}
		} else {
			reconciledString += pattern[group.Start:group.End]
		}
		idx = group.End
	}
	if idx < len(pattern) {
		reconciledString += pattern[idx:]
	}

	// And for clarity, highlight any capturegroups that had different values
	// matched at different points
	for cgName, cgValues := range cgDiff.caps {
		if len(cgValues) > 1 {
			reconciledString += fmt.Sprintf("\nWARNING: Capturegroup (?<%s>…) matched multiple values: « %s »", cgName, strings.Join(cgValues, " | "))
		}
	}

	return reconciledString
}

// Validation entrypoint called by referenceV2.go
func (id CapturegroupsInlineDiff) Validate(pattern string) error {
	var errs error
	for i, line := range strings.Split(pattern, "\n") {
		// Find all capturegroups in the line
		groups := CapturegroupIndex(line)

		// For each line, ensure our quoted capturegroup result is
		// regex-compliant by compiling it
		_, err := regexp.Compile(CapturegroupQuoteMeta(line, groups))
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("line %d %w", i+1, err))
			continue
		}
		// Furthermore, ensure each capturegroup has no spaces or linebreaks
		// inside (because otherwise the DiffCleanupSemantic() above may split
		// it and render it useless)
		for _, group := range groups {
			if strings.ContainsAny(line[group.Start:group.End], " \n") {
				errs = errors.Join(errs, fmt.Errorf("line %d:%d capturegroup contains spaces or linebreaks", i+1, group.Start))
			}
		}
	}
	return errs
}
