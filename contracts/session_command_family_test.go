package contracts_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"
)

func TestProductionSessionManifestContractsCanonicalFamily(t *testing.T) {
	instance := readJSON(t, filepath.Join("cli", "commands.json"))
	commands := requireObject(t, instance, "commands")

	wantIDs := []string{
		"you.session",
		"you.session.create",
		"you.session.delete",
		"you.session.dispatches",
		"you.session.list",
		"you.session.pause",
		"you.session.resume",
		"you.session.show",
	}
	gotIDs := make([]string, 0, len(wantIDs))
	for id := range commands {
		if id == "you.session" || len(id) > len("you.session.") && id[:len("you.session.")] == "you.session." {
			gotIDs = append(gotIDs, id)
		}
	}
	sort.Strings(gotIDs)
	if !equalStrings(gotIDs, wantIDs) {
		t.Fatalf("session command IDs = %v, want %v", gotIDs, wantIDs)
	}

	parent := requireObject(t, commands, "you.session")
	if runnable, _ := parent["runnable"].(bool); runnable {
		t.Fatal("you.session must remain a non-runnable parent")
	}

	leaves := []struct {
		id          string
		operationID string
		argRequired bool
		portMode    bool
	}{
		{id: "you.session.create", operationID: "openFactorySession", portMode: true},
		{id: "you.session.delete", operationID: "closeFactorySession", argRequired: true, portMode: true},
		{id: "you.session.dispatches", operationID: "listFactorySessionDispatches", argRequired: true},
		{id: "you.session.list", operationID: "listFactorySessions", portMode: true},
		{id: "you.session.pause", operationID: "pauseFactorySession"},
		{id: "you.session.resume", operationID: "resumeFactorySession"},
		{id: "you.session.show", operationID: "getFactorySession"},
	}
	for _, leaf := range leaves {
		t.Run(leaf.id, func(t *testing.T) {
			record := requireObject(t, commands, leaf.id)
			if runnable, _ := record["runnable"].(bool); !runnable {
				t.Fatalf("%s must be runnable", leaf.id)
			}
			if path, _ := record["path"].(string); path != "you session "+record["name"].(string) {
				t.Fatalf("%s path = %q", leaf.id, path)
			}
			handler := requireObject(t, record, "handler")
			if got := handler["id"]; got != leaf.id+".handler" {
				t.Fatalf("%s handler id = %v", leaf.id, got)
			}
			if got := handler["operationId"]; got != leaf.operationID {
				t.Fatalf("%s operationId = %v, want %s", leaf.id, got, leaf.operationID)
			}
			assertCompleteSessionExecutionMetadata(t, leaf.id, record)
			assertSessionPortMode(t, leaf.id, record, leaf.portMode)
			assertSessionArgument(t, leaf.id, record, leaf.argRequired)
		})
	}
}

func TestProductionSessionCreateContractsRelationshipsAndDefaults(t *testing.T) {
	instance := readJSON(t, filepath.Join("cli", "commands.json"))
	commands := requireObject(t, instance, "commands")
	create := requireObject(t, commands, "you.session.create")
	flags := requireObject(t, create, "flags")

	dir := requireObject(t, flags, "you.session.create.flag.dir")
	if required, _ := dir["required"].(bool); !required {
		t.Fatal("you.session.create --dir must be required")
	}
	for _, flag := range []string{"init-new-factory", "validate-only", "target-kind", "target-name"} {
		requireObject(t, flags, "you.session.create.flag."+flag)
	}

	relationships := requireObject(t, create, "relationships")
	mutex := requireObject(t, relationships, "you.session.create.rel.mutex.init-new-factory-validate-only")
	if got := mutex["kind"]; got != "mutually-exclusive" {
		t.Fatalf("create relationship kind = %v", got)
	}
	participants, ok := mutex["participants"].([]any)
	if !ok || len(participants) != 2 {
		t.Fatalf("create mutex participants = %#v, want two", mutex["participants"])
	}

	list := requireObject(t, commands, "you.session.list")
	listFlags := requireObject(t, list, "flags")
	scope := requireObject(t, listFlags, "you.session.list.flag.scope")
	if got := scope["default"]; got != "live" {
		t.Fatalf("you.session.list --scope default = %v, want live", got)
	}
}

func assertCompleteSessionExecutionMetadata(t *testing.T, id string, record map[string]any) {
	t.Helper()
	for _, field := range []string{"documentation", "usage", "flags", "precedence", "channels", "outputs", "exits", "sideEffects", "constraints"} {
		if _, ok := record[field]; !ok {
			t.Fatalf("%s missing %s", id, field)
		}
	}
	channels := requireObject(t, record, "channels")
	if len(channels["output"].([]any)) != 2 {
		t.Fatalf("%s output channels = %v, want stdout and stderr", id, channels["output"])
	}
	exits := requireObject(t, record, "exits")
	for _, kind := range []string{"success", "usage", "failure"} {
		requireObject(t, exits, id+".exit."+kind)
	}
}

func assertSessionPortMode(t *testing.T, id string, record map[string]any, portMode bool) {
	t.Helper()
	flags := requireObject(t, record, "flags")
	port := requireObject(t, flags, id+".flag.port")
	wantVisibility, wantDefault := "hidden", "0"
	if portMode {
		wantVisibility, wantDefault = "visible", "7437"
	}
	if got := port["visibility"]; got != wantVisibility {
		t.Fatalf("%s --port visibility = %v, want %s", id, got, wantVisibility)
	}
	if got := port["default"]; got != wantDefault {
		t.Fatalf("%s --port default = %v, want %s", id, got, wantDefault)
	}
	server := requireObject(t, flags, id+".flag.server")
	if got := server["scope"]; got != "inherited" {
		t.Fatalf("%s --server scope = %v, want inherited", id, got)
	}
}

func assertSessionArgument(t *testing.T, id string, record map[string]any, required bool) {
	t.Helper()
	arguments := requireObject(t, record, "arguments")
	argument := requireObject(t, arguments, id+".arg.0")
	if got, _ := argument["required"].(bool); got != required {
		t.Fatalf("%s argument required = %t, want %t", id, got, required)
	}
	if id == "you.session.create" || id == "you.session.list" {
		if variadic, _ := argument["variadic"].(bool); !variadic || fmt.Sprint(argument["maxCardinality"]) != "-1" {
			t.Fatalf("%s must preserve legacy unbounded positional handling", id)
		}
	}
}

func requireObject(t *testing.T, value any, key string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %T, want object containing %s", value, key)
	}
	nested, ok := object[key].(map[string]any)
	if !ok {
		t.Fatalf("missing object %s", key)
	}
	return nested
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
