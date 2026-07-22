package snapshot

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactorySnapshotFromInitialStructurePreservesCanonicalFactoryShape(t *testing.T) {
	physical := time.Date(2026, 7, 16, 8, 45, 0, 0, time.FixedZone("snapshot", 2*60*60))
	snapshot := FromInitialStructure(interfaces.InitialStructurePayload{
		Version:   &interfaces.FactoryVersion{Logical: 42, Physical: physical},
		Resources: []interfaces.FactoryResource{{ID: "gpu", Capacity: 2}},
		WorkTypes: []interfaces.FactoryWorkType{{
			ID:     "story",
			States: []interfaces.FactoryStateDefinition{{Value: "ready", Category: string(interfaces.StateTypeInitial)}},
		}},
		Workers: []interfaces.FactoryWorker{{
			ID:            "builder",
			Provider:      "codex-cli",
			ModelProvider: "openai",
			Config:        map[string]string{"type": interfaces.WorkerTypeModel},
		}},
		Places: []interfaces.FactoryPlace{{ID: "story:ready", TypeID: "story", State: "ready"}},
		Workstations: []interfaces.FactoryWorkstation{{
			ID:            "build",
			WorkerID:      "builder",
			Kind:          string(interfaces.WorkstationKindRepeater),
			InputPlaceIDs: []string{"story:ready"},
		}},
	})

	var document map[string]json.RawMessage
	if err := snapshot.Decode(&document); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := string(document["name"]); got != `"factory"` {
		t.Fatalf("name = %s, want default Factory name", got)
	}
	if got := string(document["version"]); got != `{"logical":"42","physical":"2026-07-16T06:45:00Z"}` {
		t.Fatalf("version = %s, want canonical timestamp and string logical counter", got)
	}
	assertInitialStructureSnapshotJSON(t, document["resources"], `[{"capacity":2,"name":"gpu"}]`)
	assertInitialStructureSnapshotJSON(t, document["workTypes"], `[{"name":"story","states":[{"name":"ready","type":"INITIAL"}]}]`)
	assertInitialStructureSnapshotJSON(t, document["workers"], `[{"executorProvider":"SCRIPT_WRAP","modelProvider":"CODEX","name":"builder","type":"INFERENCE_WORKER"}]`)
	assertInitialStructureSnapshotJSON(t, document["workstations"], `[{"behavior":"REPEATER","id":"build","inputs":[{"state":"ready","workType":"story"}],"name":"build","worker":"builder"}]`)
}

func assertInitialStructureSnapshotJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	if string(got) != want {
		t.Fatalf("snapshot field = %s, want %s", got, want)
	}
}
