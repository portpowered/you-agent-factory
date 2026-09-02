package definitions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	authoredLayoutFactoryName     = "authored-layout-round-trip"
	authoredLayoutWorkerName      = "executor"
	authoredLayoutWorkstationName = "execute-task"
	authoredLayoutWorkTypeName    = "task"
)

// TestFactoryConfigFlattenExpandRoundTripsThroughRootProcess proves the
// customer-facing flatten and expand commands preserve a representative
// Factory definition when they are executed through the reusable root process.
func TestFactoryConfigFlattenExpandRoundTripsThroughRootProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, authoredLayoutFactoryJSON(), 0o644); err != nil {
		t.Fatalf("write authored Factory: %v", err)
	}

	process := buildDefinitionsProcess(t)

	beforePayload, err := support.FlattenFactoryConfigWithProcessAndEnv(
		t, process, isolatedHomeEnvironment(t), dir,
	)
	if err != nil {
		t.Fatalf("Process.Execute(factory config flatten before expand): %v", err)
	}
	before, err := support.DecodeFactoryDefinition(beforePayload)
	if err != nil {
		t.Fatalf("decode Factory before expand: %v", err)
	}

	expandInputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "expand", factoryPath,
	})
	expandInputs.Input.Env = isolatedHomeEnvironment(t)
	expandInputs.Input.WorkingDirectory = dir
	if err := process.Execute(expandInputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory config expand): %v\nstdout:\n%s\nstderr:\n%s",
			err,
			expandInputs.Stdout(),
			expandInputs.Stderr(),
		)
	}
	if !strings.Contains(expandInputs.Stdout(), "Expanded factory config into") {
		t.Fatalf("expand output = %q, want success marker", expandInputs.Stdout())
	}

	assertAuthoredLayoutFilesMaterialized(t, dir)
	afterPayload, err := support.FlattenFactoryConfigWithProcessAndEnv(
		t, process, isolatedHomeEnvironment(t), dir,
	)
	if err != nil {
		t.Fatalf("Process.Execute(factory config flatten after expand): %v", err)
	}
	after, err := support.DecodeFactoryDefinition(afterPayload)
	if err != nil {
		t.Fatalf("decode Factory after expand: %v", err)
	}

	assertAuthoredLayoutFactoryPreserved(t, before, after)
}

func assertAuthoredLayoutFilesMaterialized(t *testing.T, dir string) {
	t.Helper()

	for _, relativePath := range []string{
		filepath.Join("workers", authoredLayoutWorkerName, "AGENTS.md"),
		filepath.Join("workstations", authoredLayoutWorkstationName, "AGENTS.md"),
	} {
		path := filepath.Join(dir, relativePath)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read expanded authored file %s: %v", relativePath, err)
		}
		if strings.TrimSpace(string(content)) == "" {
			t.Fatalf("expanded authored file %s is empty", relativePath)
		}
	}
}

func assertAuthoredLayoutFactoryPreserved(
	t *testing.T,
	before factoryapi.Factory,
	after factoryapi.Factory,
) {
	t.Helper()

	if after.Name != before.Name || after.Name != authoredLayoutFactoryName {
		t.Fatalf("Factory name after round trip = %q, want %q", after.Name, before.Name)
	}

	beforeWorkType, ok := authoredLayoutWorkType(before)
	if !ok {
		t.Fatalf("Factory before round trip missing work type %q", authoredLayoutWorkTypeName)
	}
	afterWorkType, ok := authoredLayoutWorkType(after)
	if !ok {
		t.Fatalf("Factory after round trip missing work type %q", authoredLayoutWorkTypeName)
	}
	if len(afterWorkType.States) != len(beforeWorkType.States) {
		t.Fatalf("work type state count after round trip = %d, want %d", len(afterWorkType.States), len(beforeWorkType.States))
	}
	for index := range beforeWorkType.States {
		beforeState := beforeWorkType.States[index]
		afterState := afterWorkType.States[index]
		if afterState.Name != beforeState.Name || afterState.Type != beforeState.Type {
			t.Fatalf("work state %d after round trip = %#v, want %#v", index, afterState, beforeState)
		}
	}

	beforeWorker, ok := support.FindFactoryWorker(before, authoredLayoutWorkerName)
	if !ok {
		t.Fatalf("Factory before round trip missing worker %q", authoredLayoutWorkerName)
	}
	afterWorker, ok := support.FindFactoryWorker(after, authoredLayoutWorkerName)
	if !ok {
		t.Fatalf("Factory after round trip missing worker %q", authoredLayoutWorkerName)
	}
	if stringValue(afterWorker.Body) != stringValue(beforeWorker.Body) {
		t.Fatalf("worker body after round trip = %q, want %q", stringValue(afterWorker.Body), stringValue(beforeWorker.Body))
	}

	beforeWorkstation, ok := support.FindFactoryWorkstation(before, authoredLayoutWorkstationName)
	if !ok {
		t.Fatalf("Factory before round trip missing workstation %q", authoredLayoutWorkstationName)
	}
	afterWorkstation, ok := support.FindFactoryWorkstation(after, authoredLayoutWorkstationName)
	if !ok {
		t.Fatalf("Factory after round trip missing workstation %q", authoredLayoutWorkstationName)
	}
	if stringValue(afterWorkstation.Worker) != stringValue(beforeWorkstation.Worker) {
		t.Fatalf("workstation worker after round trip = %q, want %q", stringValue(afterWorkstation.Worker), stringValue(beforeWorkstation.Worker))
	}
	assertWorkstationReferencesPreserved(t, beforeWorkstation, afterWorkstation)
}

func assertWorkstationReferencesPreserved(
	t *testing.T,
	before factoryapi.Workstation,
	after factoryapi.Workstation,
) {
	t.Helper()

	if len(after.Inputs) != len(before.Inputs) {
		t.Fatalf("workstation input count after round trip = %d, want %d", len(after.Inputs), len(before.Inputs))
	}
	for index := range before.Inputs {
		assertWorkstationIOPreserved(t, "input", index, before.Inputs[index], after.Inputs[index])
	}
	assertOptionalWorkstationReferences(t, "outputs", before.Outputs, after.Outputs)
	assertOptionalWorkstationReferences(t, "onFailure", before.OnFailure, after.OnFailure)
}

func assertOptionalWorkstationReferences(
	t *testing.T,
	label string,
	before *[]factoryapi.WorkstationIO,
	after *[]factoryapi.WorkstationIO,
) {
	t.Helper()

	if before == nil || after == nil {
		if before != nil || after != nil {
			t.Fatalf("workstation %s after round trip = %#v, want %#v", label, after, before)
		}
		return
	}
	if len(*after) != len(*before) {
		t.Fatalf("workstation %s count after round trip = %d, want %d", label, len(*after), len(*before))
	}
	for index := range *before {
		assertWorkstationIOPreserved(t, label, index, (*before)[index], (*after)[index])
	}
}

func assertWorkstationIOPreserved(
	t *testing.T,
	label string,
	index int,
	before factoryapi.WorkstationIO,
	after factoryapi.WorkstationIO,
) {
	t.Helper()

	if after.WorkType != before.WorkType || after.State != before.State {
		t.Fatalf("workstation %s %d after round trip = %#v, want workType=%q state=%q", label, index, after, before.WorkType, before.State)
	}
}

func authoredLayoutWorkType(factory factoryapi.Factory) (factoryapi.WorkType, bool) {
	if factory.WorkTypes == nil {
		return factoryapi.WorkType{}, false
	}
	for _, workType := range *factory.WorkTypes {
		if workType.Name == authoredLayoutWorkTypeName {
			return workType, true
		}
	}
	return factoryapi.WorkType{}, false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func authoredLayoutFactoryJSON() []byte {
	return []byte(`{
  "name": "` + authoredLayoutFactoryName + `",
  "workTypes": [{
    "name": "` + authoredLayoutWorkTypeName + `",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "` + authoredLayoutWorkerName + `",
    "type": "MODEL_WORKER",
    "model": "claude-sonnet-4-20250514",
    "modelProvider": "CLAUDE",
    "stopToken": "COMPLETE",
    "body": "You are the authored-layout executor."
  }],
  "workstations": [{
    "id": "execute-task-id",
    "name": "` + authoredLayoutWorkstationName + `",
    "behavior": "STANDARD",
    "worker": "` + authoredLayoutWorkerName + `",
    "inputs": [{"workType": "` + authoredLayoutWorkTypeName + `", "state": "init"}],
    "outputs": [{"workType": "` + authoredLayoutWorkTypeName + `", "state": "complete"}],
    "onFailure": [{"workType": "` + authoredLayoutWorkTypeName + `", "state": "failed"}],
    "definition": {
      "type": "MODEL_WORKSTATION",
      "worker": "` + authoredLayoutWorkerName + `",
      "body": "Complete {{ (index .Inputs 0).WorkID }}.",
      "stopWords": ["DONE"]
    }
  }]
}`)
}
