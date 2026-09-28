// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNormalizeInlineDiffValue(t *testing.T) {
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
				IgnoreHashComments,
			},
			expected: "keep\nvalue # inline marker\nfoo#bar\n",
		},
		{
			name:  "slash comment lines with CRLF",
			value: "keep\r\n\t// generated comment\r\nhttps://example.test/#fragment\r\n",
			options: []InlineDiffOption{
				IgnoreSlashComments,
			},
			expected: "keep\r\nhttps://example.test/#fragment\r\n",
		},
		{
			name:  "options are order independent",
			value: "# hash\n// slash\nkeep\n",
			options: []InlineDiffOption{
				IgnoreSlashComments,
				IgnoreHashComments,
			},
			expected: "keep\n",
		},
		{
			name:  "final hash comment without trailing LF",
			value: "keep\n# note",
			options: []InlineDiffOption{
				IgnoreHashComments,
			},
			expected: "keep",
		},
		{
			name:  "final slash comment without trailing CRLF",
			value: "keep\r\n// note",
			options: []InlineDiffOption{
				IgnoreSlashComments,
			},
			expected: "keep",
		},
		{
			name:     "no options preserve value and line endings",
			value:    "# hash\r\n// slash\r\nvalue # inline marker\r\n",
			expected: "# hash\r\n// slash\r\nvalue # inline marker\r\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, NormalizeInlineDiffValue(test.value, test.options))
		})
	}
}

func TestValidateInlineDiffOptions(t *testing.T) {
	require.NoError(t, ValidateInlineDiffOptions([]InlineDiffOption{
		IgnoreHashComments,
		IgnoreSlashComments,
	}))
	require.EqualError(t,
		ValidateInlineDiffOptions([]InlineDiffOption{"unknown"}),
		`inlineDiffOptions[0] has unknown option "unknown"`,
	)
	require.EqualError(t,
		ValidateInlineDiffOptions([]InlineDiffOption{
			IgnoreHashComments,
			IgnoreHashComments,
		}),
		`inlineDiffOptions[1] duplicates option "ignoreHashComments"`,
	)
}

func TestReferenceV2ValidateConfigPerFieldNormalizesReference(t *testing.T) {
	reference := ReferenceTemplateV2{
		Config: ReferenceTemplateConfigV2{PerField: []*PerFieldConfigV2{
			{
				PathToKey:      "data.value",
				InlineDiffFunc: regex,
				InlineDiffOptions: []InlineDiffOption{
					IgnoreHashComments,
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
				IgnoreHashComments,
				IgnoreHashComments,
			},
			expected: `reference contains template with config.perField[0] pathToKey "data.value" with invalid inlineDiffOptions: inlineDiffOptions[1] duplicates option "ignoreHashComments"`,
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
				IgnoreHashComments,
			},
		},
	}}

	require.Equal(t, map[string]InlineDiffConfig{
		"data.value": {
			InlineDiffFunc: regex,
			InlineDiffOptions: []InlineDiffOption{
				IgnoreHashComments,
			},
		},
	}, config.GetInlineDiffFuncs())
}

func TestInlineDiffOptionsNormalizeBothSidesAtRuntime(t *testing.T) {
	pattern := "release: (?<version>[0-9]+) # inline marker\nendpoint: https://example.test/#fragment"
	liveValue := "release: 42 # inline marker\nendpoint: https://example.test/#fragment"
	tests := []struct {
		name            string
		inlineDiffFunc  InlineDiffType
		referencePrefix string
		livePrefix      string
	}{
		{
			name:            "regex with reference-only comments",
			inlineDiffFunc:  regex,
			referencePrefix: "# reference hash\n// reference slash\n",
		},
		{
			name:           "regex with live-only comments",
			inlineDiffFunc: regex,
			livePrefix:     "# live hash\n// live slash\n",
		},
		{
			name:            "regex with different comments on both sides",
			inlineDiffFunc:  regex,
			referencePrefix: "# reference hash\n// reference slash\n",
			livePrefix:      "# live hash\n// live slash\n",
		},
		{
			name:            "capturegroups with reference-only comments",
			inlineDiffFunc:  capturegroups,
			referencePrefix: "# reference hash\n// reference slash\n",
		},
		{
			name:           "capturegroups with live-only comments",
			inlineDiffFunc: capturegroups,
			livePrefix:     "# live hash\n// live slash\n",
		},
		{
			name:            "capturegroups with different comments on both sides",
			inlineDiffFunc:  capturegroups,
			referencePrefix: "# reference hash\n// reference slash\n",
			livePrefix:      "# live hash\n// live slash\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalLiveValue := test.livePrefix + liveValue
			originalLive := &unstructured.Unstructured{Object: map[string]any{
				"data": map[string]any{"value": originalLiveValue},
			}}
			obj := InfoObject{
				injectedObjFromTemplate: &unstructured.Unstructured{Object: map[string]any{
					"data": map[string]any{"value": test.referencePrefix + pattern},
				}},
				clusterObj:        originalLive,
				comparisonLiveObj: originalLive.DeepCopy(),
				templateFieldConf: map[string]InlineDiffConfig{
					"data.value": {
						InlineDiffFunc: test.inlineDiffFunc,
						InlineDiffOptions: []InlineDiffOption{
							IgnoreHashComments,
							IgnoreSlashComments,
						},
					},
				},
			}

			require.NoError(t, obj.runInlineDiffFuncs())

			mergedValue, _, err := NestedString(obj.injectedObjFromTemplate.Object, "data", "value")
			require.NoError(t, err)
			require.Equal(t, liveValue, mergedValue)

			comparisonValue, _, err := NestedString(obj.comparisonLiveObj.Object, "data", "value")
			require.NoError(t, err)
			require.Equal(t, liveValue, comparisonValue)
			require.Same(t, obj.comparisonLiveObj, obj.Live())
			override, err := CreateMergePatch(ReferenceTemplateV2{
				ReferenceTemplateV1: ReferenceTemplateV1{Path: "template.yaml"},
			}, &obj, "test")
			require.NoError(t, err)
			require.JSONEq(t, `{}`, override.Patch)

			unchangedLiveValue, _, err := NestedString(originalLive.Object, "data", "value")
			require.NoError(t, err)
			require.Equal(t, originalLiveValue, unchangedLiveValue)
		})
	}
}

func TestInlineDiffOptionsNoOptionsPreserveExistingBehavior(t *testing.T) {
	referenceValue := "# reference comment\nrelease: (?<version>[0-9]+)"
	liveValue := "# live comment\nrelease: 42"
	originalLive := &unstructured.Unstructured{Object: map[string]any{
		"data": map[string]any{"value": liveValue},
	}}
	obj := InfoObject{
		injectedObjFromTemplate: &unstructured.Unstructured{Object: map[string]any{
			"data": map[string]any{"value": referenceValue},
		}},
		clusterObj:        originalLive,
		comparisonLiveObj: originalLive.DeepCopy(),
		templateFieldConf: map[string]InlineDiffConfig{
			"data.value": {InlineDiffFunc: regex},
		},
	}

	require.NoError(t, obj.runInlineDiffFuncs())

	mergedValue, _, err := NestedString(obj.injectedObjFromTemplate.Object, "data", "value")
	require.NoError(t, err)
	require.Equal(t, referenceValue, mergedValue)
	comparisonValue, _, err := NestedString(obj.comparisonLiveObj.Object, "data", "value")
	require.NoError(t, err)
	require.Equal(t, liveValue, comparisonValue)
	unchangedLiveValue, _, err := NestedString(originalLive.Object, "data", "value")
	require.NoError(t, err)
	require.Equal(t, liveValue, unchangedLiveValue)
}
