package compare

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNestedString(t *testing.T) {
	cases := []struct {
		name        string
		obj         any
		fields      []string
		expectError bool
		expectFound bool
		expected    string
	}{
		{
			name:        "nil object",
			obj:         nil,
			fields:      []string{"foo"},
			expectError: false,
			expectFound: false,
			expected:    "",
		},
		{
			name: "trivial case",
			obj: map[string]any{
				"foo": "bar",
			},
			fields:      []string{"foo"},
			expectError: false,
			expectFound: true,
			expected:    "bar",
		},
		{
			name: "missing",
			obj: map[string]any{
				"foo": "bar",
			},
			fields:      []string{"blee"},
			expectError: false,
			expectFound: false,
			expected:    "",
		},
		{
			name: "overshoot",
			obj: map[string]any{
				"foo": "bar",
			},
			fields:      []string{"foo", "blee"},
			expectError: true,
			expectFound: false,
			expected:    "",
		},
		{
			name: "deep structs",
			obj: map[string]any{
				"one": map[string]any{
					"two": map[string]any{
						"three": "answer",
					},
				},
			},
			fields:      []string{"one", "two", "three"},
			expectError: false,
			expectFound: true,
			expected:    "answer",
		},
		{
			name: "falling short",
			obj: map[string]any{
				"one": map[string]any{
					"two": map[string]any{
						"three": "answer",
					},
				},
			},
			fields:      []string{"one", "two"},
			expectError: true,
			expectFound: true,
			expected:    "",
		},
		{
			name: "with a slice",
			obj: map[string]any{
				"one": map[string]any{
					"two": []any{
						map[string]any{
							"three": "answer",
						},
						map[string]any{
							"threeA": "answerA",
						},
					},
				},
			},
			fields:      []string{"one", "two", "0", "three"},
			expectError: false,
			expectFound: true,
			expected:    "answer",
		},
		{
			name: "bad slice index",
			obj: map[string]any{
				"one": map[string]any{
					"two": []any{
						map[string]any{
							"three": "answer",
						},
						map[string]any{
							"threeA": "answerA",
						},
					},
				},
			},
			fields:      []string{"one", "two", "three"},
			expectError: true,
			expectFound: false,
			expected:    "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			val, found, err := NestedString(c.obj, c.fields...)
			if c.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, c.expectFound, found, "Expecting found=%v", c.expectFound)
			assert.Equal(t, c.expected, val)
		})
	}
}

func TestSetNestedString(t *testing.T) {
	obj := map[string]any{
		"foo": "unset",
		"one": map[string]any{
			"two": map[string]any{
				"three": "unset",
			},
		},
		"a": map[string]any{
			"b": []any{
				map[string]any{
					"c": "unset",
				},
			},
		},
	}
	for _, c := range []string{
		"foo",
		"one.two.three",
		"a.b.0.c",
	} {
		t.Run(c, func(t *testing.T) {
			fields := strings.Split(c, ".")
			err := SetNestedString(obj, "set", fields...)
			require.NoError(t, err)
			result, found, err := NestedString(obj, fields...)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, "set", result)
		})
	}
}
