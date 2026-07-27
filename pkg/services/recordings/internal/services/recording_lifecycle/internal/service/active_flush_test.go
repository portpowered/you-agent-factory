package service_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type manualFlushTicker struct {
	ticks     chan time.Time
	stopCalls atomic.Int32
}

func newManualFlushTicker() *manualFlushTicker {
	return &manualFlushTicker{ticks: make(chan time.Time, 16)}
}

func manualTickerHandle(ticker *manualFlushTicker) recordings.RecordingFlushTicker {
	return recordings.RecordingFlushTicker{
		Ticks: ticker.ticks,
		Stop:  func() { ticker.stopCalls.Add(1) },
	}
}

func TestActiveRecordingUsesConfiguredAndDefaultFlushCadence(t *testing.T) {
	t.Parallel()

	var intervals []time.Duration
	var tickers []*manualFlushTicker
	factory := func(interval time.Duration) recordings.RecordingFlushTicker {
		ticker := newManualFlushTicker()
		intervals = append(intervals, interval)
		tickers = append(tickers, ticker)
		return manualTickerHandle(ticker)
	}
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error { return nil },
		factory,
	)
	first := startActiveRecording(t, root, "recording-default", 0)
	second := startActiveRecording(t, root, "recording-configured", 3*time.Second)
	t.Cleanup(func() {
		stopRecording(t, root, first)
		stopRecording(t, root, second)
	})

	if len(intervals) != 2 ||
		intervals[0] != recordings.DefaultRecordingFlushInterval ||
		intervals[1] != 3*time.Second {
		t.Fatalf("flush intervals = %v", intervals)
	}
	if len(tickers) != 2 {
		t.Fatalf("ticker count = %d, want 2", len(tickers))
	}
}

func TestActiveFlushWritesOnlyDirtyConsistentSnapshots(t *testing.T) {
	t.Parallel()

	ticker := newManualFlushTicker()
	var mu sync.Mutex
	var snapshots []recordings.RecordingSnapshot
	root := newActiveFlushRoot(
		func(target string, snapshot recordings.RecordingSnapshot) error {
			if target != "artifact:active" {
				t.Errorf("write target = %q", target)
			}
			mu.Lock()
			snapshots = append(snapshots, snapshot)
			mu.Unlock()
			return nil
		},
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(ticker)
		},
	)
	recordingID := startActiveRecording(t, root, "recording-active", time.Second)
	t.Cleanup(func() { stopRecording(t, root, recordingID) })

	first := activeFlushEvent(1)
	recordEvent(t, root, recordingID, first)
	ticker.ticks <- time.Now()
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(snapshots) == 1
	})
	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	}); err != nil {
		t.Fatalf("clean FlushRecording: %v", err)
	}

	mu.Lock()
	if len(snapshots) != 1 || len(snapshots[0].Events) != 1 ||
		snapshots[0].Events[0].Cursor != first.Cursor {
		t.Fatalf("snapshots = %#v, want one consistent event snapshot", snapshots)
	}
	mu.Unlock()
	status := recordingStatus(t, root, recordingID)
	if status.FlushedThrough == nil || *status.FlushedThrough != first.Cursor {
		t.Fatalf("flushed status = %#v", status)
	}
}

func TestFailedFlushDoesNotReportUnwrittenPosition(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("storage unavailable")
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error { return writeErr },
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(newManualFlushTicker())
		},
	)
	recordingID := startActiveRecording(t, root, "recording-failed-write", time.Second)
	t.Cleanup(func() { stopRecording(t, root, recordingID) })
	recordEvent(t, root, recordingID, activeFlushEvent(1))

	_, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("FlushRecording error = %v, want storage error", err)
	}
	if status := recordingStatus(t, root, recordingID); status.FlushedThrough != nil {
		t.Fatalf("failed flush reported durable position: %#v", status)
	}
}

func TestStopRecordingJoinsPeriodicWriteAndPreventsLaterWrites(t *testing.T) {
	t.Parallel()

	ticker := newManualFlushTicker()
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writes atomic.Int32
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error {
			if writes.Add(1) == 1 {
				close(writeStarted)
				<-releaseWrite
			}
			return nil
		},
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(ticker)
		},
	)
	recordingID := startActiveRecording(t, root, "recording-stop", time.Second)
	recordEvent(t, root, recordingID, activeFlushEvent(1))
	ticker.ticks <- time.Now()
	<-writeStarted

	stopped := make(chan struct{})
	go func() {
		_, _ = root.StopRecording(recordings.StopRecordingRequest{
			RecordingID: recordingID,
		})
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("StopRecording returned before the active periodic write joined")
	case <-time.After(10 * time.Millisecond):
	}
	close(releaseWrite)
	<-stopped

	recordEvent(t, root, recordingID, activeFlushEvent(2))
	ticker.ticks <- time.Now()
	if writes.Load() != 1 || ticker.stopCalls.Load() != 1 {
		t.Fatalf("post-stop writes/ticker stops = %d/%d, want 1/1", writes.Load(), ticker.stopCalls.Load())
	}
	stopRecording(t, root, recordingID)
}

func TestConcurrentAcceptanceFlushAndStopRetainOrderedEvents(t *testing.T) {
	t.Parallel()

	ticker := newManualFlushTicker()
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error { return nil },
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(ticker)
		},
	)
	recordingID := startActiveRecording(t, root, "recording-race", time.Second)

	var producers sync.WaitGroup
	producers.Add(2)
	go func() {
		defer producers.Done()
		for sequence := int64(1); sequence <= 100; sequence++ {
			recordEvent(t, root, recordingID, activeFlushEvent(sequence))
		}
	}()
	go func() {
		defer producers.Done()
		for range 100 {
			_, _ = root.FlushRecording(recordings.FlushRecordingRequest{
				RecordingID: recordingID,
			})
		}
	}()
	producers.Wait()
	stopRecording(t, root, recordingID)

	status := recordingStatus(t, root, recordingID)
	if status.AcceptedEvents != 100 || status.LastEvent == nil ||
		status.LastEvent.Sequence != 100 {
		t.Fatalf("concurrent lifecycle status = %#v", status)
	}
}

func newActiveFlushRoot(
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
) recordings.Service {
	return recordingsservice.NewServiceWithLifecycleEffects(
		&unusedLedger{},
		recordingsservice.NewProjectionService(),
		nil,
		writer,
		tickers,
		nil,
	)
}

func startActiveRecording(
	t *testing.T,
	root recordings.Service,
	recordingID recordings.RecordingID,
	interval time.Duration,
) recordings.RecordingID {
	t.Helper()
	started, err := root.StartRecording(recordings.StartRecordingRequest{
		Enabled:       true,
		RecordingID:   recordingID,
		Scope:         recordings.CanonicalEventScope{FactorySessionID: "session-active"},
		Target:        recordings.RecordingTargetRequest{Artifact: "artifact:active"},
		FlushInterval: interval,
	})
	if err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	return started.Status.RecordingID
}

func stopRecording(
	t *testing.T,
	root recordings.Service,
	recordingID recordings.RecordingID,
) {
	t.Helper()
	if _, err := root.StopRecording(recordings.StopRecordingRequest{
		RecordingID: recordingID,
	}); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
}

func recordEvent(
	t *testing.T,
	root recordings.Service,
	recordingID recordings.RecordingID,
	event recordings.CanonicalEvent,
) {
	t.Helper()
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       event,
	}); err != nil {
		t.Errorf("RecordRecordingEvent sequence %d: %v", event.Sequence, err)
	}
}

func activeFlushEvent(sequence int64) recordings.CanonicalEvent {
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-active"}
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID("event-" + time.Unix(sequence, 0).Format("150405")),
		Sequence:   recordings.CanonicalEventSequence(sequence),
		Scope:      scope,
		Kind:       "WORK_REQUEST",
		RecordedAt: time.Unix(sequence, 0).UTC(),
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-active",
			Sequence:           recordings.CanonicalEventSequence(sequence),
		},
		Payload: "{}",
	}
}

func recordingStatus(
	t *testing.T,
	root recordings.Service,
	recordingID recordings.RecordingID,
) recordings.RecordingStatusFacts {
	t.Helper()
	result, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	return result.Status
}

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for asynchronous recording lifecycle state")
		}
		time.Sleep(time.Millisecond)
	}
}
