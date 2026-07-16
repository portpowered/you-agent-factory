package builtinsubagent_test

import (
	"encoding/json"
	"os"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	builtinsubagent "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/subagent"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

const (
	workerPromptPath      = "prompts/worker.md"
	workstationPromptPath = "prompts/run-subagent.md"
)

func TestBuiltInSubagentFactoryJSON_AssemblesFromDeclarativePromptAssets(t *testing.T) {
	wantWorkerPrompt := readPromptAsset(t, workerPromptPath)
	wantWorkstationPrompt := readPromptAsset(t, workstationPromptPath)
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinsubagent.BuiltInSubagentFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	if len(cfg.Workers) != 1 || cfg.Workers[0].Name != "subagent-worker" {
		t.Fatalf("workers = %#v, want subagent-worker", cfg.Workers)
	}
	if cfg.Workers[0].Body != wantWorkerPrompt {
		t.Fatal("subagent-worker body does not exactly match authored asset")
	}
	if cfg.Workers[0].AgentTools == nil || cfg.Workers[0].AgentTools.Policy != workerconfig.AgentToolPolicyReadOnly {
		t.Fatalf("subagent-worker agentTools = %#v, want READ_ONLY policy", cfg.Workers[0].AgentTools)
	}
	if len(cfg.Workstations) != 1 || cfg.Workstations[0].Name != "run-subagent" {
		t.Fatalf("workstations = %#v, want run-subagent", cfg.Workstations)
	}
	if cfg.Workstations[0].Body != wantWorkstationPrompt {
		t.Fatal("run-subagent body does not exactly match authored asset")
	}
	if cfg.Workstations[0].PromptFile != workstationPromptPath {
		t.Fatalf("run-subagent promptFile = %q, want %q", cfg.Workstations[0].PromptFile, workstationPromptPath)
	}
}

func TestBuiltInSubagentFactoryJSON_DeclaresPromptsWithoutInlineBodies(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal(builtinsubagent.FactoryJSON(), &raw); err != nil {
		t.Fatalf("unmarshal authored factory.json: %v", err)
	}

	assertDeclaredPrompt(t, raw, "workers", "subagent-worker", workerPromptPath)
	assertDeclaredPrompt(t, raw, "workstations", "run-subagent", workstationPromptPath)
}

func readPromptAsset(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(content)
}

func assertDeclaredPrompt(t *testing.T, raw map[string]any, collection, name, promptPath string) {
	t.Helper()
	entries, ok := raw[collection].([]any)
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
		if got := subject["promptFile"]; got != promptPath {
			t.Fatalf("authored %s %q promptFile = %q, want %q", collection, name, got, promptPath)
		}
		if _, hasBody := subject["body"]; hasBody {
			t.Fatalf("authored %s %q must not inline prompt body", collection, name)
		}
		return
	}
	t.Fatalf("authored %s missing %q", collection, name)
}
