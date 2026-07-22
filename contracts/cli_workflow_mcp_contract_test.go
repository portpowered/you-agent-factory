package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestCanonicalMCPServeContractIsAuthoritative(t *testing.T) {
	document := readJSON(t, filepath.Join("cli", "commands.json"))
	commands := document.(map[string]any)["commands"].(map[string]any)

	parent := requireCLICommand(t, commands, "you.mcp")
	if runnable, _ := parent["runnable"].(bool); runnable {
		t.Fatal("you.mcp must remain a non-runnable command group")
	}
	serve := requireCLICommand(t, commands, "you.mcp.serve")
	if got := serve["completeness"]; got != "authoritative" {
		t.Fatalf("you.mcp.serve completeness = %v, want authoritative", got)
	}
	if got := serve["path"]; got != "you mcp serve" {
		t.Fatalf("you.mcp.serve path = %v, want you mcp serve", got)
	}
	if got := serve["handler"].(map[string]any)["id"]; got != "you.mcp.serve.handler" {
		t.Fatalf("you.mcp.serve handler id = %v, want you.mcp.serve.handler", got)
	}

	flags := serve["flags"].(map[string]any)
	for _, flagID := range []string{
		"you.mcp.serve.flag.fixture-catalog",
		"you.mcp.serve.flag.runtime",
		"you.mcp.serve.flag.project-root",
	} {
		if _, ok := flags[flagID]; !ok {
			t.Fatalf("you.mcp.serve missing %s", flagID)
		}
	}
	relationships := serve["relationships"].(map[string]any)
	mutex := relationships["you.mcp.serve.relationship.runtime-source"].(map[string]any)
	if got := mutex["kind"]; got != "mutually-exclusive" {
		t.Fatalf("runtime source relationship kind = %v, want mutually-exclusive", got)
	}
	channels := serve["channels"].(map[string]any)
	if got := stringSlice(channels["input"]); !slices.Contains(got, "stdin") {
		t.Fatalf("you.mcp.serve input channels = %v, want stdin", got)
	}
}

func TestWorkflowCompatibilityContractsAreRemoved(t *testing.T) {
	compatibility := readJSON(t, filepath.Join("cli", "deprecated-commands.json"))
	compatibilityCommands := compatibility.(map[string]any)["commands"].(map[string]any)
	if len(compatibilityCommands) != 0 {
		t.Fatalf("workflow compatibility commands = %#v, want none", compatibilityCommands)
	}

	primary := readJSON(t, filepath.Join("cli", "commands.json"))
	primaryCommands := primary.(map[string]any)["commands"].(map[string]any)
	for commandID := range primaryCommands {
		if len(commandID) > len("you.workflow.") && commandID[:len("you.workflow.")] == "you.workflow." {
			t.Fatalf("primary manifest still contains removed workflow command %s", commandID)
		}
	}
}

func requireCLICommand(t *testing.T, commands map[string]any, id string) map[string]any {
	t.Helper()
	record, ok := commands[id].(map[string]any)
	if !ok {
		t.Fatalf("manifest missing %s", id)
	}
	return record
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.(string))
	}
	return result
}
