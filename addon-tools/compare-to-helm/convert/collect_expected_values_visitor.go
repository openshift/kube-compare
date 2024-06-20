package convert

import "text/template/parse"

func getNodeField(n *parse.FieldNode) [][]string {
	var result [][]string
	result = append(result, n.Ident)
	return result
}

type CollectExpectedVisitor struct {
	expected [][]string
}

func (v *CollectExpectedVisitor) Visit() func(parse.Node) bool {
	return func(node parse.Node) bool {
		if node == nil {
			return false
		}
		switch n := node.(type) {
		case *parse.FieldNode:

			v.expected = append(v.expected, getNodeField(n)...)
			break
		case *parse.RangeNode:
			visitor := CollectExpectedVisitor{}
			Inspect(n.Pipe, visitor.Visit())
			for _, fieldRangedOn := range visitor.expected {
				completeField0 := append(fieldRangedOn, "0")
				v.expected = append(v.expected, completeField0)
				// if
				visitor2 := CollectExpectedVisitor{}
				Inspect(n.List, visitor2.Visit())
				for _, field := range visitor2.expected {
					completeField := append(fieldRangedOn, "0")
					v.expected = append(v.expected, append(completeField, field...))
				}
				// else
				if n.ElseList != nil {
					visitor2 = CollectExpectedVisitor{}
					Inspect(n.ElseList, visitor2.Visit())
					for _, field := range visitor2.expected {
						completeField := append(fieldRangedOn, "0")
						v.expected = append(v.expected, append(completeField, field...))
					}
				}
			}
			return false
		case *parse.CommandNode:
			if n.Args[0].Type() == parse.NodeIdentifier && n.Args[0].(*parse.IdentifierNode).Ident == "index" {
				visto := CollectExpectedVisitor{}
				Inspect(n.Args[1], visto.Visit())
				for _, fieldRangedOn := range visto.expected {
					var text string
					switch requestedIndex := n.Args[2].(type) {
					case *parse.NumberNode:
						text = requestedIndex.Text
						break
					case *parse.StringNode:
						text = requestedIndex.Text
					}
					v.expected = append(v.expected, append(fieldRangedOn, text))
				}
				return false
			}
		}
		return true
	}

}
