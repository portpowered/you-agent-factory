package builtingoal

import (
	"strings"
	"testing"
)

func TestAuthoredRolePrompt_ReturnsTrimmedPromptForKnownRoles(t *testing.T) {
	for role, want := range AuthoredRolePrompts {
		got, ok := AuthoredRolePrompt(role)
		if !ok {
			t.Fatalf("AuthoredRolePrompt(%q) ok = false, want true", role)
		}
		if got != strings.TrimSpace(want) {
			t.Fatalf("AuthoredRolePrompt(%q) = %q, want %q", role, got, strings.TrimSpace(want))
		}
	}
}

func TestAuthoredRolePrompt_RejectsUnknownRole(t *testing.T) {
	if got, ok := AuthoredRolePrompt("unknown-role"); ok || got != "" {
		t.Fatalf("AuthoredRolePrompt(unknown) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestInjectSummarizerBundledPrompt_RejectsMalformedSupportingFiles(t *testing.T) {
	cases := []struct {
		name string
		root map[string]any
		want string
	}{
		{
			name: "missing supportingFiles object",
			root: map[string]any{},
			want: "supportingFiles must be an object",
		},
		{
			name: "missing bundled files",
			root: map[string]any{
				"supportingFiles": map[string]any{},
			},
			want: "exactly one summarizer bundled file",
		},
		{
			name: "bundled file not object",
			root: map[string]any{
				"supportingFiles": map[string]any{
					"bundledFiles": []any{"not-an-object"},
				},
			},
			want: "must be an object",
		},
		{
			name: "wrong target path",
			root: map[string]any{
				"supportingFiles": map[string]any{
					"bundledFiles": []any{
						map[string]any{
							"targetPath": "factory/docs/wrong.md",
							"content":    map[string]any{},
						},
					},
				},
			},
			want: "targetPath",
		},
		{
			name: "content not object",
			root: map[string]any{
				"supportingFiles": map[string]any{
					"bundledFiles": []any{
						map[string]any{
							"targetPath": SummarizerPromptTargetPath,
							"content":    "inline-string",
						},
					},
				},
			},
			want: "content must be an object",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := injectSummarizerBundledPrompt(tc.root)
			if err == nil {
				t.Fatal("injectSummarizerBundledPrompt: expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("injectSummarizerBundledPrompt error = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAssembleBuiltInGoalFactoryJSON_RejectsMalformedWorkstations(t *testing.T) {
	t.Run("workstations not array", func(t *testing.T) {
		root := map[string]any{
			"workstations": "not-an-array",
			"supportingFiles": map[string]any{
				"bundledFiles": []any{
					map[string]any{
						"targetPath": SummarizerPromptTargetPath,
						"content":    map[string]any{},
					},
				},
			},
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstations must be an array") {
			t.Fatalf("assemble error = %v, want workstations array validation", err)
		}
	})

	t.Run("workstation entry not object", func(t *testing.T) {
		root := map[string]any{
			"workstations": []any{"not-an-object"},
			"supportingFiles": map[string]any{
				"bundledFiles": []any{
					map[string]any{
						"targetPath": SummarizerPromptTargetPath,
						"content":    map[string]any{},
					},
				},
			},
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstation entry must be an object") {
			t.Fatalf("assemble error = %v, want workstation object validation", err)
		}
	})
}
