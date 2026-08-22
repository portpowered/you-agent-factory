package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestWorkSnapshotReaderConstructsRecordingsRequestsThroughRoot proves the
// Recordings-owned Work snapshot reader constructs Recordings queries only
// through the published Recordings service root contract. The fake implements
// the whole published root, so any reach past the two query operations would
// show up here. Each read builds its own scoped subscribe/reconstruct pair, so
// a consumer that reads twice never reuses a previous read's replay window.
func TestWorkSnapshotReaderConstructsRecordingsRequestsThroughRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-root"}
	events := []recordings.CanonicalEvent{{
		ID:          "event-1",
		Sequence:    0,
		FactoryTick: 3,
		Scope:       scope,
		Kind:        recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:     `{"requestId":"request-1"}`,
	}}
	fake := &workRecordingsRootFake{
		events: events,
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         scope,
			SelectedTick:  3,
			Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
		},
	}
	reader := recordingswire.NewWorkSnapshotReader(fake)
	ctx := context.Background()

	first, err := reader.ReadWorkSnapshot(ctx, "session-recordings-root")
	if err != nil {
		t.Fatalf("ReadWorkSnapshot: %v", err)
	}
	assertReviewStorySnapshot(t, first)

	second, err := reader.ReadWorkSnapshot(ctx, "session-recordings-root")
	if err != nil {
		t.Fatalf("second ReadWorkSnapshot: %v", err)
	}
	assertReviewStorySnapshot(t, second)

	if len(fake.subscribeRequests) != 2 {
		t.Fatalf("subscribe requests = %d, want one per read", len(fake.subscribeRequests))
	}
	for _, request := range fake.subscribeRequests {
		if request.Scope != scope {
			t.Fatalf("SubscribeRequest scope = %#v, want %#v", request.Scope, scope)
		}
	}
	if len(fake.reconstructRequests) != 2 {
		t.Fatalf("reconstruct requests = %d, want one per read", len(fake.reconstructRequests))
	}
	for _, request := range fake.reconstructRequests {
		if request.Scope != scope || request.SelectedTick != 3 || len(request.Events) != 1 {
			t.Fatalf("ReconstructWorldStateRequest = %#v, want scoped tick-3 replay", request)
		}
	}
}

// TestWorkSnapshotReaderTypedRootFailuresStayClassifiable proves a typed
// failure raised by the published Recordings root reaches the reader's caller
// unchanged, so a Work consumer can still classify it by Recordings sentinel
// without importing anything beyond the sentinel it already names.
func TestWorkSnapshotReaderTypedRootFailuresStayClassifiable(t *testing.T) {
	t.Parallel()

	reader := recordingswire.NewWorkSnapshotReader(&workRecordingsRootFake{
		reconstructErr: recordings.ErrInvalidProjectionInput,
	})
	_, err := reader.ReadWorkSnapshot(context.Background(), "session-recordings-root")
	if err == nil {
		t.Fatal("ReadWorkSnapshot error = nil, want typed projection failure")
	}
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReadWorkSnapshot error = %v, want ErrInvalidProjectionInput", err)
	}
}

func assertReviewStorySnapshot(t *testing.T, snapshot work.ReadSnapshot) {
	t.Helper()
	if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != "work-story" {
		t.Fatalf("snapshot = %#v, want one story work item", snapshot)
	}
	if snapshot.Items[0].State == nil || snapshot.Items[0].State.Name != "review" {
		t.Fatalf("snapshot state = %#v, want review", snapshot.Items[0].State)
	}
}

type workRecordingsRootFake struct {
	events              []recordings.CanonicalEvent
	worldState          recordings.WorldStateView
	reconstructErr      error
	subscribeRequests   []recordings.SubscribeRequest
	reconstructRequests []recordings.ReconstructWorldStateRequest
}

func (fake *workRecordingsRootFake) Append(
	recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
}

func (fake *workRecordingsRootFake) SubscribeFrom(
	_ context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	fake.subscribeRequests = append(fake.subscribeRequests, request)
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

func (fake *workRecordingsRootFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	fake.reconstructRequests = append(fake.reconstructRequests, request)
	if fake.reconstructErr != nil {
		return recordings.ReconstructWorldStateResult{}, fake.reconstructErr
	}
	return recordings.ReconstructWorldStateResult{WorldState: fake.worldState}, nil
}

func (fake *workRecordingsRootFake) QuerySimpleDashboard(
	recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	return recordings.SimpleDashboardQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *workRecordingsRootFake) QueryWorkstationRequests(
	recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *workRecordingsRootFake) ValidateReconnectReplayFrom(
	recordings.ValidateReconnectReplayRequest,
) error {
	return recordings.ErrInvalidProjectionInput
}

func (fake *workRecordingsRootFake) BindRecording(
	recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) StartRecording(
	recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) RecordRecordingEvent(
	recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (fake *workRecordingsRootFake) RecordRecordingError(
	recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (fake *workRecordingsRootFake) FlushRecording(
	recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) StopRecording(
	recordings.StopRecordingRequest,
) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) FinishRecording(
	recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) QueryRecordingStatus(
	recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) LoadReplayRecording(
	recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (fake *workRecordingsRootFake) LoadReplayRecordingForResume(
	recordings.LoadReplayRecordingForResumeRequest,
) (recordings.LoadReplayRecordingForResumeResult, error) {
	return recordings.LoadReplayRecordingForResumeResult{}, recordings.ErrMissingReplayArtifact
}

func (fake *workRecordingsRootFake) CreateReplayPlan(
	recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) ObserveReplay(
	recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) BuildPortableArtifact(
	recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) ValidatePortableArtifact(
	recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) EncodePortableArtifact(
	recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) DecodePortableArtifact(
	recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) SummarizePortableArtifact(
	recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *workRecordingsRootFake) ExportPortableArtifact(
	context.Context,
	recordings.ExportPortableArtifactRequest,
) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) ReadPortableArtifact(
	context.Context,
	recordings.ReadPortableArtifactRequest,
) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *workRecordingsRootFake) BeginRecordingScope(context.Context, recordings.BeginRecordingScopeRequest) (recordings.BeginRecordingScopeResult, error) {
	return recordings.BeginRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) AppendRecordingScopeEvent(context.Context, recordings.AppendRecordingScopeEventRequest) (recordings.AppendRecordingScopeEventResult, error) {
	return recordings.AppendRecordingScopeEventResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) FlushRecordingScope(context.Context, recordings.FlushRecordingScopeRequest) (recordings.FlushRecordingScopeResult, error) {
	return recordings.FlushRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) FinalizeRecordingScope(context.Context, recordings.FinalizeRecordingScopeRequest) (recordings.FinalizeRecordingScopeResult, error) {
	return recordings.FinalizeRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) CloseRecordingScope(context.Context, recordings.CloseRecordingScopeRequest) (recordings.CloseRecordingScopeResult, error) {
	return recordings.CloseRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) QueryRecordingScope(context.Context, recordings.QueryRecordingScopeRequest) (recordings.QueryRecordingScopeResult, error) {
	return recordings.QueryRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) OpenRecordingScope(context.Context, recordings.OpenRecordingScopeRequest) (recordings.OpenRecordingScopeResult, error) {
	return recordings.OpenRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) SubscribeRecordingScope(context.Context, recordings.SubscribeRecordingScopeRequest) (recordings.SubscribeRecordingScopeResult, error) {
	return recordings.SubscribeRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) LoadReplayRecordingScope(context.Context, recordings.LoadReplayRecordingScopeRequest) (recordings.LoadReplayRecordingScopeResult, error) {
	return recordings.LoadReplayRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) CreateReplayPlanScope(context.Context, recordings.CreateReplayPlanScopeRequest) (recordings.CreateReplayPlanScopeResult, error) {
	return recordings.CreateReplayPlanScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) ObserveReplayScope(context.Context, recordings.ObserveReplayScopeRequest) (recordings.ObserveReplayScopeResult, error) {
	return recordings.ObserveReplayScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) ReconstructRecordingScope(context.Context, recordings.ReconstructRecordingScopeRequest) (recordings.ReconstructRecordingScopeResult, error) {
	return recordings.ReconstructRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) QuerySimpleDashboardScope(context.Context, recordings.QuerySimpleDashboardScopeRequest) (recordings.QuerySimpleDashboardScopeResult, error) {
	return recordings.QuerySimpleDashboardScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) QueryWorkstationRequestsScope(context.Context, recordings.QueryWorkstationRequestsScopeRequest) (recordings.QueryWorkstationRequestsScopeResult, error) {
	return recordings.QueryWorkstationRequestsScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) BuildPortableArtifactScope(context.Context, recordings.BuildPortableArtifactScopeRequest) (recordings.BuildPortableArtifactScopeResult, error) {
	return recordings.BuildPortableArtifactScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) ExportPortableArtifactScope(context.Context, recordings.ExportPortableArtifactScopeRequest) (recordings.ExportPortableArtifactScopeResult, error) {
	return recordings.ExportPortableArtifactScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *workRecordingsRootFake) ReadPortableArtifactScope(context.Context, recordings.ReadPortableArtifactScopeRequest) (recordings.ReadPortableArtifactScopeResult, error) {
	return recordings.ReadPortableArtifactScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func recordingsBackedWorldPayload(
	t *testing.T,
	workID string,
	workTypeID string,
	stateName string,
) string {
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
