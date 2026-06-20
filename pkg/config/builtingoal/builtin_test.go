package builtingoal_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/builtingoal"
)

func TestBuiltInGoalFactoryJSON_AssemblesFromAuthoredPromptFiles(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	workstationBodies := map[string]string{}
	for _, workstation := range cfg.Workstations {
		workstationBodies[workstation.Name] = strings.TrimSpace(workstation.Body)
	}

	for role, authoredPrompt := range builtingoal.AuthoredRolePrompts {
		authoredPrompt = strings.TrimSpace(authoredPrompt)
		if authoredPrompt == "" {
			t.Fatalf("authored prompt for role %q is empty", role)
		}
	}

	if got := workstationBodies["plan-goal"]; got != strings.TrimSpace(builtingoal.AuthoredRolePrompts["planner"]) {
		t.Fatalf("plan-goal body does not match authored planner prompt")
	}
	if got := workstationBodies["execute-goal"]; got != strings.TrimSpace(builtingoal.AuthoredRolePrompts["executor"]) {
		t.Fatalf("execute-goal body does not match authored executor prompt")
	}
	if got := workstationBodies["check-goal"]; got != strings.TrimSpace(builtingoal.AuthoredRolePrompts["checker"]) {
		t.Fatalf("check-goal body does not match authored checker prompt")
	}
	if got := workstationBodies["review-goal"]; got != strings.TrimSpace(builtingoal.AuthoredRolePrompts["reviewer"]) {
		t.Fatalf("review-goal body does not match authored reviewer prompt")
	}

	if cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) != 1 {
		t.Fatalf("resource manifest bundled files = %#v, want one summarizer doc", cfg.ResourceManifest)
	}
	bundled := cfg.ResourceManifest.BundledFiles[0]
	if bundled.TargetPath != builtingoal.SummarizerPromptTargetPath {
		t.Fatalf("summarizer bundled targetPath = %q, want %q", bundled.TargetPath, builtingoal.SummarizerPromptTargetPath)
	}
	if strings.TrimSpace(bundled.Content.Inline) != strings.TrimSpace(builtingoal.AuthoredRolePrompts["summarizer"]) {
		t.Fatalf("summarizer bundled inline content does not match authored summarizer prompt")
	}

	var raw map[string]any
	if err := json.Unmarshal(builtingoal.FactoryJSON(), &raw); err != nil {
		t.Fatalf("unmarshal authored factory.json: %v", err)
	}
	workstations, ok := raw["workstations"].([]any)
	if !ok {
		t.Fatal("authored factory.json workstations must be an array")
	}
	for _, entry := range workstations {
		workstation, ok := entry.(map[string]any)
		if !ok {
			t.Fatal("authored workstation entry must be an object")
		}
		if _, hasBody := workstation["body"]; hasBody {
			t.Fatalf("authored workstation %q must not inline prompt body in factory.json", workstation["name"])
		}
	}
}
