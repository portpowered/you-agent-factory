package recordings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRecordingLifecycleRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	service := recordings.Service(&peerRootServiceFake{})
	ctx := context.Background()
	recordingID := assertLifecycleSuccessPath(t, service, ctx)
	assertLifecycleMissingTargetFailures(t, service)
	assertLifecycleFlushFailure(t, service, ctx)
	assertLifecyclePostFinishRejection(t, service, recordingID)
	if _, err := service.StopRecording(recordings.StopRecordingRequest{
		RecordingID: recordingID,
	}); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
}

func assertLifecycleSuccessPath(
	t *testing.T,
	service recordings.Service,
	ctx context.Context,
) string {
	t.Helper()
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordPath:    "session.replay.json",
		FlushInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BindRecording success path: %v", err)
	}
	if bound.RecordingID == "" {
		t.Fatal("BindRecording RecordingID empty, want bound handle")
	}
	if _, err := service.StartRecording(ctx, recordings.StartRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       interfaces.FactoryEvent{Id: "event-1", Type: "WORK_REQUEST"},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording success path: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: bound.RecordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	if !status.Finished {
		t.Fatalf("QueryRecordingStatus = %#v, want Finished true after finish", status)
	}
	return bound.RecordingID
}

func assertLifecycleMissingTargetFailures(t *testing.T, service recordings.Service) {
	t.Helper()
	_, err := service.BindRecording(recordings.BindRecordingRequest{RecordPath: ""})
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("missing recording target error = %v, want ErrMissingRecordingTarget", err)
	}
	_, err = service.FlushRecording(recordings.FlushRecordingRequest{RecordingID: "missing-id"})
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("missing recording id error = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertLifecycleFlushFailure(
	t *testing.T,
	service recordings.Service,
	ctx context.Context,
) {
	t.Helper()
	failing, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordPath: "failing.replay.json",
	})
	if err != nil {
		t.Fatalf("BindRecording for flush failure: %v", err)
	}
	if _, err := service.StartRecording(ctx, recordings.StartRecordingRequest{
		RecordingID: failing.RecordingID,
	}); err != nil {
		t.Fatalf("StartRecording for flush failure: %v", err)
	}
	if _, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: failing.RecordingID,
		Err:         errors.New("producer boundary failure"),
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	_, err = service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: failing.RecordingID,
	})
	if !errors.Is(err, recordings.ErrRecordingFlushFailed) {
		t.Fatalf("flush failure error = %v, want ErrRecordingFlushFailed", err)
	}
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("flush failure must remain distinct from ErrMissingRecordingTarget")
	}
}

func assertLifecyclePostFinishRejection(
	t *testing.T,
	service recordings.Service,
	recordingID string,
) {
	t.Helper()
	_, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       interfaces.FactoryEvent{Id: "event-after-finish", Type: "WORK_STATE_CHANGE"},
	})
	if !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write error = %v, want ErrRecordingWriteRejected", err)
	}
	if errors.Is(err, recordings.ErrMissingRecordingTarget) || errors.Is(err, recordings.ErrRecordingFlushFailed) {
		t.Fatalf("post-finish write rejection must remain distinct from other lifecycle typed errors")
	}
}
