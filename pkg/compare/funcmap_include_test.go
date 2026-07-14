// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"strings"
	"testing"
	"text/template"
)

func TestIncludeFun(t *testing.T) {
	tests := []struct {
		name        string
		templateStr string
		expectOut   string
		expectErr   string
	}{
		{
			name:        "basic include",
			templateStr: `{{- define "greeting" -}}hello world{{- end -}}{{ include "greeting" . }}`,
			expectOut:   "hello world",
		},
		{
			name:        "include with data",
			templateStr: `{{- define "greet" -}}hello {{ .name }}{{- end -}}{{ include "greet" . }}`,
			expectOut:   "hello test-user",
		},
		{
			name:        "include piped to upper",
			templateStr: `{{- define "msg" -}}hello{{- end -}}{{ include "msg" . | upper }}`,
			expectOut:   "HELLO",
		},
		{
			name:        "include piped to indent",
			templateStr: `{{- define "block" -}}line1` + "\n" + `line2{{- end -}}{{ include "block" . | indent 2 }}`,
			expectOut:   "  line1\n  line2",
		},
		{
			name:        "nested include",
			templateStr: `{{- define "inner" -}}inner{{- end -}}{{- define "outer" -}}[{{ include "inner" . }}]{{- end -}}{{ include "outer" . }}`,
			expectOut:   "[inner]",
		},
		{
			name:        "include non-existent template returns error",
			templateStr: `{{ include "does-not-exist" . }}`,
			expectErr:   "does-not-exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := template.New("test").Funcs(FuncMap())
			tmpl, err := tmpl.Parse(tt.templateStr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			InitInclude(tmpl)

			data := map[string]any{"name": "test-user"}
			var buf strings.Builder
			err = tmpl.Execute(&buf, data)
			if tt.expectErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectErr)
				}
				if !strings.Contains(err.Error(), tt.expectErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := buf.String(); got != tt.expectOut {
				t.Errorf("output mismatch:\n  got:  %q\n  want: %q", got, tt.expectOut)
			}
		})
	}
}

func TestIncludeRecursionGuard(t *testing.T) {
	// A template that includes itself should hit the recursion limit
	tmpl := template.New("test").Funcs(FuncMap())
	tmpl, err := tmpl.Parse(`{{- define "loop" -}}{{ include "loop" . }}{{- end -}}{{ include "loop" . }}`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	InitInclude(tmpl)

	var buf strings.Builder
	err = tmpl.Execute(&buf, nil)
	if err == nil {
		t.Fatal("expected recursion error, got nil")
	}
	if !strings.Contains(err.Error(), "nested reference") {
		t.Fatalf("expected recursion error, got: %v", err)
	}
}
