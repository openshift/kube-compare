// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

var FieldSeparator = "_"

// Correlator provides an abstraction that allow the usage of different Resource correlation logics
// in the kubectl cluster-compare. The correlation process Matches for each Resource a template.
type Correlator[T CorrelationEntry] interface {
	Match(*unstructured.Unstructured) ([]T, error)
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
		return strings.Join([]string{r.GetAPIVersion(), r.GetKind(), r.GetName()}, FieldSeparator)
	}
	return strings.Join([]string{r.GetAPIVersion(), r.GetKind(), r.GetNamespace(), r.GetName()}, FieldSeparator)
}

// MultiCorrelator Matches templates by attempting to find a match with one of its predefined Correlators.
type MultiCorrelator[T CorrelationEntry] struct {
	correlators []Correlator[T]
}

func NewMultiCorrelator[T CorrelationEntry](correlators []Correlator[T]) *MultiCorrelator[T] {
	return &MultiCorrelator[T]{correlators: correlators}
}

func (c *MultiCorrelator[T]) AddCorrelator(correlator Correlator[T]) {
	c.correlators = append(c.correlators, correlator)
}

func (c MultiCorrelator[T]) Match(object *unstructured.Unstructured) ([]T, error) {
	var errs []error
	for _, core := range c.correlators {
		temp, err := core.Match(object)
		if err == nil || !errors.As(err, &UnknownMatch{}) {
			return temp, err // nolint:wrapcheck
		}
		errs = append(errs, err)
	}
	var res []T
	return res, errors.Join(errs...) // nolint:wrapcheck
}

type CorrelationEntry interface {
	GetIdentifier() string
	GetMetadata() *unstructured.Unstructured
}

// ExactMatchCorrelator Matches templates by exact match between a predefined config including pairs of Resource names and there equivalent template.
// The names of the resources are in the apiVersion-kind-namespace-name format.
// For fields that are not namespaced apiVersion-kind-name format will be used.
type ExactMatchCorrelator[T CorrelationEntry] struct {
	apiKindNamespaceName map[string]T
}

func NewExactMatchCorrelator[T CorrelationEntry](matchPairs map[string]string, templates []T) (*ExactMatchCorrelator[T], error) {
	core := ExactMatchCorrelator[T]{}
	core.apiKindNamespaceName = make(map[string]T)
	nameToObject := make(map[string]T)
	for _, temp := range templates {
		nameToObject[temp.GetIdentifier()] = temp
	}
	for cr, temp := range matchPairs {
		obj, ok := nameToObject[temp]
		if !ok {
			return nil, fmt.Errorf("error in template manual matching for resource: %s no template in the name of %s", cr, temp)
		}
		core.apiKindNamespaceName[cr] = obj

	}
	return &core, nil
}

func (c ExactMatchCorrelator[T]) Match(object *unstructured.Unstructured) ([]T, error) {
	temp, ok := c.apiKindNamespaceName[apiKindNamespaceName(object)]
	if !ok {
		return []T{}, UnknownMatch{Resource: object}
	}
	return []T{temp}, nil
}

// GroupCorrelator Matches templates by hashing predefined fields.
// All The templates are indexed by  hashing groups of `indexed` fields. The `indexed` fields can be nested.
// Resources will be attempted to be matched with hashing by the group with the largest amount of `indexed` fields.
// In case a Resource Matches by a hash a group of templates the group correlator will continue looking for a match
// (with groups with less `indexed fields`) until it finds a distinct match, in case it doesn't, MultipleMatches error
// will be returned.
// Templates will be only indexed by a group of fields only if all fields in group are not templated.
type GroupCorrelator[T CorrelationEntry] struct {
	fieldCorrelators []*FieldCorrelator[T]
}

// NewGroupCorrelator creates a new GroupCorrelator using inputted fieldGroups and generated GroupFunctions and templatesByGroups.
// The templates will be divided into different kinds of groups based on the fields that are templated. Templates will be added
// to the kind of group that contains the biggest amount of fully defined `indexed` fields.
// For fieldsGroups =  {{{"metadata", "namespace"}, {"kind"}}, {{"kind"}}} and the following templates: [fixedKindTemplate, fixedNamespaceKindTemplate]
// the fixedNamespaceKindTemplate will be added to a mapping where the keys are  in the format of `namespace_kind`. The fixedKindTemplate
// will be added to a mapping where the keys are  in the format of `kind`.
func NewGroupCorrelator[T CorrelationEntry](fieldGroups [][][]string, objects []T) (*GroupCorrelator[T], error) {
	sort.Slice(fieldGroups, func(i, j int) bool {
		return len(fieldGroups[i]) >= len(fieldGroups[j])
	})
	core := GroupCorrelator[T]{}
	for _, group := range fieldGroups {
		fc := FieldCorrelator[T]{Fields: group, hashFunc: createGroupHashFunc(group)}
		newObjects := fc.ClaimTemplates(objects)

		// Ignore if the fc didn't take any objects
		if len(newObjects) == len(objects) {
			continue
		}

		objects = newObjects
		core.fieldCorrelators = append(core.fieldCorrelators, &fc)

		err := fc.ValidateTemplates()
		if err != nil {
			if klog.V(1).Enabled() {
				klog.Warning(err)
			} else {
				klog.Warning("The reference contains overlapping object definitions which may result in unexpected correlation results. Re-run with '--verbose' output enabled to view a detailed description of the issues")
			}
		}

		if len(objects) == 0 {
			break
		}
	}

	return &core, nil
}

func getFields(fields [][]string) string {
	var stringifiedFields []string
	for _, field := range fields {
		stringifiedFields = append(stringifiedFields, strings.Join(field, FieldSeparator))
	}
	return strings.Join(stringifiedFields, ", ")
}

type templateHashFunc func(*unstructured.Unstructured, string) (group string, err error)

// createGroupHashFunc creates a hashing function for a specific field group
func createGroupHashFunc(fieldGroup [][]string) templateHashFunc {
	groupHashFunc := func(cr *unstructured.Unstructured, replaceEmptyWith string) (group string, err error) {
		var values []string
		for _, fields := range fieldGroup {
			value, isFound, NotStringErr := NestedString(cr.Object, fields...)
			if !isFound || value == "" {
				return "", fmt.Errorf("the field %s doesn't exist in resource", strings.Join(fields, FieldSeparator))
			}
			if NotStringErr != nil {
				return "", fmt.Errorf("the field %s isn't string - grouping by non string values isn't supported", strings.Join(fields, FieldSeparator))
			}
			values = append(values, value)
		}
		return strings.Join(values, FieldSeparator), nil
	}
	return groupHashFunc
}

func getTemplatesNames[T CorrelationEntry](templates []T) string {
	var names []string
	for _, temp := range templates {
		names = append(names, temp.GetIdentifier())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (c *GroupCorrelator[T]) Match(object *unstructured.Unstructured) ([]T, error) {
	for _, fc := range c.fieldCorrelators {
		temp, err := fc.Match(object)
		if err != nil {
			continue
		}
		if len(temp) > 0 {
			return temp, nil
		}
	}
	return []T{}, UnknownMatch{Resource: object}
}

// MetricsTracker Matches templates by using an existing correlator and gathers summary info related the correlation.
type MetricsTracker struct {
	UnMatchedCRs          []*unstructured.Unstructured
	unMatchedLock         sync.Mutex
	MatchedTemplatesNames map[string]int
	matchedLock           sync.Mutex
}

func NewMetricsTracker() *MetricsTracker {
	cr := MetricsTracker{
		UnMatchedCRs:          []*unstructured.Unstructured{},
		MatchedTemplatesNames: map[string]int{},
	}
	return &cr
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

func (c *MetricsTracker) addMatch(temp ReferenceTemplate) {
	c.matchedLock.Lock()
	c.MatchedTemplatesNames[temp.GetIdentifier()] += 1
	c.matchedLock.Unlock()
}

func (c *MetricsTracker) addUNMatch(cr *unstructured.Unstructured) {
	c.unMatchedLock.Lock()
	c.UnMatchedCRs = append(c.UnMatchedCRs, cr)
	c.unMatchedLock.Unlock()
}

func (c *MetricsTracker) getTotalCRs() int {
	count := 0
	for _, v := range c.MatchedTemplatesNames {
		count += v
	}
	return count
}

type FieldCorrelator[T CorrelationEntry] struct {
	Fields   [][]string
	hashFunc templateHashFunc
	objects  map[string][]T
}

func (f *FieldCorrelator[T]) ClaimTemplates(templates []T) []T {
	if f.objects == nil {
		f.objects = make(map[string][]T)
	}

	discarded := make([]T, 0)
	for _, temp := range templates {
		md := temp.GetMetadata()
		hash, err := f.hashFunc(md, noValue)
		if err != nil || strings.Contains(hash, noValue) {
			discarded = append(discarded, temp)
		} else {
			f.objects[hash] = append(f.objects[hash], temp)
		}
	}

	return discarded
}

func (f *FieldCorrelator[T]) ValidateTemplates() error {
	errs := make([]error, 0)
	for _, values := range f.objects {
		if len(values) > 1 {
			errs = append(errs, fmt.Errorf(
				"More than one template with same %s. By Default for each Cluster CR that is correlated "+
					"to one of these templates the template with the least number of diffs will be used. "+
					"To use a different template for a specific CR specify it in the diff-config (-c flag) "+
					"Template names are: %s",
				getFields(f.Fields), getTemplatesNames(values)),
			)
		}
	}

	return errors.Join(errs...)
}

func (f FieldCorrelator[T]) Match(object *unstructured.Unstructured) ([]T, error) {
	group_hash, err := f.hashFunc(object, "")
	if err != nil {
		return nil, err
	}
	objs, ok := f.objects[group_hash]
	if !ok {
		return nil, UnknownMatch{Resource: object}
	}
	return objs, nil
}

// OwnerReferenceCorrelator searches for resources by checking if the resource appears in the ownerReferences
// field of any cluster resource. This handles cases where resources are managed by operators and don't exist
// as standalone objects but are referenced as owners.
type OwnerReferenceCorrelator[T CorrelationEntry] struct {
	templates     []T
	ownerRefIndex map[string]bool // Index of all ownerReferences found (key = apiVersion_kind_namespace_name)
	templatesMap  map[string][]T  // Map from apiVersion_kind_namespace_name to templates
}

// NewOwnerReferenceCorrelator creates a new OwnerReferenceCorrelator
// It builds an index of all ownerReferences found in cluster resources
func NewOwnerReferenceCorrelator[T CorrelationEntry](templates []T, clusterCRs []*unstructured.Unstructured) *OwnerReferenceCorrelator[T] {
	templatesMap := make(map[string][]T)
	for _, temp := range templates {
		md := temp.GetMetadata()
		key := apiKindNamespaceName(md)
		templatesMap[key] = append(templatesMap[key], temp)
	}

	// Build index of all ownerReferences found in cluster resources
	ownerRefIndex := make(map[string]bool)
	for _, clusterCR := range clusterCRs {
		ownerRefs, found, err := unstructured.NestedSlice(clusterCR.Object, "metadata", "ownerReferences")
		if err != nil || !found {
			continue
		}

		for _, ownerRefInterface := range ownerRefs {
			ownerRef, ok := ownerRefInterface.(map[string]interface{})
			if !ok {
				continue
			}

			apiVersion, _, err1 := unstructured.NestedString(ownerRef, "apiVersion")
			kind, _, err2 := unstructured.NestedString(ownerRef, "kind")
			name, _, err3 := unstructured.NestedString(ownerRef, "name")
			if err1 != nil || err2 != nil || err3 != nil {
				klog.V(1).Infof("Skipping ownerReference with non-string fields: %v %v %v", err1, err2, err3)
				continue
			}

			if apiVersion == "" || kind == "" || name == "" {
				continue
			}

			// Build the owner key
			// For cluster-scoped owners referenced from namespaced resources, we need to index both:
			// 1. Without namespace (for cluster-scoped owners)
			// 2. With namespace (for namespaced owners)
			ownerKeyWithoutNS := strings.Join([]string{apiVersion, kind, name}, FieldSeparator)
			ownerRefIndex[ownerKeyWithoutNS] = true

			if clusterCR.GetNamespace() != "" {
				ownerKeyWithNS := strings.Join([]string{apiVersion, kind, clusterCR.GetNamespace(), name}, FieldSeparator)
				ownerRefIndex[ownerKeyWithNS] = true
			}
		}
	}

	return &OwnerReferenceCorrelator[T]{
		templates:     templates,
		ownerRefIndex: ownerRefIndex,
		templatesMap:  templatesMap,
	}
}

// Match searches for the resource in the pre-built ownerReferences index
func (c OwnerReferenceCorrelator[T]) Match(object *unstructured.Unstructured) ([]T, error) {
	searchKey := apiKindNamespaceName(object)

	// Check if this resource exists as an ownerReference
	if c.ownerRefIndex[searchKey] {
		// Found the resource in ownerReferences index
		if temps, ok := c.templatesMap[searchKey]; ok {
			klog.V(1).Infof("Found resource %s via ownerReferences", searchKey)
			return temps, nil
		}
	}

	return []T{}, UnknownMatch{Resource: object}
}

// SubjectsCorrelator searches for resources by checking if the resource appears in the subjects
// field of RBAC resources (ClusterRoleBindings, RoleBindings). This handles cases where ServiceAccounts
// and other subjects are referenced but don't exist as standalone objects in the collection.
type SubjectsCorrelator[T CorrelationEntry] struct {
	templates     []T
	subjectsIndex map[string]bool // Index of all subjects found (key = apiVersion_kind_namespace_name)
	templatesMap  map[string][]T  // Map from apiVersion_kind_namespace_name to templates
}

// NewSubjectsCorrelator creates a new SubjectsCorrelator
// It builds an index of all subjects found in RBAC resources
func NewSubjectsCorrelator[T CorrelationEntry](templates []T, clusterCRs []*unstructured.Unstructured) *SubjectsCorrelator[T] {
	templatesMap := make(map[string][]T)
	for _, temp := range templates {
		md := temp.GetMetadata()
		key := apiKindNamespaceName(md)
		templatesMap[key] = append(templatesMap[key], temp)
	}

	// Build index of all subjects found in cluster resources
	subjectsIndex := make(map[string]bool)
	for _, clusterCR := range clusterCRs {
		// Only check RBAC resources (ClusterRoleBinding, RoleBinding)
		kind := clusterCR.GetKind()
		if kind != "ClusterRoleBinding" && kind != "RoleBinding" {
			continue
		}

		subjects, found, err := unstructured.NestedSlice(clusterCR.Object, "subjects")
		if err != nil || !found {
			continue
		}

		for _, subjectInterface := range subjects {
			subject, ok := subjectInterface.(map[string]interface{})
			if !ok {
				continue
			}

			// Extract subject information
			// Note: subjects use "kind" not "apiVersion"
			subjKind, _, err1 := unstructured.NestedString(subject, "kind")
			subjName, _, err2 := unstructured.NestedString(subject, "name")
			subjNamespace, _, err3 := unstructured.NestedString(subject, "namespace")
			if err1 != nil || err2 != nil || err3 != nil {
				klog.V(1).Infof("Skipping subject with non-string fields: %v %v %v", err1, err2, err3)
				continue
			}

			if subjKind == "" || subjName == "" {
				continue
			}

			// Build the subject key
			// ServiceAccount subjects use "v1" as apiVersion
			var apiVersion string
			switch subjKind {
			case "ServiceAccount":
				apiVersion = "v1"
			case "User", "Group":
				// Users and Groups don't have apiVersions in the same way
				continue
			default:
				continue
			}

			// Index both with and without namespace
			subjKeyWithoutNS := strings.Join([]string{apiVersion, subjKind, subjName}, FieldSeparator)
			subjectsIndex[subjKeyWithoutNS] = true

			if subjNamespace != "" {
				subjKeyWithNS := strings.Join([]string{apiVersion, subjKind, subjNamespace, subjName}, FieldSeparator)
				subjectsIndex[subjKeyWithNS] = true
			}
		}
	}

	return &SubjectsCorrelator[T]{
		templates:     templates,
		subjectsIndex: subjectsIndex,
		templatesMap:  templatesMap,
	}
}

// Match searches for the resource in the pre-built subjects index
func (c SubjectsCorrelator[T]) Match(object *unstructured.Unstructured) ([]T, error) {
	searchKey := apiKindNamespaceName(object)

	// Check if this resource exists as a subject
	if c.subjectsIndex[searchKey] {
		// Found the resource in subjects index
		if temps, ok := c.templatesMap[searchKey]; ok {
			klog.V(1).Infof("Found resource %s via RBAC subjects", searchKey)
			return temps, nil
		}
	}

	return []T{}, UnknownMatch{Resource: object}
}
