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

func TestBuiltInGoalFactoryJSON_DoesNotUseLegacyTopLevelWorkIDTemplateAlias(t *testing.T) {
	if strings.Contains(string(BuiltInGoalFactoryJSON), "{{ .WorkID }}") {
		t.Fatal("BuiltInGoalFactoryJSON still contains legacy top-level WorkID template alias")
	}
}

func TestAssembleBuiltInGoalFactoryJSON_RejectsMalformedWorkstations(t *testing.T) {
	t.Run("workers not array", func(t *testing.T) {
		root := map[string]any{
			"workers": "not-an-array",
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workers must be an array") {
			t.Fatalf("assemble error = %v, want workers array validation", err)
		}
	})

	t.Run("worker entry not object", func(t *testing.T) {
		root := map[string]any{
			"workers": []any{"not-an-object"},
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "worker entry must be an object") {
			t.Fatalf("assemble error = %v, want worker object validation", err)
		}
	})

	t.Run("workstations not array", func(t *testing.T) {
		root := map[string]any{
			"workers":      []any{},
			"workstations": "not-an-array",
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstations must be an array") {
			t.Fatalf("assemble error = %v, want workstations array validation", err)
		}
	})

	t.Run("workstation entry not object", func(t *testing.T) {
		root := map[string]any{
			"workers":      []any{},
			"workstations": []any{"not-an-object"},
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstation entry must be an object") {
			t.Fatalf("assemble error = %v, want workstation object validation", err)
		}
	})
}
