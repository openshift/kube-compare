// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNormalizeInlineDiffReference(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		options  []InlineDiffOption
		expected string
	}{
		{
			name:  "hash comment lines with LF",
			value: "keep\n  # generated hash\nvalue # inline marker\nfoo#bar\n",
			options: []InlineDiffOption{
				IgnoreReferenceHashCommentLines,
			},
			expected: "keep\nvalue # inline marker\nfoo#bar\n",
		},
		{
			name:  "slash comment lines with CRLF",
			value: "keep\r\n\t// generated comment\r\nhttps://example.test/#fragment\r\n",
			options: []InlineDiffOption{
				IgnoreReferenceSlashCommentLines,
			},
			expected: "keep\r\nhttps://example.test/#fragment\r\n",
		},
		{
			name:  "options are order independent",
			value: "# hash\n// slash\nkeep\n",
			options: []InlineDiffOption{
				IgnoreReferenceSlashCommentLines,
				IgnoreReferenceHashCommentLines,
			},
			expected: "keep\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, NormalizeInlineDiffReference(test.value, test.options))
		})
	}
}

func TestValidateInlineDiffOptions(t *testing.T) {
	require.NoError(t, ValidateInlineDiffOptions([]InlineDiffOption{
		IgnoreReferenceHashCommentLines,
		IgnoreReferenceSlashCommentLines,
	}))
	require.EqualError(t,
		ValidateInlineDiffOptions([]InlineDiffOption{"unknown"}),
		`inlineDiffOptions[0] has unknown option "unknown"`,
	)
	require.EqualError(t,
		ValidateInlineDiffOptions([]InlineDiffOption{
			IgnoreReferenceHashCommentLines,
			IgnoreReferenceHashCommentLines,
		}),
		`inlineDiffOptions[1] duplicates option "ignoreReferenceHashCommentLines"`,
	)
}

func TestReferenceV2ValidateConfigPerFieldNormalizesReference(t *testing.T) {
	reference := ReferenceTemplateV2{
		Config: ReferenceTemplateConfigV2{PerField: []*PerFieldConfigV2{
			{
				PathToKey:      "data.value",
				InlineDiffFunc: regex,
				InlineDiffOptions: []InlineDiffOption{
					IgnoreReferenceHashCommentLines,
				},
			},
		}},
		ReferenceTemplateV1: ReferenceTemplateV1{metadata: &unstructured.Unstructured{Object: map[string]any{
			"data": map[string]any{"value": "# invalid regex (?<\n(?<name>[a-z]+)"},
		}}},
	}

	require.NoError(t, reference.validateConfigPerField())
}

func TestReferenceV2ValidateConfigPerFieldReportsOptionPath(t *testing.T) {
	tests := []struct {
		name     string
		options  []InlineDiffOption
		expected string
	}{
		{
			name:     "unknown option",
			options:  []InlineDiffOption{"unknown"},
			expected: `reference contains template with config.perField[0] pathToKey "data.value" with invalid inlineDiffOptions: inlineDiffOptions[0] has unknown option "unknown"`,
		},
		{
			name: "duplicate option",
			options: []InlineDiffOption{
				IgnoreReferenceHashCommentLines,
				IgnoreReferenceHashCommentLines,
			},
			expected: `reference contains template with config.perField[0] pathToKey "data.value" with invalid inlineDiffOptions: inlineDiffOptions[1] duplicates option "ignoreReferenceHashCommentLines"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reference := ReferenceTemplateV2{
				Config: ReferenceTemplateConfigV2{PerField: []*PerFieldConfigV2{
					{
						PathToKey:         "data.value",
						InlineDiffFunc:    regex,
						InlineDiffOptions: test.options,
					},
				}},
			}
			require.EqualError(t, reference.validateConfigPerField(), test.expected)
		})
	}
}

func TestGetInlineDiffFuncsRetainsOptions(t *testing.T) {
	config := ReferenceTemplateConfigV2{PerField: []*PerFieldConfigV2{
		{
			PathToKey:      "data.value",
			InlineDiffFunc: regex,
			InlineDiffOptions: []InlineDiffOption{
				IgnoreReferenceHashCommentLines,
			},
		},
	}}

	require.Equal(t, map[string]InlineDiffConfig{
		"data.value": {
			InlineDiffFunc: regex,
			InlineDiffOptions: []InlineDiffOption{
				IgnoreReferenceHashCommentLines,
			},
		},
	}, config.GetInlineDiffFuncs())
}

func TestInlineDiffOptionsNormalizeOnlyReferenceAtRuntime(t *testing.T) {
	liveRegex := "release: 42 # inline marker\nendpoint: https://example.test/#fragment"
	liveCapturegroups := "release: 42 # inline marker\nendpoint: https://example.test/#fragment"
	obj := InfoObject{
		injectedObjFromTemplate: &unstructured.Unstructured{Object: map[string]any{
			"data": map[string]any{
				"regex":         "# invalid regex (?<\n// invalid regex (\nrelease: (?<version>[0-9]+) # inline marker\nendpoint: https://example.test/#fragment",
				"capturegroups": "# generated hash\n// generated slash\nrelease: (?<version>[0-9]+) # inline marker\nendpoint: https://example.test/#fragment",
			},
		}},
		clusterObj: &unstructured.Unstructured{Object: map[string]any{
			"data": map[string]any{
				"regex":         liveRegex,
				"capturegroups": liveCapturegroups,
			},
		}},
		templateFieldConf: map[string]InlineDiffConfig{
			"data.regex": {
				InlineDiffFunc: regex,
				InlineDiffOptions: []InlineDiffOption{
					IgnoreReferenceSlashCommentLines,
					IgnoreReferenceHashCommentLines,
				},
			},
			"data.capturegroups": {
				InlineDiffFunc: capturegroups,
				InlineDiffOptions: []InlineDiffOption{
					IgnoreReferenceHashCommentLines,
					IgnoreReferenceSlashCommentLines,
				},
			},
		},
	}

	require.NoError(t, obj.runInlineDiffFuncs())
	regexValue, _, err := NestedString(obj.injectedObjFromTemplate.Object, "data", "regex")
	require.NoError(t, err)
	require.Equal(t, liveRegex, regexValue)
	capturegroupsValue, _, err := NestedString(obj.injectedObjFromTemplate.Object, "data", "capturegroups")
	require.NoError(t, err)
	require.Equal(t, liveCapturegroups, capturegroupsValue)

	unchangedLiveRegex, _, err := NestedString(obj.clusterObj.Object, "data", "regex")
	require.NoError(t, err)
	require.Equal(t, liveRegex, unchangedLiveRegex)
}
