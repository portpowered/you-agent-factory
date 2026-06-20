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

func TestSupplementaryWorkstationPromptFiles_ReviewGoalHostsSummarizerPrompt(t *testing.T) {
	got := SupplementaryWorkstationPromptFiles("review-goal")
	wantPath := "prompts/summarizer.md"
	prompt, ok := got[wantPath]
	if !ok {
		t.Fatalf("supplementary prompts = %#v, want %q entry", got, wantPath)
	}
	if prompt != strings.TrimSpace(summarizerPrompt) {
		t.Fatalf("supplementary summarizer prompt does not match authored source")
	}
}

func TestSupplementaryWorkstationPromptFiles_OmitsUnknownWorkstations(t *testing.T) {
	if got := SupplementaryWorkstationPromptFiles("plan-goal"); got != nil {
		t.Fatalf("supplementary prompts for plan-goal = %#v, want nil", got)
	}
}

func TestAssembleBuiltInGoalFactoryJSON_RejectsMalformedWorkstations(t *testing.T) {
	t.Run("workstations not array", func(t *testing.T) {
		root := map[string]any{
			"workstations": "not-an-array",
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstations must be an array") {
			t.Fatalf("assemble error = %v, want workstations array validation", err)
		}
	})

	t.Run("workstation entry not object", func(t *testing.T) {
		root := map[string]any{
			"workstations": []any{"not-an-object"},
		}
		_, err := assembleBuiltInGoalFactoryJSONFromRoot(root)
		if err == nil || !strings.Contains(err.Error(), "workstation entry must be an object") {
			t.Fatalf("assemble error = %v, want workstation object validation", err)
		}
	})
}
