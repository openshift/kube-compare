// Package groups provides a set of utilities for dividing and dealing with groups of elements.
package groups

import (
	"errors"
	"fmt"
)

// Divide divides elements into groups based on specified grouping functions.
// The grouping functions are applied to the parsed elements, and the elements
// are grouped into maps based on the results of these functions.
// Before applying the grouping functions, each element is parsed using the
// provided parsing function. Additionally, the filterGroupsFor function is
// called for each parsed element to determine the groups to which the element
// should be added.
// The returned slice contains mappings of the elements grouped by different
// grouping functions. Each key in the maps within the slice represents a group,
// and the corresponding value is a slice of elements belonging to that group.
func Divide[E any, V any, G comparable](elements []E, filterGroupsFor func(element V) ([]int, error), parse func(E) (V, error), groupingFunctions ...func(V) (group G, err error)) ([]map[G][]E, error) {
	var groups []map[G][]E
	var errs []error
	for range groupingFunctions {
		groups = append(groups, make(map[G][]E))
	}
	for _, element := range elements {
		parsedElement, err := parse(element)
		if err != nil {
			errs = append(errs, err)
			break
		}
		indexes, err := filterGroupsFor(parsedElement)
		if err != nil {
			errs = append(errs, err)
			break
		}
		for _, i := range indexes {
			groupName, groupErr := groupingFunctions[i](parsedElement)
			if groupErr != nil {
				errs = append(errs, fmt.Errorf("error while finding group hash for %v. Error: %v", parsedElement, groupErr))
				break
			}
			if _, ok := groups[i][groupName]; !ok {
				groups[i][groupName] = []E{}
			}
			groups[i][groupName] = append(groups[i][groupName], element)
		}
	}
	return groups, errors.Join(errs...)
}

// GetWithMoreThen retrieves elements from groups where the group size exceeds a
// specified threshold.
func GetWithMoreThen[K comparable, V any](groups map[K][]V, threshold int) []V {
	for _, elements := range groups {
		if len(elements) > threshold {
			return elements
		}
	}
	return nil
}
