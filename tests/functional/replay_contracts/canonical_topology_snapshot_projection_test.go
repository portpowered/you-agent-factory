package replay_contracts

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/replayfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence(t *testing.T) {
	events, err := replayfixtures.CanonicalTopologyReplacementEvents()
	if err != nil {
		t.Fatalf("CanonicalTopologyReplacementEvents: %v", err)
	}
	if len(events) != 2 || events[0].Type != interfaces.FactoryEventTypeInitialStructureRequest || events[1].Type != interfaces.FactoryEventTypeFactoryChange {
		t.Fatalf("events = %#v, want initial and replacement topology events", events)
	}

	for _, event := range events {
		var snapshot *interfaces.FactorySnapshot
		if event.Type == interfaces.FactoryEventTypeFactoryChange {
			var payload interfaces.FactoryChangeEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode replacement payload: %v", err)
			}
			snapshot = payload.Factory
		} else {
			var payload interfaces.InitialStructureRequestEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode initial payload: %v", err)
			}
			snapshot = payload.Factory
		}
		assertCanonicalTopologySnapshotEvidence(t, snapshot)
	}
}

func assertCanonicalTopologySnapshotEvidence(t *testing.T, snapshot *interfaces.FactorySnapshot) {
	t.Helper()
	var factory struct {
		Resources []struct {
			ID string `json:"id"`
		} `json:"resources"`
		Workers []struct {
			ID        string `json:"id"`
			Resources []struct {
				Name string `json:"name"`
			} `json:"resources"`
		} `json:"workers"`
		WorkTypes []struct {
			ID     string `json:"id"`
			States []struct {
				ID string `json:"id"`
			} `json:"states"`
		} `json:"workTypes"`
		Workstations []struct {
			ID        string `json:"id"`
			Resources []struct {
				Name string `json:"name"`
			} `json:"resources"`
		} `json:"workstations"`
	}
	if snapshot == nil {
		t.Fatal("Factory snapshot is nil")
	}
	if err := snapshot.Decode(&factory); err != nil {
		t.Fatalf("decode Factory snapshot: %v", err)
	}
	if len(factory.Resources) != 1 || factory.Resources[0].ID != "gpu-stable" ||
		len(factory.Workers) != 1 || factory.Workers[0].ID != "writer-stable" || len(factory.Workers[0].Resources) != 1 ||
		len(factory.WorkTypes) != 1 || factory.WorkTypes[0].ID != "task-stable" || len(factory.WorkTypes[0].States) != 3 || factory.WorkTypes[0].States[0].ID != "queued-stable" ||
		len(factory.Workstations) != 1 || factory.Workstations[0].ID != "review-stable" || len(factory.Workstations[0].Resources) != 1 {
		t.Fatalf("Factory snapshot lost durable topology evidence: %#v", factory)
	}
}
