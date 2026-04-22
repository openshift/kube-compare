package compare

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"k8s.io/klog/v2"
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
	topLevelCapturedGroups []capturedGroupIndex
}

func (c *capturedValueIndices) addCapture(name, value string, start, end int) {
	c.CapturedValues.addCapture(name, value)
	addNew := true
	replaces := []int{}
	for n, tlg := range c.topLevelCapturedGroups {
		if start >= tlg.start && end <= tlg.end {
			addNew = false
		} else if start <= tlg.start && end >= tlg.end {
			replaces = append(replaces, n)
		}
	}
	if addNew || len(replaces) > 0 {
		c.topLevelCapturedGroups = append(c.topLevelCapturedGroups,
			capturedGroupIndex{name: name, start: start, end: end},
		)
	}

	for _, i := range replaces {
		c.topLevelCapturedGroups = slices.Delete(c.topLevelCapturedGroups, i, i+1)
	}
}

func (c *capturedValueIndices) getTopLevelIndices() []capturedGroupIndex {
	sort.Slice(c.topLevelCapturedGroups, func(i, j int) bool {
		return cmp.Less(c.topLevelCapturedGroups[j].start, c.topLevelCapturedGroups[i].start)
	})
	return c.topLevelCapturedGroups
}

func (id RegexInlineDiff) Diff(regex, crValue string, sharedCapturedValues CapturedValues) (string, CapturedValues) {
	re, err := regexp.Compile(regex)
	if err != nil {
		klog.V(1).Infof("Failed to compile regex %q in inline diff: %v", regex, err)
		return regex, sharedCapturedValues
	}
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
		return fmt.Errorf("invalid regex passed to inline regex diff function: %w", err)
	}
	return nil
}
