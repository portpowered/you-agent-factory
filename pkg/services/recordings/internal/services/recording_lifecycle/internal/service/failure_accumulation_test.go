package service_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type fixedRecordingClock struct {
	now time.Time
}

func (clock fixedRecordingClock) Now() time.Time {
	return clock.now
}

type mutableRecordingError struct {
	message string
}

func (err *mutableRecordingError) Error() string {
	return err.message
}

func TestLifecycleFailuresAccumulateInOccurrenceOrderAndTerminalError(t *testing.T) {
	t.Parallel()

	failureTime := time.Date(2026, 7, 27, 20, 0, 0, 0, time.UTC)
	periodicTime := failureTime.Add(time.Minute)
	periodicErr := errors.New("periodic storage unavailable")
	finalErr := errors.New("final storage unavailable")
	ticker := newManualFlushTicker()
	var writes atomic.Int32
	root := newFailureTestRoot(
		func(string, recordings.RecordingSnapshot) error {
			if writes.Add(1) == 1 {
				return periodicErr
			}
			return finalErr
		},
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(ticker)
		},
		fixedRecordingClock{now: failureTime},
	)
	recordingID := startActiveRecording(t, root, "recording-accumulated-failures", time.Second)
	recordEvent(t, root, recordingID, activeFlushEvent(1))
	if _, err := root.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: recordingID,
		Failure: recordings.RecordingFailure{
			Code:    "producer_failed",
			Message: "producer canceled",
		},
		Cause: context.Canceled,
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}

	ticker.ticks <- periodicTime
	waitFor(t, func() bool {
		return len(recordingStatus(t, root, recordingID).Failures) == 2
	})

	finished, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
	})
	assertAccumulatedTerminalError(t, err, periodicErr, finalErr)
	assertAccumulatedFailureStatus(t, finished.Status, failureTime, periodicTime)
	assertErrorTextOrder(t, err, []string{
		"producer canceled",
		"periodic storage unavailable",
		"FinishedAt is required",
		"final storage unavailable",
	})

	finished.Status.Failures[0].Message = "caller mutation"
	status := recordingStatus(t, root, recordingID)
	if status.Failures[0].Message != "producer canceled" {
		t.Fatalf("caller mutated lifecycle failure status: %#v", status.Failures)
	}
	repeated, repeatedErr := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  failureTime.Add(time.Hour),
	})
	if !errors.Is(repeatedErr, context.Canceled) ||
		len(repeated.Status.Failures) != 4 ||
		writes.Load() != 2 {
		t.Fatalf("repeated finalization = (%#v, %v, %d writes)", repeated, repeatedErr, writes.Load())
	}
}

func assertAccumulatedTerminalError(
	t *testing.T,
	err error,
	periodicErr error,
	finalErr error,
) {
	t.Helper()
	if !errors.Is(err, context.Canceled) ||
		!errors.Is(err, periodicErr) ||
		!errors.Is(err, recordings.ErrInvalidRecordingTerminalMetadata) ||
		!errors.Is(err, finalErr) {
		t.Fatalf("FinishRecording error = %v, want every underlying cause", err)
	}
}

func assertAccumulatedFailureStatus(
	t *testing.T,
	status recordings.RecordingStatusFacts,
	failureTime time.Time,
	periodicTime time.Time,
) {
	t.Helper()
	wantCodes := []string{
		"producer_failed",
		"periodic_flush_failed",
		"terminal_metadata_failed",
		"final_flush_failed",
	}
	if status.State != recordings.RecordingFailed ||
		status.FinalizedAt != nil ||
		len(status.Failures) != len(wantCodes) {
		t.Fatalf("terminal failure status = %#v", status)
	}
	for index, wantCode := range wantCodes {
		if got := status.Failures[index].Code; got != wantCode {
			t.Fatalf("failure code %d = %q, want %q", index, got, wantCode)
		}
	}
	if !status.Failures[0].RecordedAt.Equal(failureTime) ||
		!status.Failures[1].RecordedAt.Equal(periodicTime) {
		t.Fatalf("failure timestamps = %#v, want injected occurrence times", status.Failures)
	}
}

func TestSnapshotEncodingFailureIsDetachedAndRetriedAtFinalFlush(t *testing.T) {
	t.Parallel()

	writeCalls := atomic.Int32{}
	writer := recordingsservice.NewRecordingSnapshotWriter(
		func(string, []byte) error {
			writeCalls.Add(1)
			return nil
		},
	)
	root := newFailureTestRoot(
		writer,
		nil,
		fixedRecordingClock{now: time.Date(2026, 7, 27, 21, 0, 0, 0, time.UTC)},
	)
	recordingID := startActiveRecording(t, root, "recording-encoding-failure", time.Second)
	event := activeFlushEvent(1)
	event.RecordedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	recordEvent(t, root, recordingID, event)

	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	}); !errors.Is(err, recordings.ErrRecordingSnapshotEncoding) {
		t.Fatalf("FlushRecording error = %v, want encoding identity", err)
	}
	status := recordingStatus(t, root, recordingID)
	if len(status.Failures) != 1 ||
		status.Failures[0].Code != "snapshot_encoding_failed" ||
		status.FlushedThrough != nil ||
		writeCalls.Load() != 0 {
		t.Fatalf("encoding failure status = %#v, writes = %d", status, writeCalls.Load())
	}

	finished, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Date(2026, 7, 27, 21, 1, 0, 0, time.UTC),
	})
	if !errors.Is(err, recordings.ErrRecordingSnapshotEncoding) ||
		finished.Status.State != recordings.RecordingFailed ||
		len(finished.Status.Failures) != 2 ||
		finished.Status.Failures[1].Code != "snapshot_encoding_failed" ||
		writeCalls.Load() != 0 {
		t.Fatalf("encoding finalization = (%#v, %v, %d writes)", finished, err, writeCalls.Load())
	}
}

func TestProducerCauseMessageIsDetachedWhileErrorIdentityIsPreserved(t *testing.T) {
	t.Parallel()

	failureTime := time.Date(2026, 7, 27, 22, 0, 0, 0, time.UTC)
	root := newFailureTestRoot(nil, nil, fixedRecordingClock{now: failureTime})
	recordingID := startActiveRecording(t, root, "recording-detached-cause", time.Second)
	cause := &mutableRecordingError{message: "original implementation error"}
	if _, err := root.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: recordingID,
		Failure: recordings.RecordingFailure{
			Code:    "producer_failed",
			Message: "safe producer failure",
		},
		Cause: cause,
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	cause.message = "mutated implementation error"

	finished, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  failureTime.Add(time.Minute),
	})
	if !errors.Is(err, cause) ||
		!strings.Contains(err.Error(), "original implementation error") ||
		strings.Contains(err.Error(), "mutated implementation error") {
		t.Fatalf("detached terminal error = %v, want stable text and original identity", err)
	}
	if len(finished.Status.Failures) != 1 ||
		finished.Status.Failures[0].Message != "safe producer failure" ||
		!finished.Status.Failures[0].RecordedAt.Equal(failureTime) {
		t.Fatalf("detached producer status = %#v", finished.Status)
	}
}

func newFailureTestRoot(
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	clock recordings.RecordingClock,
) recordings.Service {
	return recordingsservice.NewServiceWithLifecycleEffects(
		&unusedLedger{},
		recordingsservice.NewProjectionService(),
		nil,
		writer,
		tickers,
		clock,
	)
}

func assertErrorTextOrder(t *testing.T, err error, messages []string) {
	t.Helper()
	text := err.Error()
	previous := -1
	for _, message := range messages {
		index := strings.Index(text, message)
		if index <= previous {
			t.Fatalf("error %q does not contain %q in occurrence order", text, message)
		}
		previous = index
	}
}
