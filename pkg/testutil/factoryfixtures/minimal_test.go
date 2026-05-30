package factoryfixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestMinimalFactoryConfig_WriteReadSmoke(t *testing.T) {
	dir := t.TempDir()
	WriteFactoryJSON(t, dir, MinimalFactoryConfig())
	assertWrittenFactoryJSONSmoke(t, dir)
}

func assertWrittenFactoryJSONSmoke(t *testing.T, dir string) {
	t.Helper()
	got := readFactoryJSONMap(t, dir)
	assertFactoryJSONTopLevelKeys(t, got)
	assertFactoryJSONName(t, got, "factory")
	assertFactoryJSONTaskInitState(t, got)
	assertFactoryJSONProcessInputWiring(t, got)
}

func readFactoryJSONMap(t *testing.T, dir string) map[string]any {
	t.Helper()
	path := filepath.Join(dir, interfaces.FactoryConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}
	return got
}

func assertFactoryJSONTopLevelKeys(t *testing.T, got map[string]any) {
	t.Helper()
	for _, key := range []string{"name", "workTypes", "workers", "workstations"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("factory.json missing top-level key %q", key)
		}
	}
}

func assertFactoryJSONName(t *testing.T, got map[string]any, want string) {
	t.Helper()
	if got["name"] != want {
		t.Fatalf("factory name = %v, want %s", got["name"], want)
	}
}

func assertFactoryJSONTaskInitState(t *testing.T, got map[string]any) {
	t.Helper()
	task := soleMapEntryNamed(t, got["workTypes"], "workTypes", "task")
	states, ok := task["states"].([]any)
	if !ok {
		t.Fatalf("task states = %#v, want array", task["states"])
	}
	if !hasStateNamed(states, "init", "INITIAL") {
		t.Fatalf("task states = %#v, want init INITIAL state", states)
	}
}

func assertFactoryJSONProcessInputWiring(t *testing.T, got map[string]any) {
	t.Helper()
	process := soleMapEntryNamed(t, got["workstations"], "workstations", "process")
	inputs, ok := process["inputs"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("process inputs = %#v, want task init wiring", process["inputs"])
	}
	input, ok := inputs[0].(map[string]any)
	if !ok || input["workType"] != "task" || input["state"] != "init" {
		t.Fatalf("process input = %#v, want task init", inputs[0])
	}
}

func soleMapEntryNamed(t *testing.T, entries any, label, name string) map[string]any {
	t.Helper()
	list, ok := entries.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("%s = %#v, want one entry", label, entries)
	}
	entry, ok := list[0].(map[string]any)
	if !ok || entry["name"] != name {
		t.Fatalf("%s entry = %#v, want name %s", label, list[0], name)
	}
	return entry
}

func hasStateNamed(states []any, name, stateType string) bool {
	for _, state := range states {
		entry, ok := state.(map[string]any)
		if !ok {
			continue
		}
		if entry["name"] == name && entry["type"] == stateType {
			return true
		}
	}
	return false
}
