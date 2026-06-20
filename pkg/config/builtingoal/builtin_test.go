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
	"plan-goal":    "planner",
	"execute-goal": "executor",
	"check-goal":   "checker",
	"review-goal":  "reviewer",
}

func TestBuiltInGoalFactoryJSON_AssemblesFromAuthoredPromptFiles(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	assertAuthoredRolePromptsNonEmpty(t)
	assertWorkstationBodiesMatchAuthoredPrompts(t, cfg)
	assertFactoryJSONWorkstationsHaveNoInlineBodies(t)
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
