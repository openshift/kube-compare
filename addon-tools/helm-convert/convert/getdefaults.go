package convert

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/openshift/kube-compare/pkg/compare"
	"k8s.io/client-go/util/jsonpath"
	"k8s.io/kubectl/pkg/cmd/get"
)

func buildJsonPath(pathParts []string) (string, error) {
	var parts []string
	for i, part := range pathParts {
		escapedPart := strings.ReplaceAll(part, ".", "\\.")
		if i < len(pathParts)-1 && isSliceIndex(pathParts[i+1]) {
			escapedPart += pathParts[i+1]
		}
		parts = append(parts, escapedPart)
	}
	path := strings.Join(parts, ".")
	path, err := get.RelaxedJSONPathExpression(path)
	if err != nil {
		return path, fmt.Errorf("error building jsonpath for %q: %w", path, err)
	}
	return path, nil
}

// Function to get values from JSON using jsonpath expressions
func getValuesFromJson(jsonData map[string]interface{}, paths [][]string) (map[*[]string]interface{}, error) {
	results := make(map[*[]string]interface{})

	for _, pathParts := range paths {
		jsonPathExpr, err := buildJsonPath(pathParts)
		if err != nil {
			return nil, err
		}
		jp := jsonpath.New("jsonpath")
		if err := jp.Parse(jsonPathExpr); err != nil {
			return nil, fmt.Errorf("error parsing jsonpath expression %q: %w", jsonPathExpr, err)
		}

		result, err := jp.FindResults(jsonData)
		if err != nil {
			results[&pathParts] = make(map[string]interface{})
		}

		if len(result) > 0 && len(result[0]) > 0 {
			results[&pathParts] = result[0][0].Interface()
		}
	}

	return results, nil
}

func isSliceIndex(part string) bool {
	return len(part) > 2 && part[0] == '[' && part[len(part)-1] == ']'
}

func ExtractIntFromBrackets(s string) (int, error) {
	if !isSliceIndex(s) {
		return 0, fmt.Errorf("input string is not properly formatted")
	}
	inner := s[1 : len(s)-1]
	value, err := strconv.Atoi(inner)
	if err != nil {
		return 0, fmt.Errorf("error converting string to integer: %w", err)
	}

	return value, nil
}

func getNextLevel(parts []string, index int, tempNextLevel interface{}) interface{} {
	part := parts[index]
	if isSliceIndex(part) {
		num, _ := ExtractIntFromBrackets(part)
		newSlice := make([]interface{}, num+1)
		if tempNextLevelSlice, ok := tempNextLevel.([]interface{}); ok && tempNextLevelSlice != nil {
			copy(newSlice, tempNextLevelSlice)
		}
		return newSlice
	}
	if tempNextLevel != nil {
		return tempNextLevel
	}
	return make(map[string]interface{})
}

// Unflatten This function will unflatten a JSON
func Unflatten(inputMap map[*[]string]interface{}) (map[string]interface{}, error) {
	resultTree := make(map[string]interface{})

	keys := make([]*[]string, 0)
	for k := range inputMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Compare(strings.Join(*keys[i], compare.FieldSeparator), strings.Join(*keys[j], compare.FieldSeparator)) == -1
	})

	for _, key := range keys {
		keyParts := *key
		var currentLevel interface{}
		var nextLevel interface{}
		currentLevel = resultTree
		for partIndex, part := range keyParts[:len(keyParts)-1] {
			switch typedCurrentLevel := currentLevel.(type) {
			case map[string]interface{}:
				nextLevel = getNextLevel(keyParts, partIndex+1, typedCurrentLevel[part])
				typedCurrentLevel[part] = nextLevel
			case []interface{}:
				index, err := ExtractIntFromBrackets(part)
				if err != nil {
					return resultTree, err
				}
				nextLevel = getNextLevel(keyParts, partIndex+1, typedCurrentLevel[index])
				typedCurrentLevel[index] = nextLevel
			}
			currentLevel = nextLevel

		}
		switch typedCurrentLevel := currentLevel.(type) {
		case map[string]interface{}:
			typedCurrentLevel[keyParts[len(keyParts)-1]] = inputMap[key]
		case []interface{}:
			index, err := ExtractIntFromBrackets(keyParts[len(keyParts)-1])
			if err != nil {
				return resultTree, err
			}
			typedCurrentLevel[index] = inputMap[key]
		}
	}
	return resultTree, nil
}
