package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestCanonicalServerMCPContractIsAuthoritative(t *testing.T) {
	document := readJSON(t, filepath.Join("cli", "commands.json"))
	commands := document.(map[string]any)["commands"].(map[string]any)

	parent := requireCLICommand(t, commands, "you.server")
	if runnable, _ := parent["runnable"].(bool); !runnable {
		t.Fatal("you.server must remain the runnable HTTP/dashboard command")
	}
	serve := requireCLICommand(t, commands, "you.server.mcp")
	if got := serve["completeness"]; got != "authoritative" {
		t.Fatalf("you.server.mcp completeness = %v, want authoritative", got)
	}
	if got := serve["path"]; got != "you server mcp" {
		t.Fatalf("you.server.mcp path = %v, want you server mcp", got)
	}
	if got := serve["handler"].(map[string]any)["id"]; got != "you.server.mcp.handler" {
		t.Fatalf("you.server.mcp handler id = %v, want you.server.mcp.handler", got)
	}

	flags := serve["flags"].(map[string]any)
	for _, flagID := range []string{
		"you.server.mcp.flag.fixture-catalog",
		"you.server.mcp.flag.runtime",
		"you.server.mcp.flag.project-root",
	} {
		if _, ok := flags[flagID]; !ok {
			t.Fatalf("you.server.mcp missing %s", flagID)
		}
	}
	relationships := serve["relationships"].(map[string]any)
	mutex := relationships["you.server.mcp.relationship.runtime-source"].(map[string]any)
	if got := mutex["kind"]; got != "mutually-exclusive" {
		t.Fatalf("runtime source relationship kind = %v, want mutually-exclusive", got)
	}
	channels := serve["channels"].(map[string]any)
	if got := stringSlice(channels["input"]); !slices.Contains(got, "stdin") {
		t.Fatalf("you.server.mcp input channels = %v, want stdin", got)
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
