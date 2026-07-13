package builtingoal_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/builtingoal"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var workstationRoleByName = map[string]string{
	"execute-goal": "executor",
}

func TestBuiltInGoalFactoryJSON_AssemblesFromAuthoredPromptFiles(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	assertAuthoredRolePromptsNonEmpty(t)
	assertWorkerBodiesMatchAuthoredPrompts(t, cfg)
	assertWorkersWithoutAuthoredPromptsHaveEmptyBodies(t, cfg)
	assertWorkstationBodiesMatchAuthoredPrompts(t, cfg)
	assertFactoryJSONWorkersHaveNoInlineBodies(t)
	assertFactoryJSONWorkstationsHaveNoInlineBodies(t)
	assertFactoryJSONDoesNotEmbedAuthoredPromptContent(t)
}

func assertAuthoredRolePromptsNonEmpty(t *testing.T) {
	t.Helper()
	for role, authoredPrompt := range builtingoal.AuthoredRolePrompts {
		if strings.TrimSpace(authoredPrompt) == "" {
			t.Fatalf("authored prompt for role %q is empty", role)
		}
	}
}

func assertWorkstationBodiesMatchAuthoredPrompts(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	workstationBodies := map[string]string{}
	for _, workstation := range cfg.Workstations {
		workstationBodies[workstation.Name] = strings.TrimSpace(workstation.Body)
	}
	for workstationName, role := range workstationRoleByName {
		want := strings.TrimSpace(builtingoal.AuthoredRolePrompts[role])
		if got := workstationBodies[workstationName]; got != want {
			t.Fatalf("%s body does not match authored %s prompt", workstationName, role)
		}
	}
}

func assertWorkerBodiesMatchAuthoredPrompts(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	workerBodies := map[string]string{}
	for _, worker := range cfg.Workers {
		workerBodies[worker.Name] = strings.TrimSpace(worker.Body)
	}
	want := strings.TrimSpace(builtingoal.AuthoredRolePrompts["executor"])
	if got := workerBodies["goal-executor"]; got != want {
		t.Fatalf("goal-executor body does not match authored executor prompt")
	}
}

func assertWorkersWithoutAuthoredPromptsHaveEmptyBodies(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	authoredWorkers := map[string]struct{}{"goal-executor": {}}
	for _, worker := range cfg.Workers {
		if _, ok := authoredWorkers[worker.Name]; ok {
			continue
		}
		if strings.TrimSpace(worker.Body) != "" {
			t.Fatalf("worker %q body = %q, want empty body derived only from workstation prompt files", worker.Name, worker.Body)
		}
	}
}

func assertFactoryJSONWorkersHaveNoInlineBodies(t *testing.T) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(builtingoal.FactoryJSON(), &raw); err != nil {
		t.Fatalf("unmarshal authored factory.json: %v", err)
	}
	workers, ok := raw["workers"].([]any)
	if !ok {
		t.Fatal("authored factory.json workers must be an array")
	}
	for _, entry := range workers {
		worker, ok := entry.(map[string]any)
		if !ok {
			t.Fatal("authored worker entry must be an object")
		}
		if _, hasBody := worker["body"]; hasBody {
			t.Fatalf("authored worker %q must not inline prompt body in factory.json", worker["name"])
		}
	}
}

func assertFactoryJSONDoesNotEmbedAuthoredPromptContent(t *testing.T) {
	t.Helper()
	raw := string(builtingoal.FactoryJSON())
	for _, marker := range []string{"bounded plan", "bounded execution result", "reviewable disposition", "bounded final summary"} {
		if strings.Contains(raw, marker) {
			t.Fatalf("authored factory.json must not embed prompt content %q; edit pkg/config/builtingoal/prompts instead", marker)
		}
	}
}

func assertFactoryJSONWorkstationsHaveNoInlineBodies(t *testing.T) {
	t.Helper()
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
