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
	cfg := MinimalFactoryConfig()
	WriteFactoryJSON(t, dir, cfg)

	path := filepath.Join(dir, interfaces.FactoryConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}
	for _, key := range []string{"name", "workTypes", "workers", "workstations"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("factory.json missing top-level key %q", key)
		}
	}
	if got["name"] != "factory" {
		t.Fatalf("factory name = %v, want factory", got["name"])
	}

	workTypes, ok := got["workTypes"].([]any)
	if !ok || len(workTypes) != 1 {
		t.Fatalf("workTypes = %#v, want one task work type", got["workTypes"])
	}
	task, ok := workTypes[0].(map[string]any)
	if !ok || task["name"] != "task" {
		t.Fatalf("task work type = %#v, want name task", workTypes[0])
	}
	states, ok := task["states"].([]any)
	if !ok {
		t.Fatalf("task states = %#v, want array", task["states"])
	}
	foundInit := false
	for _, state := range states {
		entry, ok := state.(map[string]any)
		if !ok {
			continue
		}
		if entry["name"] == "init" && entry["type"] == "INITIAL" {
			foundInit = true
			break
		}
	}
	if !foundInit {
		t.Fatalf("task states = %#v, want init INITIAL state", states)
	}

	workstations, ok := got["workstations"].([]any)
	if !ok || len(workstations) != 1 {
		t.Fatalf("workstations = %#v, want one process workstation", got["workstations"])
	}
	process, ok := workstations[0].(map[string]any)
	if !ok || process["name"] != "process" {
		t.Fatalf("process workstation = %#v, want name process", workstations[0])
	}
	inputs, ok := process["inputs"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("process inputs = %#v, want task init wiring", process["inputs"])
	}
	input, ok := inputs[0].(map[string]any)
	if !ok || input["workType"] != "task" || input["state"] != "init" {
		t.Fatalf("process input = %#v, want task init", inputs[0])
	}
}
