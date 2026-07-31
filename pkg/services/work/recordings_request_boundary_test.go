package work_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

// TestWorkConstructsRecordingsRequestsThroughRoot proves CUT-WORK-REC story 003:
// leased Work state_access Recordings-backed reads construct Recordings queries
// only through the published Recordings service root contract.
func TestWorkConstructsRecordingsRequestsThroughRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-root"}
	events := []recordings.CanonicalEvent{{
		ID:          "event-1",
		Sequence:    0,
		FactoryTick: 3,
		Scope:       scope,
		Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
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
	svc := stateaccesswire.NewService(
		unavailableSessionResolver{},
		stateaccesswire.NewRecordingsAdapter(fake),
	)
	ctx := context.Background()

	list, err := svc.ListWork(ctx, "session-recordings-root", work.ListOptions{})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWork = %#v, want one story work item", list)
	}
	if list.Results[0].State == nil || list.Results[0].State.Name != "review" {
		t.Fatalf("ListWork state = %#v, want review", list.Results[0].State)
	}

	got, err := svc.GetWork(ctx, "session-recordings-root", "work-story")
	if err != nil || got.WorkID != "work-story" || got.State.Name != "review" {
		t.Fatalf("GetWork = %#v, %v", got, err)
	}

	if len(fake.subscribeRequests) != 2 {
		t.Fatalf("subscribe requests = %d, want 2 (ListWork and GetWork)", len(fake.subscribeRequests))
	}
	for _, request := range fake.subscribeRequests {
		if request.Scope != scope {
			t.Fatalf("SubscribeRequest scope = %#v, want %#v", request.Scope, scope)
		}
	}
	if len(fake.reconstructRequests) != 2 {
		t.Fatalf("reconstruct requests = %d, want 2", len(fake.reconstructRequests))
	}
	for _, request := range fake.reconstructRequests {
		if request.Scope != scope || request.SelectedTick != 3 || len(request.Events) != 1 {
			t.Fatalf("ReconstructWorldStateRequest = %#v, want scoped tick-3 replay", request)
		}
	}
}

// TestWorkRecordingsTypedProjectionFailuresSurfaceThroughReadEdge proves typed
// Recordings projection failures propagate through the leased Work read edge.
func TestWorkRecordingsTypedProjectionFailuresSurfaceThroughReadEdge(t *testing.T) {
	t.Parallel()

	svc := stateaccesswire.NewService(
		unavailableSessionResolver{},
		stateaccesswire.NewRecordingsAdapter(&workRecordingsRootFake{
			reconstructErr: recordings.ErrInvalidProjectionInput,
		}),
	)
	_, err := svc.ListWork(context.Background(), "session-recordings-root", work.ListOptions{})
	if err == nil {
		t.Fatal("ListWork error = nil, want typed projection failure")
	}
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ListWork error = %v, want ErrInvalidProjectionInput", err)
	}
}

// TestWorkRecordingsRequestConstructionImportsRecordingsRootOnly seals the
// request-construction path: Work boundary tests may depend on Recordings query
// helpers only through the service root contract.

type unavailableSessionResolver struct{}

func (unavailableSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return nil, errors.New("session unavailable")
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
