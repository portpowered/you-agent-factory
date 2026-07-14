package builtinsubagent_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/subagent"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInSubagentFactoryJSON_AssemblesFromAuthoredPromptFiles(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinsubagent.BuiltInSubagentFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	assertWorkerBodiesMatchAuthoredPrompts(t, cfg)
	assertWorkstationBodiesMatchAuthoredPrompts(t, cfg)
	assertFactoryJSONWorkstationsHaveNoInlineBodies(t)
}

func assertWorkstationBodiesMatchAuthoredPrompts(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	for _, workstation := range cfg.Workstations {
		if workstation.Name != "run-subagent" {
			continue
		}
		body := strings.TrimSpace(workstation.Body)
		if !strings.Contains(body, "${input}") {
			t.Fatalf("run-subagent body = %q, want invocation request interpolation", body)
		}
		if !strings.Contains(body, "(index .Inputs 0).WorkID") {
			t.Fatalf("run-subagent body = %q, want canonical work input binding", body)
		}
		return
	}
	t.Fatal("run-subagent workstation not found")
}

func assertWorkerBodiesMatchAuthoredPrompts(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	for _, worker := range cfg.Workers {
		if worker.Name != "subagent-worker" {
			continue
		}
		if strings.TrimSpace(worker.Body) == "" {
			t.Fatal("subagent-worker body is empty")
		}
		if worker.AgentTools == nil || worker.AgentTools.Policy != interfaces.AgentWorkerToolPolicyReadOnly {
			t.Fatalf("subagent-worker agentTools = %#v, want READ_ONLY policy", worker.AgentTools)
		}
		return
	}
	t.Fatal("subagent-worker not found")
}

func assertFactoryJSONWorkstationsHaveNoInlineBodies(t *testing.T) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(builtinsubagent.FactoryJSON(), &raw); err != nil {
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
