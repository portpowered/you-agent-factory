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

func (adapter *projectionServiceRoot) LoadReplayRecording(
	recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
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
