// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/sprig/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// This File is almost identical to the FuncMap used in Helm to match Helm templating behaviour.

var FuncHelp = make(map[string]string)

const SprigImportFlag = `<<sprig>>`

// recursionMaxNums is the maximum number of times a template may be
// recursively included before we assume an infinite loop and bail out.
// This matches Helm's recursion limit.
const recursionMaxNums = 1000

// FuncMap returns a mapping of all of the functions that Engine has.
//
// Because some functions are late-bound (e.g. contain context-sensitive
// data), the functions may not all perform identically outside of an Engine
// as they will inside of an Engine.
//
// Known late-bound functions:
//
//   - "include": executes a named template and returns its output as a string.
//
// These are late-bound after template parsing via InitInclude.
// The version included in the FuncMap is a placeholder that will
// return an error if called before binding.
func FuncMap() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")
	delete(f, "getHostByName")

	for key := range f {
		FuncHelp[key] = SprigImportFlag
	}

	// Add a placeholder for the late-bound "include" function.
	// The real implementation is injected after template parsing via InitInclude.
	f["include"] = func(name string, data any) (string, error) {
		return "", fmt.Errorf("include is not yet bound to a template; this is a placeholder")
	}
	FuncHelp["include"] = "Execute a named template and return its output as a string (like Helm's include)"

	// Add some extra functionality
	extra := map[string]struct {
		fn   any
		help string
	}{
		"toToml": {
			fn:   toTOML,
			help: "Render incoming data of any type as a TOML document string",
		},
		"toYaml": {
			fn:   toYAML,
			help: "Render incoming data of any type as a YAML document string",
		},
		"toJson": {
			fn:   toJSON,
			help: "Render incoming data of any type as a JSON string",
		},
		"fromYaml": {
			fn:   FromYAML,
			help: "Parse the incoming string as a structured YAML object",
		},
		"fromYamlArray": {
			fn:   fromYAMLArray,
			help: "Parse the incoming string as a structured YAML array",
		},
		"fromJson": {
			fn:   fromJSON,
			help: "Parse the incoming string as a structured JSON object",
		},
		"fromJsonArray": {
			fn:   fromJSONArray,
			help: "Parse the incoming string as a structured JSON array",
		},
		"lookupCRs": {
			fn:   lookupCRs,
			help: "Lookup an external CR and return an array of matching objects",
		},
		"lookupCR": {
			fn:   lookupCR,
			help: "Lookup an external CR and return exactly one matching object",
		},
		"doNotMatch": {
			fn:   doNotMatch,
			help: "Skip matching this target against the current CR",
		},
	}

	for k, v := range extra {
		f[k] = v.fn
		FuncHelp[k] = v.help
	}

	return f
}

func DisplayFuncmap(w io.Writer) error {
	if len(FuncHelp) == 0 {
		// Populate the help text
		FuncMap()
	}
	customNames := make([]string, 0, len(FuncHelp))
	sprigNames := make([]string, 0, len(FuncHelp))
	for k, v := range FuncHelp {
		if v == SprigImportFlag {
			sprigNames = append(sprigNames, k)
		} else {
			customNames = append(customNames, k)
		}
	}
	slices.Sort(customNames)
	slices.Sort(sprigNames)

	if _, err := fmt.Fprintln(w, "Available Template Functions"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "============================"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return err
	}
	for _, name := range customNames {
		if _, err := fmt.Fprintf(w, "%s:\n  %s\n", name, strings.Join(strings.Split(FuncHelp[name], "\n"), "\n  ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Imported from https://masterminds.github.io/sprig/"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "--------------------------------------------------"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, strings.Join(sprigNames, ", "))
	return err
}

// toYAML takes an interface, marshals it to yaml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toYAML(v any) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		klog.Warningf("Failed to marshal value to YAML in template: %v", err)
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}

// FromYAML converts a YAML document into a map[string]any.
//
// This is not a general-purpose YAML parser, and will not parse all valid
// YAML documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string into
// m["Error"] in the returned map.
func FromYAML(str string) map[string]any {
	m := map[string]any{}

	if err := yaml.Unmarshal([]byte(str), &m); err != nil {
		m["Error"] = err.Error()
	}
	return m
}

// fromYAMLArray converts a YAML array into a []any.
//
// This is not a general-purpose YAML parser, and will not parse all valid
// YAML documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string as
// the first and only item in the returned array.
func fromYAMLArray(str string) []any {
	a := []any{}

	if err := yaml.Unmarshal([]byte(str), &a); err != nil {
		a = []any{err.Error()}
	}
	return a
}

// toTOML takes an interface, marshals it to toml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toTOML(v any) string {
	b := bytes.NewBuffer(nil)
	e := toml.NewEncoder(b)
	err := e.Encode(v)
	if err != nil {
		return err.Error()
	}
	return b.String()
}

// toJSON takes an interface, marshals it to json, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		klog.Warningf("Failed to marshal value to JSON in template: %v", err)
		return ""
	}
	return string(data)
}

// fromJSON converts a JSON document into a map[string]any.
//
// This is not a general-purpose JSON parser, and will not parse all valid
// JSON documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string into
// m["Error"] in the returned map.
func fromJSON(str string) map[string]any {
	m := make(map[string]any)

	if err := json.Unmarshal([]byte(str), &m); err != nil {
		m["Error"] = err.Error()
	}
	return m
}

// fromJSONArray converts a JSON array into a []any.
//
// This is not a general-purpose JSON parser, and will not parse all valid
// JSON documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string as
// the first and only item in the returned array.
func fromJSONArray(str string) []any {
	a := []any{}

	if err := json.Unmarshal([]byte(str), &a); err != nil {
		a = []any{err.Error()}
	}
	return a
}

// In order to use `lookupCRs` and `lookupCR`, AllCRs must be populated
var AllCRs []*unstructured.Unstructured

// lookupCRs looks for all known CRS that match the given fields.
// apiVersion and kind must be supplied.
// namespace is optional (may be "" or "*")
// name is optional (may be "" or "*")
func lookupCRs(apiVersion, kind, namespace, name string) []map[string]any {
	var matched []map[string]any
	for _, obj := range AllCRs {
		if apiVersion != obj.GetAPIVersion() {
			continue
		}
		if kind != obj.GetKind() {
			continue
		}
		if namespace != "" && namespace != "*" && namespace != obj.GetNamespace() {
			continue
		}
		if name != "" && name != "*" && name != obj.GetName() {
			continue
		}
		matched = append(matched, obj.Object)
	}
	if klog.V(1).Enabled() {
		stage := ""
		if len(AllCRs) == 0 {
			stage = "pre-init "
		}
		klog.Infof("%slookupCRs %q %q %q %q located %d objects", stage, apiVersion, kind, namespace, name, len(matched))
	}
	return matched
}

// lookupCR returns a single object if exactly one object matched the search criteria
// If 0 objects or >1 objects matched, returns nil
// apiVersion and kind must be supplied.
// namespace is optional (may be "" or "*")
// name is optional (may be "" or "*")
func lookupCR(apiVersion, kind, namespace, name string) map[string]any {
	all := lookupCRs(apiVersion, kind, namespace, name)
	if len(all) != 1 {
		return nil
	}
	return all[0]
}

type DoNotMatch struct {
	Reason string
}

func (e DoNotMatch) Error() string {
	return fmt.Sprintf("Do not match: %s", e.Reason)
}

func doNotMatch(reason string) (string, error) {
	return "", &DoNotMatch{Reason: reason}
}

// includeFun returns a function that executes a named template and returns
// its output as a string. This matches Helm's include semantics.
// It includes a recursion guard to prevent infinite loops.
func includeFun(t *template.Template, includedNames map[string]int) func(string, any) (string, error) {
	return func(name string, data any) (string, error) {
		var buf strings.Builder
		if v, ok := includedNames[name]; ok {
			if v > recursionMaxNums {
				return "", fmt.Errorf(
					"rendering template has a nested reference name: %s: unable to execute template",
					name)
			}
			includedNames[name]++
		} else {
			includedNames[name] = 1
		}
		err := t.ExecuteTemplate(&buf, name, data)
		includedNames[name]--
		return buf.String(), err
	}
}

// InitInclude binds the late-bound "include" template function to a parsed
// template. This must be called after template parsing but before execution.
// It replaces the placeholder include function with a real implementation
// that can execute named sub-templates.
func InitInclude(t *template.Template) {
	includedNames := make(map[string]int)
	t.Funcs(template.FuncMap{
		"include": includeFun(t, includedNames),
	})
}
