package http

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func assertHistoricalHTTPUsageWithTokens(t *testing.T, usage *factoryapi.FactoryDispatchUsage, duration, input, output, total int64) {
	t.Helper()
	if usage == nil {
		t.Fatal("token-present response usage = nil")
	}
	if usage.DurationMillis == nil || *usage.DurationMillis != duration {
		t.Fatalf("token-present response duration = %#v, want %d", usage.DurationMillis, duration)
	}
	if usage.InputTokens == nil || *usage.InputTokens != input {
		t.Fatalf("token-present response input tokens = %#v, want %d", usage.InputTokens, input)
	}
	if usage.OutputTokens == nil || *usage.OutputTokens != output {
		t.Fatalf("token-present response output tokens = %#v, want %d", usage.OutputTokens, output)
	}
	if usage.TotalTokens == nil || *usage.TotalTokens != total {
		t.Fatalf("token-present response total tokens = %#v, want %d", usage.TotalTokens, total)
	}
	if usage.CostUsd != nil {
		t.Fatalf("token-present response cost = %#v, want unset", usage.CostUsd)
	}
}

func assertHistoricalHTTPDurationOnlyUsage(t *testing.T, usage *factoryapi.FactoryDispatchUsage, duration int64) {
	t.Helper()
	if usage == nil {
		t.Fatal("token-absent response usage = nil")
	}
	if usage.DurationMillis == nil || *usage.DurationMillis != duration {
		t.Fatalf("token-absent response duration = %#v, want %d", usage.DurationMillis, duration)
	}
	if usage.InputTokens != nil {
		t.Fatalf("token-absent response input tokens = %#v, want nil", usage.InputTokens)
	}
	if usage.OutputTokens != nil {
		t.Fatalf("token-absent response output tokens = %#v, want nil", usage.OutputTokens)
	}
	if usage.TotalTokens != nil {
		t.Fatalf("token-absent response total tokens = %#v, want nil", usage.TotalTokens)
	}
	if usage.CostUsd != nil {
		t.Fatalf("token-absent response cost = %#v, want nil", usage.CostUsd)
	}
}
