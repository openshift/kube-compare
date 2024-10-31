package compare

import (
	"fmt"
	"regexp"
)

const (
	regex inlineDiffType = "regex"
)

type RegexInlineDiff struct{}

func (id RegexInlineDiff) Diff(regex, crValue string) string {
	re, err := regexp.Compile(regex)
	if err != nil {
		return regex
	}
	if re.MatchString(crValue) {
		return crValue
	}
	return regex
}

func (id RegexInlineDiff) Validate(regex string) error {
	_, err := regexp.Compile(regex)
	if err != nil {
		return fmt.Errorf("invalid regex passed to inline rgegex diff function: %w", err)
	}
	return nil
}
