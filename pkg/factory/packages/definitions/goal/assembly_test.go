package builtingoal

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

func TestBuiltInGoalFactoryJSON_AssemblesExactDeclaredPromptAsset(t *testing.T) {
	want, err := fs.ReadFile(promptAssets, "prompts/executor.md")
	if err != nil {
		t.Fatalf("read embedded executor prompt: %v", err)
	}

	var assembled map[string]any
	if err := json.Unmarshal(BuiltInGoalFactoryJSON, &assembled); err != nil {
		t.Fatalf("unmarshal assembled goal definition: %v", err)
	}
	assertAssembledGoalPrompt(t, assembled, "workers", "goal-executor", string(want), false)
	assertAssembledGoalPrompt(t, assembled, "workstations", "execute-goal", string(want), true)
}

func TestBuiltInGoalFactoryJSON_DoesNotUseLegacyTopLevelWorkIDTemplateAlias(t *testing.T) {
	if strings.Contains(string(BuiltInGoalFactoryJSON), "{{ .WorkID }}") {
		t.Fatal("BuiltInGoalFactoryJSON still contains legacy top-level WorkID template alias")
	}
}

func assertAssembledGoalPrompt(t *testing.T, assembled map[string]any, collection, name, want string, wantPromptFile bool) {
	t.Helper()
	for _, entry := range assembled[collection].([]any) {
		subject := entry.(map[string]any)
		if subject["name"] != name {
			continue
		}
		if got := subject["body"]; got != want {
			t.Fatalf("assembled %s %q body = %q, want exact authored prompt %q", collection, name, got, want)
		}
		_, hasPromptFile := subject["promptFile"]
		if hasPromptFile != wantPromptFile {
			t.Fatalf("assembled %s %q promptFile presence = %v, want %v", collection, name, hasPromptFile, wantPromptFile)
		}
		return
	}
	t.Fatalf("assembled %s missing %q", collection, name)
}
