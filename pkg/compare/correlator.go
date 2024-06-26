// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/openshift/kube-compare/pkg/groups"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

var fieldSeparator = "_"

// Correlator provides an abstraction that allow the usage of different Resource correlation logics
// in the kubectl cluster-compare. The correlation process Matches for each Resource a template.
type Correlator interface {
	Match(*unstructured.Unstructured) (*ReferenceTemplate, error)
}

// UnknownMatch an error that can be returned by a Correlator in a case no template was matched for a Resource.
type UnknownMatch struct {
	Resource *unstructured.Unstructured
}

func (e UnknownMatch) Error() string {
	return fmt.Sprintf("Template couldn't be matched for: %s", apiKindNamespaceName(e.Resource))
}

func apiKindNamespaceName(r *unstructured.Unstructured) string {
	if r.GetNamespace() == "" {
		return strings.Join([]string{r.GetAPIVersion(), r.GetKind(), r.GetName()}, fieldSeparator)
	}
	return strings.Join([]string{r.GetAPIVersion(), r.GetKind(), r.GetNamespace(), r.GetName()}, fieldSeparator)
}

// MultipleMatches an error that can be returned by a Correlator in a case multiple template Matches were found for a Resource.
type MultipleMatches struct {
	Resource *unstructured.Unstructured
	Matches  []*ReferenceTemplate
}

func (e MultipleMatches) Error() string {
	return fmt.Sprintf("Multiple matches were found for: %s. The matches found are: %s", apiKindNamespaceName(e.Resource), getTemplatesName(e.Matches))
}

// MultiCorrelator Matches templates by attempting to find a match with one of its predefined Correlators.
type MultiCorrelator struct {
	correlators []Correlator
}

func NewMultiCorrelator(correlators []Correlator) *MultiCorrelator {
	return &MultiCorrelator{correlators: correlators}
}

func (c MultiCorrelator) Match(object *unstructured.Unstructured) (*ReferenceTemplate, error) {
	var errs []error
	for _, core := range c.correlators {
		temp, err := core.Match(object)
		if err == nil || (!errors.As(err, &UnknownMatch{}) && !errors.As(err, &MultipleMatches{})) {
			return temp, err // nolint:wrapcheck
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...) // nolint:wrapcheck
}

// ExactMatchCorrelator Matches templates by exact match between a predefined config including pairs of Resource names and there equivalent template.
// The names of the resources are in the apiVersion-kind-namespace-name format.
// For fields that are not namespaced apiVersion-kind-name format will be used.
type ExactMatchCorrelator struct {
	apiKindNamespaceName map[string]*ReferenceTemplate
}

func NewExactMatchCorrelator(crToTemplate map[string]string, templates []*ReferenceTemplate) (*ExactMatchCorrelator, error) {
	core := ExactMatchCorrelator{}
	core.apiKindNamespaceName = make(map[string]*ReferenceTemplate)
	nameToTemplate := make(map[string]*ReferenceTemplate)
	for _, temp := range templates {
		nameToTemplate[temp.Name()] = temp
	}
	for cr, temp := range crToTemplate {
		templateObj, ok := nameToTemplate[temp]
		if !ok {
			return nil, fmt.Errorf("error in template manual matching for resource: %s no template in the name of %s", cr, temp)
		}
		core.apiKindNamespaceName[cr] = templateObj

	}
	return &core, nil
}

func (c ExactMatchCorrelator) Match(object *unstructured.Unstructured) (*ReferenceTemplate, error) {
	temp, ok := c.apiKindNamespaceName[apiKindNamespaceName(object)]
	if !ok {
		return nil, UnknownMatch{Resource: object}
	}
	return temp, nil
}

// GroupCorrelator Matches templates by hashing predefined fields.
// All The templates are indexed by  hashing groups of `indexed` fields. The `indexed` fields can be nested.
// Resources will be attempted to be matched with hashing by the group with the largest amount of `indexed` fields.
// In case a Resource Matches by a hash a group of templates the group correlator will continue looking for a match
// (with groups with less `indexed fields`) until it finds a distinct match, in case it doesn't, MultipleMatches error
// will be returned.
// Templates will be only indexed by a group of fields only if all fields in group are not templated.
type GroupCorrelator struct {
	// List of groups of nested fields (each field is represented by []string)
	fieldGroups [][][]string
	// List of Hash functions for groups of fields organized in same order of fieldGroups
	GroupFunctions []func(unstructured2 *unstructured.Unstructured) (group string, err error)
	// List of template mappings by different grouping (hashing) options
	templatesByGroups []map[string][]*ReferenceTemplate
}

// NewGroupCorrelator creates a new GroupCorrelator using inputted fieldGroups and generated GroupFunctions and templatesByGroups.
// The templates will be divided into different kinds of groups based on the fields that are templated. Templates will be added
// to the kind of group that contains the biggest amount of fully defined `indexed` fields.
// For fieldsGroups =  {{{"metadata", "namespace"}, {"kind"}}, {{"kind"}}} and the following templates: [fixedKindTemplate, fixedNamespaceKindTemplate]
// the fixedNamespaceKindTemplate will be added to a mapping where the keys are  in the format of `namespace_kind`. The fixedKindTemplate
// will be added to a mapping where the keys are  in the format of `kind`.
func NewGroupCorrelator(fieldGroups [][][]string, templates []*ReferenceTemplate) (*GroupCorrelator, error) {
	var functionGroups []func(*unstructured.Unstructured) (group string, err error)
	sort.Slice(fieldGroups, func(i, j int) bool {
		return len(fieldGroups[i]) >= len(fieldGroups[j])
	})
	for _, group := range fieldGroups {
		functionGroups = append(functionGroups, createGroupHashFunc(group))
	}
	core := GroupCorrelator{fieldGroups: fieldGroups, GroupFunctions: functionGroups}
	mappings, err := groups.Divide(templates, core.getGroupsFunction(), extractMetadata, functionGroups...)
	if err != nil {
		return nil, fmt.Errorf("failed to group templates: %w", err)
	}
	core.templatesByGroups = mappings
	for i, mapping := range mappings {
		res := groups.GetWithMoreThen(mapping, 1)
		if res != nil {
			klog.Warningf("More then one template with same %s. These templates wont be used for"+
				" correlation. To use them use different correlator (manual matching) or remove one of them from the"+
				" reference.  Template names are: %s", getFields(fieldGroups[i]), getTemplatesName(res))

		}
	}
	return &core, nil
}

func getFields(fields [][]string) string {
	var stringifiedFields []string
	for _, field := range fields {
		stringifiedFields = append(stringifiedFields, strings.Join(field, fieldSeparator))
	}
	return strings.Join(stringifiedFields, ", ")
}

// createGroupHashFunc creates a hashing function for a specific field group
func createGroupHashFunc(fieldGroup [][]string) func(*unstructured.Unstructured) (group string, err error) {
	groupHashFunc := func(cr *unstructured.Unstructured) (group string, err error) {
		var values []string
		for _, fields := range fieldGroup {
			value, isFound, NotStringErr := unstructured.NestedString(cr.Object, fields...)
			if !isFound {
				return "", fmt.Errorf("the field %s doesn't exist in resource", strings.Join(fields, fieldSeparator))
			}
			if NotStringErr != nil {
				return "", fmt.Errorf("the field %s isn't string - grouping by non string values isn't supported", strings.Join(fields, fieldSeparator))
			}
			values = append(values, value)
		}
		return strings.Join(values, fieldSeparator), nil
	}
	return groupHashFunc
}

func (c *GroupCorrelator) getGroupsFunction() func(cr *unstructured.Unstructured) ([]int, error) {
	return func(cr *unstructured.Unstructured) ([]int, error) {
		lenGroupMatch := 0
		var groupIndexes []int
		for i, group := range c.fieldGroups {
			if len(group) < lenGroupMatch {
				break
			}
			if areFieldsNotTemplated(cr, group) {
				lenGroupMatch = len(group)
				groupIndexes = append(groupIndexes, i)
			}
		}
		return groupIndexes, nil
	}
}

func areFieldsNotTemplated(cr *unstructured.Unstructured, group [][]string) bool {
	for _, field := range group {
		value, _, _ := unstructured.NestedString(cr.Object, field...)
		if value == "" {
			return false
		}
	}
	return true
}

func getTemplatesName(templates []*ReferenceTemplate) string {
	var names []string
	for _, temp := range templates {
		names = append(names, temp.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (c *GroupCorrelator) Match(object *unstructured.Unstructured) (*ReferenceTemplate, error) {
	var multipleMatchError error
	for i, group := range c.templatesByGroups {
		group_hash, _ := c.GroupFunctions[i](object)
		templates := group[group_hash]
		switch {
		case len(templates) == 1:
			return templates[0], nil
		case len(templates) > 1 && multipleMatchError == nil:
			multipleMatchError = MultipleMatches{Resource: object, Matches: templates}
		}
	}
	if multipleMatchError != nil {
		return nil, multipleMatchError
	}
	return nil, UnknownMatch{Resource: object}
}

// MetricsCorrelatorDecorator Matches templates by using an existing correlator and gathers summary info related the correlation.
type MetricsCorrelatorDecorator struct {
	correlator            *Correlator
	UnMatchedCRs          []*unstructured.Unstructured
	unMatchedLock         sync.Mutex
	MatchedTemplatesNames map[string]bool
	matchedLock           sync.Mutex
	parts                 []Part
	errsToIgnore          []error
}

func NewMetricsCorrelatorDecorator(correlator Correlator, parts []Part, errsToIgnore []error) *MetricsCorrelatorDecorator {
	cr := MetricsCorrelatorDecorator{
		correlator:            &correlator,
		UnMatchedCRs:          []*unstructured.Unstructured{},
		MatchedTemplatesNames: map[string]bool{},
		parts:                 parts,
		errsToIgnore:          errsToIgnore,
	}
	return &cr
}

func (c *MetricsCorrelatorDecorator) Match(object *unstructured.Unstructured) (*ReferenceTemplate, error) {
	temp, err := (*c.correlator).Match(object)
	if err != nil && !containOnly(err, c.errsToIgnore) {
		c.addUNMatch(object)
	}
	if err != nil {
		return temp, err // nolint:wrapcheck
	}
	c.addMatch(temp)
	return temp, nil
}

// containOnly checks if at least one of the joined errors isn't from the err-types passed in errTypes
func containOnly(err error, errTypes []error) bool {
	var errs []error
	joinedErr, isJoined := err.(interface{ Unwrap() []error })
	if isJoined {
		errs = joinedErr.Unwrap()
	} else {
		errs = []error{err}
	}
	for _, errPart := range errs {
		c := false
		for _, errType := range errTypes {
			if reflect.TypeOf(errType).Name() == reflect.TypeOf(errPart).Name() {
				c = true
			}
		}
		if !c {
			return false
		}
	}
	return true
}

func (c *MetricsCorrelatorDecorator) addMatch(temp *ReferenceTemplate) {
	c.matchedLock.Lock()
	c.MatchedTemplatesNames[temp.Name()] = true
	c.matchedLock.Unlock()
}

func (c *MetricsCorrelatorDecorator) addUNMatch(cr *unstructured.Unstructured) {
	c.unMatchedLock.Lock()
	c.UnMatchedCRs = append(c.UnMatchedCRs, cr)
	c.unMatchedLock.Unlock()
}
