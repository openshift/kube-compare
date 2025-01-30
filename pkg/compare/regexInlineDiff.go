package compare

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sort"
)

const (
	regex inlineDiffType = "regex"
)

type RegexInlineDiff struct{}

type capturedGroupIndex struct {
	name  string
	start int
	end   int
}

type capturedValueIndices struct {
	CapturedValues
	topLevelCaputuredGroups []capturedGroupIndex
}

func (c *capturedValueIndices) addCapture(name, value string, start, end int) {
	c.CapturedValues.addCapture(name, value)
	addNew := true
	replaces := []int{}
	for n, tlg := range c.topLevelCaputuredGroups {
		if start >= tlg.start && end <= tlg.end {
			addNew = false
		} else if start <= tlg.start && end >= tlg.end {
			replaces = append(replaces, n)
		}
	}
	if addNew || len(replaces) > 0 {
		c.topLevelCaputuredGroups = append(c.topLevelCaputuredGroups,
			capturedGroupIndex{name: name, start: start, end: end},
		)
	}

	for _, i := range replaces {
		c.topLevelCaputuredGroups = slices.Delete(c.topLevelCaputuredGroups, i, i+1)
	}
}

func (c *capturedValueIndices) getTopLevelIndices() []capturedGroupIndex {
	sort.Slice(c.topLevelCaputuredGroups, func(i, j int) bool {
		return cmp.Less(c.topLevelCaputuredGroups[j].start, c.topLevelCaputuredGroups[i].start)
	})
	return c.topLevelCaputuredGroups
}

func (id RegexInlineDiff) Diff(regex, crValue string, sharedCapturedValues CapturedValues) (string, CapturedValues) {
	re, _ := regexp.Compile(regex)
	matchedIndices := re.FindStringSubmatchIndex(crValue)
	if matchedIndices == nil {
		return regex, sharedCapturedValues
	}

	capturedValues := capturedValueIndices{
		CapturedValues: sharedCapturedValues,
	}
	for n, name := range re.SubexpNames() {
		if name == "" {
			continue
		}
		start := matchedIndices[(n+1)*2-2]
		end := matchedIndices[(n+1)*2-1]
		value := crValue[start:end]
		capturedValues.addCapture(name, value, start, end)
	}

	result := crValue[matchedIndices[0]:matchedIndices[1]]
	for _, tl := range capturedValues.getTopLevelIndices() {
		result = result[:tl.start] + capturedValues.groupValues(tl.name) + result[tl.end:]
	}
	result += capturedValues.getWarnings()
	return result, capturedValues.CapturedValues
}

func (id RegexInlineDiff) Validate(regex string) error {
	if _, err := regexp.Compile(regex); err != nil {
		return fmt.Errorf("invalid regex passed to inline rgegex diff function: %w", err)
	}
	return nil
}
