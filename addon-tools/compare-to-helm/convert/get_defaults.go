package convert

import (
	"fmt"
	"strings"

	"k8s.io/client-go/util/jsonpath"
	"k8s.io/kubectl/pkg/cmd/get"
)

// Function to get values from JSON using jsonpath expressions
func getValuesFromJSONPaths(jsonData map[string]interface{}, paths [][]string) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	for _, pathParts := range paths {
		path := strings.Join(pathParts, ".")
		//path = strings.ReplaceAll(path, ".0", "[0]")
		jsonPathExpr := "." + strings.Join(pathParts, ".")
		jsonPathExpr, err := get.RelaxedJSONPathExpression(jsonPathExpr)
		jsonPathExpr = strings.ReplaceAll(jsonPathExpr, ".0", "[0]")
		if err != nil {
			panic(err)
		}
		jp := jsonpath.New("jsonpath")
		if err := jp.Parse(jsonPathExpr); err != nil {
			return nil, fmt.Errorf("error parsing jsonpath expression %q: %v", jsonPathExpr, err)
		}

		result, err := jp.FindResults(jsonData)
		if err != nil {
			results[path] = make(map[string]interface{})
		}

		if len(result) > 0 && len(result[0]) > 0 {
			results[path] = result[0][0].Interface()
		}
	}

	return results, nil
}
