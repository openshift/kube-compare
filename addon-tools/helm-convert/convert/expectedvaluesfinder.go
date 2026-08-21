package convert

import "text/template/parse"

type ExpectedValuesFinder struct {
	expected [][]string
}

func (v *ExpectedValuesFinder) Visit() func(parse.Node) bool {
	return func(node parse.Node) bool {
		if node == nil {
			return false
		}
		switch n := node.(type) {
		case *parse.FieldNode:
			v.expected = append(v.expected, getNodeField(n)...)
		case *parse.RangeNode:
			for _, fieldRangedOn := range getFieldsAccessInNode(n.Pipe) { // for complex range statements get the path of the map/slice to be ranged on
				// in case of range, the minimum list length will be always in length 1, including only element with index 0:
				completeField := append(fieldRangedOn, "[0]") //nolint:gocritic
				v.expected = append(v.expected, completeField)
				v.expected = append(v.expected, getFieldsFromNodeWithPrefix(n.List, completeField)...)
				if n.ElseList != nil {
					v.expected = append(v.expected, getFieldsFromNodeWithPrefix(n.ElseList, completeField)...)
				}
			}
			return false
		case *parse.CommandNode:
			if n.Args[0].Type() == parse.NodeIdentifier && n.Args[0].(*parse.IdentifierNode).Ident == "index" {
				for _, fieldRangedOn := range getFieldsAccessInNode(n.Args[1]) { // n.Args[1] first argument to index node - may be a complex statement for example {{ index (index . x) y }}
					var text string
					switch node := n.Args[2].(type) { // n.Args[2] is the second argument for index function
					case *parse.NumberNode:
						text = node.Text
					case *parse.StringNode:
						text = node.Text
					}
					v.expected = append(v.expected, append(fieldRangedOn, text))
				}
				return false
			}
		}
		return true
	}

}

func getNodeField(n *parse.FieldNode) [][]string {
	return append([][]string{}, n.Ident)
}

func getFieldsAccessInNode(node parse.Node) [][]string {
	v := ExpectedValuesFinder{}
	Inspect(node, v.Visit())
	return v.expected
}

func getFieldsFromNodeWithPrefix(node parse.Node, prefix []string) [][]string {
	var fields [][]string
	for _, field := range getFieldsAccessInNode(node) {
		prefixCopy := append([]string(nil), prefix...)
		fields = append(fields, append(prefixCopy, field...))
	}
	return fields
}
