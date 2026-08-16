package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// rootFake is a focused Recordings root fake for adapter-edge tests. It avoids
// constructing ledger, lifecycle flush, replay, or artifact-export graphs.
type rootFake struct {
	recordings.Service

	streamGenerationID       string
	subscribeFrom            func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
	queryRecordingStatus     func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	queryHistoricalRecording func(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	buildPortableArtifact    func(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	reconstructWorldState    func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
}

func (fake *rootFake) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if fake.subscribeFrom != nil {
		return fake.subscribeFrom(ctx, request)
	}
	return recordings.SubscribeResult{}, recordings.ErrInvalidSubscribeScope
}

func (fake *rootFake) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	if fake.queryRecordingStatus != nil {
		return fake.queryRecordingStatus(request)
	}
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *rootFake) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	if fake.queryHistoricalRecording != nil {
		return fake.queryHistoricalRecording(request)
	}
	return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
		Kind:        recordings.HistoricalRecordingQueryErrorUnavailable,
		RecordingID: request.Recording.RecordingID,
	}
}

func (fake *rootFake) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	if fake.buildPortableArtifact != nil {
		return fake.buildPortableArtifact(request)
	}
	return recordings.BuildPortableArtifactResult{}, recordings.ErrPortableArtifactUnavailable
}

func (fake *rootFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if fake.reconstructWorldState != nil {
		return fake.reconstructWorldState(request)
	}
	return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
}
