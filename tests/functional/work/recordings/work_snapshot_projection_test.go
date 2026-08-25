package workrecordings_test

import (
	"context"
	"encoding/json"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestRecordingsWorkSnapshotReaderPreservesTheProducingCursor proves the
// Recordings-owned projection emits the Work state and the canonical cursor
// that the Work read path uses for its durability confirmation decision.
func TestRecordingsWorkSnapshotReaderPreservesTheProducingCursor(t *testing.T) {
	t.Parallel()

	const (
		sessionID  = "session-recordings-projection"
		generation = "generation-recordings-projection"
		workID     = "work-story"
		workTypeID = "story"
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	contextPayload := `{"workIds":["` + workID + `"]}`
	events := []recordings.CanonicalEvent{
		functionalWorkSnapshotEvent(
			"work-request",
			1,
			scope,
			generation,
			recordings.CanonicalEventKind(recordings.FactoryEventTypeWorkRequest),
			`{"works":[{"name":"Review PRD","workId":"`+workID+`"}]}`,
			contextPayload,
		),
		functionalWorkSnapshotEvent(
			"work-state-change",
			8,
			scope,
			generation,
			recordings.CanonicalEventKind(recordings.FactoryEventTypeWorkStateChange),
			`{"workId":"`+workID+`","toState":"review"}`,
			contextPayload,
		),
		functionalWorkSnapshotEvent(
			"dispatch-response",
			9,
			scope,
			generation,
			recordings.CanonicalEventKind(recordings.FactoryEventTypeDispatchResponse),
			`{"output_work":[{"id":"`+workID+`"}]}`,
			contextPayload,
		),
	}
	root := &functionalWorkSnapshotRoot{
		events: events,
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         scope,
			Through: recordings.CanonicalEventCursor{
				StreamGenerationID: generation,
				Sequence:           9,
			},
			Payload: functionalWorkSnapshotWorldPayload(t, workID, workTypeID),
		},
	}

	snapshot, err := recordingswire.NewWorkSnapshotReader(root).ReadWorkSnapshot(
		context.Background(),
		sessionID,
	)
	if err != nil {
		t.Fatalf("ReadWorkSnapshot: %v", err)
	}
	if snapshot.StreamGenerationID != generation {
		t.Fatalf("stream generation = %q, want %q", snapshot.StreamGenerationID, generation)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("snapshot items = %#v, want one Work item", snapshot.Items)
	}
	item := snapshot.Items[0]
	if item.WorkID != workID || item.State == nil || item.State.Name != "review" {
		t.Fatalf("snapshot item = %#v, want %s in review", item, workID)
	}
	if !item.CurrentStateSequenceKnown || item.CurrentStateSequence != 9 {
		t.Fatalf("current state cursor = known:%t sequence:%d, want known sequence 9", item.CurrentStateSequenceKnown, item.CurrentStateSequence)
	}
	if len(root.reconstructRequests) != 1 || len(root.reconstructRequests[0].Events) != len(events) {
		t.Fatalf("reconstruct requests = %#v, want one request with all canonical events", root.reconstructRequests)
	}
}

func functionalWorkSnapshotEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	generation string,
	kind recordings.CanonicalEventKind,
	payload string,
	sourceContext string,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:            recordings.CanonicalEventID(id),
		Sequence:      sequence,
		Scope:         scope,
		Cursor:        recordings.CanonicalEventCursor{StreamGenerationID: generation, Sequence: sequence},
		Kind:          kind,
		Payload:       payload,
		SourceContext: sourceContext,
	}
}

type functionalWorkSnapshotRoot struct {
	events              []recordings.CanonicalEvent
	worldState          recordings.WorldStateView
	reconstructRequests []recordings.ReconstructWorldStateRequest
}

func (root *functionalWorkSnapshotRoot) SubscribeFrom(
	_ context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	events := make([]recordings.CanonicalEvent, 0, len(root.events))
	for _, event := range root.events {
		if event.Scope == request.Scope {
			events = append(events, event)
		}
	}
	index := 0
	return recordings.SubscribeResult{
		Subscription: func(context.Context) recordings.SubscriptionOutcome {
			if index >= len(events) {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			event := events[index]
			index++
			return recordings.SubscriptionOutcome{
				Kind:  recordings.SubscriptionEvent,
				Event: event,
			}
		},
	}, nil
}

func (root *functionalWorkSnapshotRoot) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	root.reconstructRequests = append(root.reconstructRequests, request)
	return recordings.ReconstructWorldStateResult{WorldState: root.worldState}, nil
}

func functionalWorkSnapshotWorldPayload(t *testing.T, workID, workTypeID string) string {
	t.Helper()
	state := factorydefinitions.FactoryWorldState{
		Topology: factorydefinitions.InitialStructurePayload{
			WorkTypes: []factorydefinitions.FactoryWorkType{{
				ID: workTypeID,
				States: []factorydefinitions.FactoryStateDefinition{
					{Value: "init", Category: work.StateTypeInitial},
					{Value: "review", Category: work.StateTypeProcessing},
				},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			workID: {
				ID:          workID,
				WorkTypeID:  workTypeID,
				DisplayName: "Review PRD",
				State:       "review",
			},
		},
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal Work snapshot world state: %v", err)
	}
	return string(payload)
}
