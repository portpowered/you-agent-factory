package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/recordingsqueries"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type projectionServiceRoot struct {
	projection recordings.ProjectionService
}

var _ recordings.Service = (*projectionServiceRoot)(nil)

func (adapter *projectionServiceRoot) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
		Kind:        recordings.HistoricalRecordingQueryErrorUnavailable,
		RecordingID: request.Recording.RecordingID,
	}
}

func (adapter *projectionServiceRoot) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return recordingsqueries.ReconstructWorldStateFromProjection(adapter.projection, request)
}

func (adapter *projectionServiceRoot) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	return recordingsqueries.QuerySimpleDashboardFromProjection(adapter.projection, request)
}

func (adapter *projectionServiceRoot) QueryWorkstationRequests(
	request recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	state, err := recordingsqueries.DecodeWorldStatePayload(request.WorldState)
	if err != nil {
		return recordings.WorkstationRequestsQueryResult{}, err
	}
	return recordings.WorkstationRequestsQueryResult{
		Projection: adapter.projection.ProjectWorkstationRequests(state),
	}, nil
}

func (adapter *projectionServiceRoot) ValidateReconnectReplayFrom(
	request recordings.ValidateReconnectReplayRequest,
) error {
	return recordingsqueries.ValidateReconnectReplayFromProjection(adapter.projection, request)
}

func (adapter *projectionServiceRoot) Append(
	recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
}

func (adapter *projectionServiceRoot) SubscribeFrom(
	context.Context,
	recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	return recordings.SubscribeResult{}, recordings.ErrReconnectCursorNotFound
}

func (adapter *projectionServiceRoot) BindRecording(
	recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) StartRecording(
	recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) RecordRecordingEvent(
	recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (adapter *projectionServiceRoot) RecordRecordingError(
	recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (adapter *projectionServiceRoot) FlushRecording(
	recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) StopRecording(
	recordings.StopRecordingRequest,
) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) FinishRecording(
	recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) QueryRecordingStatus(
	recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) BeginRecordingScope(
	context.Context,
	recordings.BeginRecordingScopeRequest,
) (recordings.BeginRecordingScopeResult, error) {
	return recordings.BeginRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) AppendRecordingScopeEvent(
	context.Context,
	recordings.AppendRecordingScopeEventRequest,
) (recordings.AppendRecordingScopeEventResult, error) {
	return recordings.AppendRecordingScopeEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (adapter *projectionServiceRoot) FlushRecordingScope(
	context.Context,
	recordings.FlushRecordingScopeRequest,
) (recordings.FlushRecordingScopeResult, error) {
	return recordings.FlushRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) FinalizeRecordingScope(
	context.Context,
	recordings.FinalizeRecordingScopeRequest,
) (recordings.FinalizeRecordingScopeResult, error) {
	return recordings.FinalizeRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) CloseRecordingScope(
	context.Context,
	recordings.CloseRecordingScopeRequest,
) (recordings.CloseRecordingScopeResult, error) {
	return recordings.CloseRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) QueryRecordingScope(
	context.Context,
	recordings.QueryRecordingScopeRequest,
) (recordings.QueryRecordingScopeResult, error) {
	return recordings.QueryRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) OpenRecordingScope(
	context.Context,
	recordings.OpenRecordingScopeRequest,
) (recordings.OpenRecordingScopeResult, error) {
	return recordings.OpenRecordingScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) SubscribeRecordingScope(
	context.Context,
	recordings.SubscribeRecordingScopeRequest,
) (recordings.SubscribeRecordingScopeResult, error) {
	return recordings.SubscribeRecordingScopeResult{}, recordings.ErrReconnectCursorNotFound
}

func (adapter *projectionServiceRoot) LoadReplayRecordingScope(
	context.Context,
	recordings.LoadReplayRecordingScopeRequest,
) (recordings.LoadReplayRecordingScopeResult, error) {
	return recordings.LoadReplayRecordingScopeResult{}, recordings.ErrMissingReplayArtifact
}

func (adapter *projectionServiceRoot) CreateReplayPlanScope(
	context.Context,
	recordings.CreateReplayPlanScopeRequest,
) (recordings.CreateReplayPlanScopeResult, error) {
	return recordings.CreateReplayPlanScopeResult{}, recordings.ErrInvalidReplayArtifact
}

func (adapter *projectionServiceRoot) ObserveReplayScope(
	context.Context,
	recordings.ObserveReplayScopeRequest,
) (recordings.ObserveReplayScopeResult, error) {
	return recordings.ObserveReplayScopeResult{}, recordings.ErrInvalidReplayArtifact
}

func (adapter *projectionServiceRoot) ReconstructRecordingScope(
	context.Context,
	recordings.ReconstructRecordingScopeRequest,
) (recordings.ReconstructRecordingScopeResult, error) {
	return recordings.ReconstructRecordingScopeResult{}, recordings.ErrInvalidProjectionInput
}

func (adapter *projectionServiceRoot) QuerySimpleDashboardScope(
	context.Context,
	recordings.QuerySimpleDashboardScopeRequest,
) (recordings.QuerySimpleDashboardScopeResult, error) {
	return recordings.QuerySimpleDashboardScopeResult{}, recordings.ErrInvalidProjectionInput
}

func (adapter *projectionServiceRoot) QueryWorkstationRequestsScope(
	context.Context,
	recordings.QueryWorkstationRequestsScopeRequest,
) (recordings.QueryWorkstationRequestsScopeResult, error) {
	return recordings.QueryWorkstationRequestsScopeResult{}, recordings.ErrInvalidProjectionInput
}

func (adapter *projectionServiceRoot) BuildPortableArtifactScope(
	context.Context,
	recordings.BuildPortableArtifactScopeRequest,
) (recordings.BuildPortableArtifactScopeResult, error) {
	return recordings.BuildPortableArtifactScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) ExportPortableArtifactScope(
	context.Context,
	recordings.ExportPortableArtifactScopeRequest,
) (recordings.ExportPortableArtifactScopeResult, error) {
	return recordings.ExportPortableArtifactScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) ReadPortableArtifactScope(
	context.Context,
	recordings.ReadPortableArtifactScopeRequest,
) (recordings.ReadPortableArtifactScopeResult, error) {
	return recordings.ReadPortableArtifactScopeResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) LoadReplayRecording(
	recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (adapter *projectionServiceRoot) LoadReplayRecordingForResume(
	recordings.LoadReplayRecordingForResumeRequest,
) (recordings.LoadReplayRecordingForResumeResult, error) {
	return recordings.LoadReplayRecordingForResumeResult{}, recordings.ErrMissingReplayArtifact
}

func (adapter *projectionServiceRoot) CreateReplayPlan(
	recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (adapter *projectionServiceRoot) ObserveReplay(
	recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (adapter *projectionServiceRoot) BuildPortableArtifact(
	recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) ValidatePortableArtifact(
	recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) EncodePortableArtifact(
	recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) DecodePortableArtifact(
	recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) SummarizePortableArtifact(
	recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) ExportPortableArtifact(
	context.Context,
	recordings.ExportPortableArtifactRequest,
) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (adapter *projectionServiceRoot) ReadPortableArtifact(
	context.Context,
	recordings.ReadPortableArtifactRequest,
) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}
