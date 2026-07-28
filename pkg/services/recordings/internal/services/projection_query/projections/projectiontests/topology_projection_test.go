package projections_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/projections"
)

func TestReconstructCanonicalFactoryWorldState_DecodesTopologyFromFactorySnapshot(t *testing.T) {
	t0 := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name":      "canonical-factory",
		"resources": []any{map[string]any{"name": "agent-slot", "capacity": 2}},
		"workers": []any{map[string]any{
			"name": "reviewer", "type": "MODEL", "executorProvider": "codex-cli",
		}},
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "done", "type": "TERMINAL"},
			},
		}},
		"workstations": []any{map[string]any{
			"id": "review", "name": "Review", "worker": "reviewer", "behavior": "STANDARD",
			"inputs":  []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs": []any{map[string]any{"workType": "task", "state": "done"}},
		}},
		"futureTopologyField": map[string]any{"preserved": true},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	payload, err := json.Marshal(interfaces.InitialStructureRequestEventPayload{Factory: snapshot})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	event := interfaces.FactoryEvent{
		Id: "canonical-initial", SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:    interfaces.FactoryEventTypeInitialStructureRequest,
		Context: interfaces.FactoryEventContext{Tick: 0, EventTime: t0}, Payload: payload,
	}

	worldState, err := ReconstructCanonicalFactoryWorldState([]interfaces.FactoryEvent{event}, 0)
	if err != nil {
		t.Fatalf("ReconstructCanonicalFactoryWorldState: %v", err)
	}
	if worldState.Topology.Name != "canonical-factory" || len(worldState.Topology.Workstations) != 1 {
		t.Fatalf("topology = %#v, want canonical Factory snapshot topology", worldState.Topology)
	}
	if got := worldState.PlaceOccupancyByID["agent-slot:available"].TokenCount; got != 2 {
		t.Fatalf("agent-slot occupancy = %d, want 2", got)
	}
	if worldState.Factory == nil || !json.Valid(*worldState.Factory) {
		t.Fatalf("Factory snapshot = %#v, want retained valid JSON", worldState.Factory)
	}
	var retained map[string]any
	if err := worldState.Factory.Decode(&retained); err != nil {
		t.Fatalf("decode retained Factory snapshot: %v", err)
	}
	if _, ok := retained["futureTopologyField"]; !ok {
		t.Fatalf("retained Factory snapshot = %#v, want unknown field preserved", retained)
	}
	if !reflect.DeepEqual(worldState.Topology.Workstations[0].OutputPlaceIDs, []string{"task:done"}) {
		t.Fatalf("output routes = %#v, want task:done", worldState.Topology.Workstations[0].OutputPlaceIDs)
	}
}
