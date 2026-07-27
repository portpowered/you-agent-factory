package service_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestFinishRecordingStopsAppliesMetadataThenFinalFlush(t *testing.T) {
	t.Parallel()

	ticker := newManualFlushTicker()
	var mu sync.Mutex
	var snapshots []recordings.RecordingSnapshot
	root := newActiveFlushRoot(
		func(_ string, snapshot recordings.RecordingSnapshot) error {
			mu.Lock()
			defer mu.Unlock()
			snapshots = append(snapshots, snapshot)
			return nil
		},
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(ticker)
		},
	)
	recordingID := startActiveRecording(t, root, "recording-finalize", time.Second)
	event := activeFlushEvent(1)
	recordEvent(t, root, recordingID, event)
	finishedAt := time.Date(2026, 7, 27, 18, 30, 0, 0, time.UTC)

	finished, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  finishedAt,
	})
	if err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	if finished.Status.State != recordings.RecordingFinalized ||
		finished.Status.FinalizedAt == nil ||
		!finished.Status.FinalizedAt.Equal(finishedAt) ||
		finished.Status.FlushedThrough == nil ||
		*finished.Status.FlushedThrough != event.Cursor {
		t.Fatalf("final status = %#v", finished.Status)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(snapshots) != 1 ||
		snapshots[0].Status.FinalizedAt == nil ||
		!snapshots[0].Status.FinalizedAt.Equal(finishedAt) ||
		len(snapshots[0].Events) != 1 {
		t.Fatalf("final snapshots = %#v, want terminal metadata and latest event", snapshots)
	}
	if ticker.stopCalls.Load() != 1 {
		t.Fatalf("ticker stop calls = %d, want 1", ticker.stopCalls.Load())
	}
}

func TestFinishRecordingAttemptsFinalFlushAfterEarlierFailure(t *testing.T) {
	t.Parallel()

	var writes int
	periodicErr := errors.New("periodic write failed")
	producerErr := errors.New("producer failed before close")
	root := newActiveFlushRoot(
		func(_ string, snapshot recordings.RecordingSnapshot) error {
			writes++
			if snapshot.Status.FinalizedAt == nil {
				return periodicErr
			}
			return nil
		},
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(newManualFlushTicker())
		},
	)
	recordingID := startActiveRecording(t, root, "recording-final-after-failure", time.Second)
	recordEvent(t, root, recordingID, activeFlushEvent(1))
	if _, err := root.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: recordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "producer failed before close",
		},
		Cause: producerErr,
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	}); err == nil {
		t.Fatal("active FlushRecording error = nil, want earlier write failure")
	}

	finished, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Now().UTC(),
	})
	if !errors.Is(err, producerErr) || !errors.Is(err, periodicErr) {
		t.Fatalf("FinishRecording final retry error = %v, want accumulated causes", err)
	}
	if writes != 2 || finished.Status.FlushedThrough == nil ||
		finished.Status.State != recordings.RecordingFailed {
		t.Fatalf("finalization = (%d writes, %#v), want final retry and failed status", writes, finished.Status)
	}
}

func TestFinishRecordingReportsFailedStatusWhenFinalFlushFails(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("final storage unavailable")
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error { return writeErr },
		nil,
	)
	recordingID := startActiveRecording(t, root, "recording-final-write-fails", time.Second)
	recordEvent(t, root, recordingID, activeFlushEvent(1))

	finished, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Now().UTC(),
	})
	if !errors.Is(err, writeErr) ||
		finished.Status.State != recordings.RecordingFailed ||
		finished.Status.FinalizedAt == nil ||
		len(finished.Status.Failures) != 1 ||
		finished.Status.Failures[0].Code != "final_flush_failed" {
		t.Fatalf("failed FinishRecording = (%#v, %v)", finished, err)
	}

	repeated, repeatedErr := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Now().UTC().Add(time.Hour),
	})
	if !errors.Is(repeatedErr, writeErr) ||
		repeated.Status.FinalizedAt == nil ||
		!repeated.Status.FinalizedAt.Equal(*finished.Status.FinalizedAt) ||
		len(repeated.Status.Failures) != 1 {
		t.Fatalf("repeated failed FinishRecording = (%#v, %v)", repeated, repeatedErr)
	}
}

func TestFinishRecordingIsSingleFlightAndRejectsLaterEvents(t *testing.T) {
	t.Parallel()

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writes int
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error {
			writes++
			close(writeStarted)
			<-releaseWrite
			return nil
		},
		nil,
	)
	recordingID := startActiveRecording(t, root, "recording-finalize-once", time.Second)
	recordEvent(t, root, recordingID, activeFlushEvent(1))
	finishedAt := time.Now().UTC()

	results := make(chan recordings.FinishRecordingResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			result, err := root.FinishRecording(recordings.FinishRecordingRequest{
				RecordingID: recordingID,
				FinishedAt:  finishedAt,
			})
			results <- result
			errs <- err
		}()
	}
	<-writeStarted
	close(releaseWrite)
	for range 2 {
		result := <-results
		if err := <-errs; err != nil || result.Status.FinalizedAt == nil {
			t.Fatalf("repeated FinishRecording = (%#v, %v)", result, err)
		}
	}
	if writes != 1 {
		t.Fatalf("final writes = %d, want one", writes)
	}
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       activeFlushEvent(2),
	}); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finalize event error = %v, want ErrRecordingWriteRejected", err)
	}
}
