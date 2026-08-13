// Package recordingsstub provides test doubles for Recordings root contracts used
// by Factory Visualization boundary tests.
package recordingsstub

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Service implements recordings.Service and recordings.ProjectionService for tests.
type Service struct {
	ReconstructWorldStateFn func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
	QuerySimpleDashboardFn  func(recordings.SimpleDashboardQueryRequest) (recordings.SimpleDashboardQueryResult, error)
	ValidateReconnectFn     func(recordings.ValidateReconnectReplayRequest) error

	DashboardData recordings.SimpleDashboardRenderData
}

var (
	_ recordings.Service           = (*Service)(nil)
	_ recordings.ProjectionService = (*Service)(nil)
)

func (stub *Service) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if stub != nil && stub.ReconstructWorldStateFn != nil {
		return stub.ReconstructWorldStateFn(request)
	}
	return recordings.ReconstructWorldStateResult{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			SelectedTick:  request.SelectedTick,
			Payload:       `{"topology":{}}`,
		},
	}, nil
}

func (stub *Service) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	if stub != nil && stub.QuerySimpleDashboardFn != nil {
		return stub.QuerySimpleDashboardFn(request)
	}
	data := recordings.SimpleDashboardRenderData{}
	if stub != nil {
		data = stub.DashboardData
	}
	return recordings.SimpleDashboardQueryResult{Data: data}, nil
}

func (stub *Service) ValidateReconnectReplayFrom(
	request recordings.ValidateReconnectReplayRequest,
) error {
	if stub != nil && stub.ValidateReconnectFn != nil {
		return stub.ValidateReconnectFn(request)
	}
	return nil
}

func (stub *Service) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) (factorydefinitions.FactoryWorldState, error) {
	result, err := stub.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       eventsToCanonical(events),
		SelectedTick: selectedTick,
	})
	if err != nil {
		return factorydefinitions.FactoryWorldState{}, err
	}
	var state factorydefinitions.FactoryWorldState
	_ = result.WorldState
	return state, nil
}

func (stub *Service) SimpleDashboardRenderData(
	_ factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	result, _ := stub.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       `{"topology":{}}`,
		},
	})
	return result.Data
}

func (stub *Service) ProjectActiveThrottlePauses(
	_ factorydefinitions.InitialStructurePayload,
	_ []factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

func (stub *Service) ProjectWorkstationRequests(
	_ factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (stub *Service) ValidateReconnectReplay(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) error {
	request, err := reconnectRequest(events, cursor, scope)
	if err != nil {
		return err
	}
	return stub.ValidateReconnectReplayFrom(request)
}

func (stub *Service) Append(recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
}

func (stub *Service) SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
	return recordings.SubscribeResult{}, recordings.ErrReconnectCursorNotFound
}

func (stub *Service) QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, nil
}

func (stub *Service) BindRecording(recordings.BindRecordingRequest) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) StartRecording(recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, nil
}

func (stub *Service) RecordRecordingEvent(recordings.RecordRecordingEventRequest) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (stub *Service) RecordRecordingError(recordings.RecordRecordingErrorRequest) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (stub *Service) FlushRecording(recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) StopRecording(recordings.StopRecordingRequest) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) FinishRecording(recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) BeginRecordingScope(
	context.Context,
	recordings.BeginRecordingScopeRequest,
) (recordings.BeginRecordingScopeResult, error) {
	return recordings.BeginRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) AppendRecordingScopeEvent(
	context.Context,
	recordings.AppendRecordingScopeEventRequest,
) (recordings.AppendRecordingScopeEventResult, error) {
	return recordings.AppendRecordingScopeEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (stub *Service) FlushRecordingScope(
	context.Context,
	recordings.FlushRecordingScopeRequest,
) (recordings.FlushRecordingScopeResult, error) {
	return recordings.FlushRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) FinalizeRecordingScope(
	context.Context,
	recordings.FinalizeRecordingScopeRequest,
) (recordings.FinalizeRecordingScopeResult, error) {
	return recordings.FinalizeRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) CloseRecordingScope(
	context.Context,
	recordings.CloseRecordingScopeRequest,
) (recordings.CloseRecordingScopeResult, error) {
	return recordings.CloseRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) QueryRecordingScope(
	context.Context,
	recordings.QueryRecordingScopeRequest,
) (recordings.QueryRecordingScopeResult, error) {
	return recordings.QueryRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) OpenRecordingScope(
	context.Context,
	recordings.OpenRecordingScopeRequest,
) (recordings.OpenRecordingScopeResult, error) {
	return recordings.OpenRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) SubscribeRecordingScope(
	context.Context,
	recordings.SubscribeRecordingScopeRequest,
) (recordings.SubscribeRecordingScopeResult, error) {
	return recordings.SubscribeRecordingScopeResult{}, recordings.ErrReconnectCursorNotFound
}

func (stub *Service) LoadReplayRecordingScope(
	context.Context,
	recordings.LoadReplayRecordingScopeRequest,
) (recordings.LoadReplayRecordingScopeResult, error) {
	return recordings.LoadReplayRecordingScopeResult{}, recordings.ErrMissingReplayArtifact
}

func (stub *Service) CreateReplayPlanScope(
	context.Context,
	recordings.CreateReplayPlanScopeRequest,
) (recordings.CreateReplayPlanScopeResult, error) {
	return recordings.CreateReplayPlanScopeResult{}, recordings.ErrInvalidReplayArtifact
}

func (stub *Service) ObserveReplayScope(
	context.Context,
	recordings.ObserveReplayScopeRequest,
) (recordings.ObserveReplayScopeResult, error) {
	return recordings.ObserveReplayScopeResult{}, recordings.ErrInvalidReplayArtifact
}

func (stub *Service) ReconstructRecordingScope(
	context.Context,
	recordings.ReconstructRecordingScopeRequest,
) (recordings.ReconstructRecordingScopeResult, error) {
	return recordings.ReconstructRecordingScopeResult{}, recordings.ErrInvalidProjectionInput
}

func (stub *Service) QuerySimpleDashboardScope(
	context.Context,
	recordings.QuerySimpleDashboardScopeRequest,
) (recordings.QuerySimpleDashboardScopeResult, error) {
	return recordings.QuerySimpleDashboardScopeResult{}, recordings.ErrInvalidProjectionInput
}

func (stub *Service) QueryWorkstationRequestsScope(
	context.Context,
	recordings.QueryWorkstationRequestsScopeRequest,
) (recordings.QueryWorkstationRequestsScopeResult, error) {
	return recordings.QueryWorkstationRequestsScopeResult{}, recordings.ErrInvalidProjectionInput
}

func (stub *Service) BuildPortableArtifactScope(
	context.Context,
	recordings.BuildPortableArtifactScopeRequest,
) (recordings.BuildPortableArtifactScopeResult, error) {
	return recordings.BuildPortableArtifactScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) ExportPortableArtifactScope(
	context.Context,
	recordings.ExportPortableArtifactScopeRequest,
) (recordings.ExportPortableArtifactScopeResult, error) {
	return recordings.ExportPortableArtifactScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) ReadPortableArtifactScope(
	context.Context,
	recordings.ReadPortableArtifactScopeRequest,
) (recordings.ReadPortableArtifactScopeResult, error) {
	return recordings.ReadPortableArtifactScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) LoadReplayRecording(recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (stub *Service) CreateReplayPlan(recordings.CreateReplayPlanRequest) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (stub *Service) ObserveReplay(recordings.ObserveReplayRequest) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (stub *Service) BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) EncodePortableArtifact(recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) DecodePortableArtifact(recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) ExportPortableArtifact(context.Context, recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *Service) ReadPortableArtifact(context.Context, recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func eventsToCanonical(events []factorydefinitions.FactoryEvent) []recordings.CanonicalEvent {
	canonical := make([]recordings.CanonicalEvent, len(events))
	for index, event := range events {
		canonical[index] = recordings.CanonicalEvent{
			ID:       recordings.CanonicalEventID(event.Id),
			Sequence: recordings.CanonicalEventSequence(event.Context.Sequence),
		}
	}
	return canonical
}

func reconnectRequest(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (recordings.ValidateReconnectReplayRequest, error) {
	canonicalEvents := eventsToCanonical(events)
	canonicalCursor := recordings.CanonicalEventCursor{
		StreamGenerationID: "factory-visualization",
	}
	if cursor.AfterSequence != nil {
		canonicalCursor.Sequence = recordings.CanonicalEventSequence(*cursor.AfterSequence)
	}
	for _, event := range events {
		if cursor.AfterEventID != "" && event.Id == cursor.AfterEventID {
			canonicalCursor.Sequence = recordings.CanonicalEventSequence(event.Context.Sequence)
			break
		}
	}
	return recordings.ValidateReconnectReplayRequest{
		Events: canonicalEvents,
		Cursor: canonicalCursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: scope.SessionID},
	}, nil
}
