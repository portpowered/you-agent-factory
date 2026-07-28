package wire

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewRecordingsAdapterConstructsReconstructWorldStateThroughRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings"}
	events := []recordings.CanonicalEvent{{
		ID:          "event-1",
		Sequence:    0,
		FactoryTick: 3,
		Scope:       scope,
		Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
		Payload:     `{"requestId":"request-1"}`,
	}}
	fake := &recordingsRootFake{
		events: events,
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         scope,
			SelectedTick:  3,
			Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
		},
	}
	adapter := NewRecordingsAdapter(fake)
	ctx := context.Background()

	snapshot, err := adapter.ReadWorkSnapshot(ctx, "session-recordings")
	if err != nil {
		t.Fatalf("ReadWorkSnapshot: %v", err)
	}
	if len(fake.reconstructRequests) != 1 {
		t.Fatalf("reconstruct requests = %d, want 1", len(fake.reconstructRequests))
	}
	request := fake.reconstructRequests[0]
	if request.Scope != scope || request.SelectedTick != 3 || len(request.Events) != 1 {
		t.Fatalf("ReconstructWorldStateRequest = %#v, want scoped tick-3 replay", request)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != "work-story" {
		t.Fatalf("snapshot = %#v, want one story work item", snapshot)
	}
	if snapshot.Items[0].State == nil || snapshot.Items[0].State.Name != "review" {
		t.Fatalf("snapshot state = %#v, want review", snapshot.Items[0].State)
	}
}

func TestNewRecordingsAdapterTypedProjectionFailures(t *testing.T) {
	t.Parallel()

	adapter := NewRecordingsAdapter(&recordingsRootFake{
		reconstructErr: recordings.ErrInvalidProjectionInput,
	})
	_, err := adapter.ReadWorkSnapshot(context.Background(), "session-recordings")
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReadWorkSnapshot error = %v, want ErrInvalidProjectionInput", err)
	}
}

func TestNewRecordingsAdapterNilRootReturnsNil(t *testing.T) {
	t.Parallel()

	if adapter := NewRecordingsAdapter(nil); adapter != nil {
		t.Fatalf("NewRecordingsAdapter(nil) = %#v, want nil", adapter)
	}
}

func TestReadWorkSnapshotValidatesContextAndSession(t *testing.T) {
	t.Parallel()

	adapter := NewRecordingsAdapter(&recordingsRootFake{})

	_, err := adapter.ReadWorkSnapshot(nil, "session-1")
	if err == nil || err.Error() != "Work state access context is required" {
		t.Fatalf("ReadWorkSnapshot(nil ctx) error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.ReadWorkSnapshot(canceled, "session-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadWorkSnapshot(canceled ctx) error = %v, want context.Canceled", err)
	}

	_, err = adapter.ReadWorkSnapshot(context.Background(), "  ")
	if !errors.Is(err, recordings.ErrInvalidProjectionScope) {
		t.Fatalf("ReadWorkSnapshot(empty session) error = %v, want ErrInvalidProjectionScope", err)
	}
}

func TestReadWorkSnapshotSubscribeAndSubscriptionFailures(t *testing.T) {
	t.Parallel()

	t.Run("subscribe error", func(t *testing.T) {
		t.Parallel()
		adapter := NewRecordingsAdapter(&recordingsRootFake{subscribeErr: errors.New("subscribe failed")})
		_, err := adapter.ReadWorkSnapshot(context.Background(), "session-1")
		if err == nil || !strings.Contains(err.Error(), "subscribe Recordings canonical facts") {
			t.Fatalf("ReadWorkSnapshot error = %v, want wrapped subscribe failure", err)
		}
	})

	t.Run("subscription gap", func(t *testing.T) {
		t.Parallel()
		adapter := NewRecordingsAdapter(&recordingsRootFake{
			subscription: func(context.Context) recordings.SubscriptionOutcome {
				return recordings.SubscriptionOutcome{
					Kind: recordings.SubscriptionGap,
					Gap:  &recordings.SubscriptionGapFacts{ExpectedSequence: 42},
				}
			},
		})
		_, err := adapter.ReadWorkSnapshot(context.Background(), "session-1")
		if err == nil || !strings.Contains(err.Error(), "recordings subscription gap at sequence 42") {
			t.Fatalf("ReadWorkSnapshot error = %v, want subscription gap", err)
		}
	})

	t.Run("subscription gap without detail", func(t *testing.T) {
		t.Parallel()
		adapter := NewRecordingsAdapter(&recordingsRootFake{
			subscription: func(context.Context) recordings.SubscriptionOutcome {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionGap}
			},
		})
		_, err := adapter.ReadWorkSnapshot(context.Background(), "session-1")
		if err == nil || err.Error() != "recordings subscription gap" {
			t.Fatalf("ReadWorkSnapshot error = %v, want generic subscription gap", err)
		}
	})

	t.Run("nil subscription", func(t *testing.T) {
		t.Parallel()
		adapter := NewRecordingsAdapter(&recordingsRootFake{
			subscription: nil,
			worldState: recordings.WorldStateView{
				SchemaVersion: recordings.WorldStateViewSchemaV1,
				Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
			},
		})
		snapshot, err := adapter.ReadWorkSnapshot(context.Background(), "session-1")
		if err != nil {
			t.Fatalf("ReadWorkSnapshot: %v", err)
		}
		if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != "work-story" {
			t.Fatalf("snapshot = %#v, want one recorded work item", snapshot)
		}
	})

	t.Run("unknown subscription outcome after event", func(t *testing.T) {
		t.Parallel()
		index := 0
		adapter := NewRecordingsAdapter(&recordingsRootFake{
			subscription: func(context.Context) recordings.SubscriptionOutcome {
				if index == 0 {
					index++
					return recordings.SubscriptionOutcome{
						Kind:  recordings.SubscriptionEvent,
						Event: recordings.CanonicalEvent{ID: "event-1", FactoryTick: 1},
					}
				}
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionOutcomeKind("unknown")}
			},
			worldState: recordings.WorldStateView{
				SchemaVersion: recordings.WorldStateViewSchemaV1,
				Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
			},
		})
		snapshot, err := adapter.ReadWorkSnapshot(context.Background(), "session-1")
		if err != nil {
			t.Fatalf("ReadWorkSnapshot: %v", err)
		}
		if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != "work-story" {
			t.Fatalf("snapshot = %#v, want one recorded work item", snapshot)
		}
	})
}

func TestCanonicalEventsFromSubscriptionReturnsNilForNilSubscription(t *testing.T) {
	t.Parallel()

	got, err := canonicalEventsFromSubscription(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("canonicalEventsFromSubscription(nil) = (%#v, %v), want (nil, nil)", got, err)
	}
}

func TestCanonicalEventsFromSubscriptionCollectsMultipleEvents(t *testing.T) {
	t.Parallel()

	index := 0
	events := []recordings.CanonicalEvent{
		{ID: "event-1", FactoryTick: 1},
		{ID: "event-2", FactoryTick: 2},
	}
	subscription := recordings.EventSubscription(func(context.Context) recordings.SubscriptionOutcome {
		if index >= len(events) {
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
		outcome := recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: events[index]}
		index++
		return outcome
	})

	got, err := canonicalEventsFromSubscription(context.Background(), subscription)
	if err != nil {
		t.Fatalf("canonicalEventsFromSubscription: %v", err)
	}
	if len(got) != 2 || got[1].FactoryTick != 2 {
		t.Fatalf("events = %#v, want two collected events", got)
	}
}

func TestCanonicalEventsFromSubscriptionReturnsPartialEventsOnUnknownOutcome(t *testing.T) {
	t.Parallel()

	index := 0
	subscription := recordings.EventSubscription(func(context.Context) recordings.SubscriptionOutcome {
		if index == 0 {
			index++
			return recordings.SubscriptionOutcome{
				Kind:  recordings.SubscriptionEvent,
				Event: recordings.CanonicalEvent{ID: "event-1"},
			}
		}
		return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionOutcomeKind("unknown")}
	})

	got, err := canonicalEventsFromSubscription(context.Background(), subscription)
	if err != nil {
		t.Fatalf("canonicalEventsFromSubscription: %v", err)
	}
	if len(got) != 1 || got[0].ID != "event-1" {
		t.Fatalf("events = %#v, want partial collected events", got)
	}
}

func TestReadSnapshotFromWorldStateDetachesWorkFields(t *testing.T) {
	t.Parallel()

	view := recordings.WorldStateView{
		SchemaVersion: recordings.WorldStateViewSchemaV1,
		Payload: recordingsBackedWorldPayload(
			t,
			"work-story",
			"story",
			"review",
		),
	}
	snapshot, err := readSnapshotFromWorldState(view)
	if err != nil {
		t.Fatalf("readSnapshotFromWorldState: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("snapshot = %#v, want one item", snapshot)
	}
	snapshot.Items[0].State.Name = "mutated"
	second, err := readSnapshotFromWorldState(view)
	if err != nil || second.Items[0].State.Name != "review" {
		t.Fatalf("detached snapshot mutated source: %#v, %v", second, err)
	}
}

type recordingsRootFake struct {
	events              []recordings.CanonicalEvent
	worldState          recordings.WorldStateView
	subscribeErr        error
	subscription        recordings.EventSubscription
	reconstructErr      error
	reconstructRequests []recordings.ReconstructWorldStateRequest
}

func (fake *recordingsRootFake) Append(
	recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
}

func (fake *recordingsRootFake) SubscribeFrom(
	_ context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if fake.subscribeErr != nil {
		return recordings.SubscribeResult{}, fake.subscribeErr
	}
	if fake.subscription != nil {
		return recordings.SubscribeResult{Subscription: fake.subscription}, nil
	}
	events := make([]recordings.CanonicalEvent, 0, len(fake.events))
	for _, event := range fake.events {
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
			outcome := recordings.SubscriptionOutcome{
				Kind:  recordings.SubscriptionEvent,
				Event: events[index],
			}
			index++
			return outcome
		},
	}, nil
}

func (fake *recordingsRootFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	fake.reconstructRequests = append(fake.reconstructRequests, request)
	if fake.reconstructErr != nil {
		return recordings.ReconstructWorldStateResult{}, fake.reconstructErr
	}
	return recordings.ReconstructWorldStateResult{WorldState: fake.worldState}, nil
}

func (fake *recordingsRootFake) QuerySimpleDashboard(
	recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	return recordings.SimpleDashboardQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *recordingsRootFake) QueryWorkstationRequests(
	recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *recordingsRootFake) ValidateReconnectReplayFrom(
	recordings.ValidateReconnectReplayRequest,
) error {
	return recordings.ErrInvalidProjectionInput
}

func (fake *recordingsRootFake) BindRecording(
	recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) StartRecording(
	recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) RecordRecordingEvent(
	recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (fake *recordingsRootFake) RecordRecordingError(
	recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (fake *recordingsRootFake) FlushRecording(
	recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) StopRecording(
	recordings.StopRecordingRequest,
) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) FinishRecording(
	recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) QueryRecordingStatus(
	recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) LoadReplayRecording(
	recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (fake *recordingsRootFake) CreateReplayPlan(
	recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) ObserveReplay(
	recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) BuildPortableArtifact(
	recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) ValidatePortableArtifact(
	recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) EncodePortableArtifact(
	recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) DecodePortableArtifact(
	recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) SummarizePortableArtifact(
	recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) ExportPortableArtifact(
	context.Context,
	recordings.ExportPortableArtifactRequest,
) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) ReadPortableArtifact(
	context.Context,
	recordings.ReadPortableArtifactRequest,
) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func recordingsBackedWorldPayload(
	t *testing.T,
	workID string,
	workTypeID string,
	stateName string,
) string {
	t.Helper()
	state := interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				ID: workTypeID,
				States: []interfaces.FactoryStateDefinition{
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
				State:       stateName,
			},
		},
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	return string(payload)
}
