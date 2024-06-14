// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/sprig/v3"
	jsonpatch "github.com/evanphx/json-patch"
	"sigs.k8s.io/yaml"
)

// This File is almost identical to the FuncMap used in Helm to match Helm templating behaviour.

// FuncMap returns a mapping of all of the functions that Engine has.
//
// Because some functions are late-bound (e.g. contain context-sensitive
// data), the functions may not all perform identically outside of an Engine
// as they will inside of an Engine.
//
// Known late-bound functions:
//
//   - "include"
//   - "tpl"
//
// These are late-bound in Engine.Render().  The
// version included in the FuncMap is a placeholder.
func FuncMap(o *Options) template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")

	// Add some extra functionality
	extra := template.FuncMap{
		"toToml":        toTOML,
		"toYaml":        toYAML,
		"fromYaml":      fromYAML,
		"fromYamlArray": fromYAMLArray,
		"toJson":        toJSON,
		"fromJson":      fromJSON,
		"fromJsonArray": fromJSONArray,
		"onlyMatch":     onlyMatch,
		"merge":         mergeWrapper(o),
	}

	for k, v := range extra {
		f[k] = v
	}

	return f
}

// toYAML takes an interface, marshals it to yaml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toYAML(v any) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		// Swallow errors inside of a template.
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}

// fromYAML converts a YAML document into a map[string]any.
//
// This is not a general-purpose YAML parser, and will not parse all valid
// YAML documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string into
// m["Error"] in the returned map.
func fromYAML(str string) map[string]any {
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
		// Swallow errors inside of a template.
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

// onlyMatch does the inverse of the merge function
//
// Can be useful when using allow-undefined-extras=true
// as the merges will cancel out allowing you to be more
// specific for a block.
// If an error occurs it will return error message string
func onlyMatch(data any, matchObj string) string {
	if data == nil {
		return matchObj
	}
	patch, err := jsonpatch.CreateMergePatch([]byte(toJSON(data)), []byte(toJSON(fromYAML(matchObj))))
	if err != nil {
		return err.Error()
	}
	return string(patch)
}

// merge does a merges mergeObj into the data structure provided
//
// Can be useful when you want allow undefined extras
// If an error occurs it will return error message string
func merge(data any, mergeObj string) string {
	if data == nil {
		return mergeObj
	}
	merged, err := jsonpatch.MergePatch([]byte(toJSON(fromYAML(mergeObj))), []byte(toJSON(data)))
	if err != nil {
		return err.Error()
	}
	return string(merged)
}

// mergeWrapper extends the effect of the show-more flag to the merge helper function
func mergeWrapper(o *Options) func(data any, mergeObj string) string {
	return func(data any, mergeObj string) string {
		if o.showMore {
			return mergeObj
		}
		return merge(data, mergeObj)
	}
}
