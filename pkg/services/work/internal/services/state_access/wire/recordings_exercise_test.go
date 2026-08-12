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

func (fake *recordingsRootFake) Append(recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
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
func (fake *recordingsRootFake) OpenRecordingScope(context.Context, recordings.OpenRecordingScopeRequest) (recordings.OpenRecordingScopeResult, error) {
	return recordings.OpenRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) SubscribeRecordingScope(context.Context, recordings.SubscribeRecordingScopeRequest) (recordings.SubscribeRecordingScopeResult, error) {
	return recordings.SubscribeRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) LoadReplayRecordingScope(context.Context, recordings.LoadReplayRecordingScopeRequest) (recordings.LoadReplayRecordingScopeResult, error) {
	return recordings.LoadReplayRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) CreateReplayPlanScope(context.Context, recordings.CreateReplayPlanScopeRequest) (recordings.CreateReplayPlanScopeResult, error) {
	return recordings.CreateReplayPlanScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) ObserveReplayScope(context.Context, recordings.ObserveReplayScopeRequest) (recordings.ObserveReplayScopeResult, error) {
	return recordings.ObserveReplayScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) ReconstructRecordingScope(context.Context, recordings.ReconstructRecordingScopeRequest) (recordings.ReconstructRecordingScopeResult, error) {
	return recordings.ReconstructRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) QuerySimpleDashboardScope(context.Context, recordings.QuerySimpleDashboardScopeRequest) (recordings.QuerySimpleDashboardScopeResult, error) {
	return recordings.QuerySimpleDashboardScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) QueryWorkstationRequestsScope(context.Context, recordings.QueryWorkstationRequestsScopeRequest) (recordings.QueryWorkstationRequestsScopeResult, error) {
	return recordings.QueryWorkstationRequestsScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) BuildPortableArtifactScope(context.Context, recordings.BuildPortableArtifactScopeRequest) (recordings.BuildPortableArtifactScopeResult, error) {
	return recordings.BuildPortableArtifactScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) ExportPortableArtifactScope(context.Context, recordings.ExportPortableArtifactScopeRequest) (recordings.ExportPortableArtifactScopeResult, error) {
	return recordings.ExportPortableArtifactScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) ReadPortableArtifactScope(context.Context, recordings.ReadPortableArtifactScopeRequest) (recordings.ReadPortableArtifactScopeResult, error) {
	return recordings.ReadPortableArtifactScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) ReconstructWorldState(
	recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return recordings.ReconstructWorldStateResult{WorldState: fake.worldState}, nil
}

func (fake *recordingsRootFake) QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest) (recordings.SimpleDashboardQueryResult, error) {
	return recordings.SimpleDashboardQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *recordingsRootFake) QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *recordingsRootFake) ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest) error {
	return recordings.ErrInvalidProjectionInput
}

func (fake *recordingsRootFake) BindRecording(recordings.BindRecordingRequest) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) StartRecording(recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) RecordRecordingEvent(recordings.RecordRecordingEventRequest) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (fake *recordingsRootFake) RecordRecordingError(recordings.RecordRecordingErrorRequest) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (fake *recordingsRootFake) FlushRecording(recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) StopRecording(recordings.StopRecordingRequest) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) FinishRecording(recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) LoadReplayRecording(recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (fake *recordingsRootFake) CreateReplayPlan(recordings.CreateReplayPlanRequest) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) ObserveReplay(recordings.ObserveReplayRequest) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) EncodePortableArtifact(recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) DecodePortableArtifact(recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *recordingsRootFake) ExportPortableArtifact(context.Context, recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) ReadPortableArtifact(context.Context, recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *recordingsRootFake) BeginRecordingScope(context.Context, recordings.BeginRecordingScopeRequest) (recordings.BeginRecordingScopeResult, error) {
	return recordings.BeginRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) AppendRecordingScopeEvent(context.Context, recordings.AppendRecordingScopeEventRequest) (recordings.AppendRecordingScopeEventResult, error) {
	return recordings.AppendRecordingScopeEventResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) FlushRecordingScope(context.Context, recordings.FlushRecordingScopeRequest) (recordings.FlushRecordingScopeResult, error) {
	return recordings.FlushRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) FinalizeRecordingScope(context.Context, recordings.FinalizeRecordingScopeRequest) (recordings.FinalizeRecordingScopeResult, error) {
	return recordings.FinalizeRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) CloseRecordingScope(context.Context, recordings.CloseRecordingScopeRequest) (recordings.CloseRecordingScopeResult, error) {
	return recordings.CloseRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
}

func (fake *recordingsRootFake) QueryRecordingScope(context.Context, recordings.QueryRecordingScopeRequest) (recordings.QueryRecordingScopeResult, error) {
	return recordings.QueryRecordingScopeResult{}, recordings.ErrRecordingScopeUnknown
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
