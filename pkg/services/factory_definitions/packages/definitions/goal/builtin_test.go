package builtingoal_test

import (
	"encoding/json"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"os"
	"testing"

	builtingoal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/goal"
)

const executorPromptPath = "prompts/executor.md"

func TestBuiltInGoalFactoryJSON_AssemblesFromDeclarativePromptAssets(t *testing.T) {
	wantPrompt, err := os.ReadFile(executorPromptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", executorPromptPath, err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	if len(cfg.Workers) != 1 || cfg.Workers[0].Name != "goal-executor" {
		t.Fatalf("workers = %#v, want goal-executor", cfg.Workers)
	}
	if cfg.Workers[0].Body != string(wantPrompt) {
		t.Fatalf("goal-executor body does not exactly match authored asset")
	}
	if len(cfg.Workstations) != 1 || cfg.Workstations[0].Name != "execute-goal" {
		t.Fatalf("workstations = %#v, want execute-goal", cfg.Workstations)
	}
	if cfg.Workstations[0].Body != string(wantPrompt) {
		t.Fatalf("execute-goal body does not exactly match authored asset")
	}
	if cfg.Workstations[0].PromptFile != executorPromptPath {
		t.Fatalf("execute-goal promptFile = %q, want %q", cfg.Workstations[0].PromptFile, executorPromptPath)
	}
}

func TestBuiltInGoalFactoryJSON_DeclaresPromptsWithoutInlineBodies(t *testing.T) {
	var authored map[string]any
	if err := json.Unmarshal(builtingoal.FactoryJSON(), &authored); err != nil {
		t.Fatalf("unmarshal authored factory.json: %v", err)
	}

	assertDeclaredGoalPrompt(t, authored, "workers", "goal-executor")
	assertDeclaredGoalPrompt(t, authored, "workstations", "execute-goal")
}

func assertDeclaredGoalPrompt(t *testing.T, authored map[string]any, collection, name string) {
	t.Helper()
	entries, ok := authored[collection].([]any)
	if !ok {
		t.Fatalf("authored factory.json %s must be an array", collection)
	}
	for _, entry := range entries {
		subject, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("authored %s entry must be an object", collection)
		}
		if subject["name"] != name {
			continue
		}
		if got := subject["promptFile"]; got != executorPromptPath {
			t.Fatalf("authored %s %q promptFile = %q, want %q", collection, name, got, executorPromptPath)
		}
		if _, hasBody := subject["body"]; hasBody {
			t.Fatalf("authored %s %q must not inline prompt body", collection, name)
		}
		return
	}
	t.Fatalf("authored %s missing %q", collection, name)
}
