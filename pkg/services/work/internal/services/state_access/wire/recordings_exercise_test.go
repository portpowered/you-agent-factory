package wire_test

import (
	"context"
	"encoding/json"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

func TestGetWorkFromRecordingsRootUsesRecordingsServiceRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-unit"}
	fake := &recordingsRootFake{
		events: []recordings.CanonicalEvent{{
			ID:          "event-1",
			FactoryTick: 5,
			Scope:       scope,
		}},
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
		},
	}

	got, err := stateaccesswire.GetWorkFromRecordingsRoot(
		context.Background(),
		"session-recordings-unit",
		"work-story",
		fake,
	)
	if err != nil {
		t.Fatalf("GetWorkFromRecordingsRoot: %v", err)
	}
	if got.WorkID != "work-story" {
		t.Fatalf("GetWorkFromRecordingsRoot = %#v, want work-story", got)
	}
}

func TestListWorkFromRecordingsRootUsesRecordingsServiceRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-unit"}
	fake := &recordingsRootFake{
		events: []recordings.CanonicalEvent{{
			ID:          "event-1",
			FactoryTick: 2,
			Scope:       scope,
			Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
		}},
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
		},
	}

	list, err := stateaccesswire.ListWorkFromRecordingsRoot(
		context.Background(),
		"session-recordings-unit",
		fake,
		work.ListOptions{},
	)
	if err != nil {
		t.Fatalf("ListWorkFromRecordingsRoot: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWorkFromRecordingsRoot = %#v, want one story work item", list)
	}
}

type recordingsRootFake struct {
	events     []recordings.CanonicalEvent
	worldState recordings.WorldStateView
}

func (fake *recordingsRootFake) SubscribeFrom(
	_ context.Context,
	_ recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	index := 0
	return recordings.SubscribeResult{
		Subscription: func(context.Context) recordings.SubscriptionOutcome {
			if index >= len(fake.events) {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			outcome := recordings.SubscriptionOutcome{
				Kind:  recordings.SubscriptionEvent,
				Event: fake.events[index],
			}
			index++
			return outcome
		},
	}, nil
}
func (fake *recordingsRootFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return recordings.ReconstructWorldStateResult{WorldState: fake.worldState}, nil
}

func recordingsBackedWorldPayload(t *testing.T, workID, workTypeID, stateName string) string {
	t.Helper()
	state := interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				ID: workTypeID,
				States: []interfaces.FactoryStateDefinition{
					{Value: "review", Category: work.StateTypeProcessing},
				},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			workID: {ID: workID, WorkTypeID: workTypeID, State: stateName},
		},
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	return string(payload)
}
