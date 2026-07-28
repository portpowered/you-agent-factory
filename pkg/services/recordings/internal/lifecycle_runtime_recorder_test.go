package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/replay"
)

type runtimeRecorderTestClock struct{ now time.Time }

func (clock runtimeRecorderTestClock) Now() time.Time { return clock.now }

func TestLifecycleRuntimeRecorderUsesComposedRootForBindingFailuresAndFinalization(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	producerErr := errors.New("producer failed")
	finalWriteErr := errors.New("final write failed")
	writeCount := 0
	writer := func(string, recordings.RecordingSnapshot) error {
		writeCount++
		if writeCount == 2 {
			return finalWriteErr
		}
		return nil
	}
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		writer,
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, "recording.json")
	recorder.RecordError(producerErr)
	recorder.RecordEvent(factorydefinitions.FactoryEvent{
		Id:   "runtime-event",
		Type: factorydefinitions.FactoryEventTypeWorkRequest,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: startedAt.Add(time.Second),
		},
		Payload: []byte(`{"workId":"work-1"}`),
	})
	scope := recordings.CanonicalEventScope{FactorySessionID: "~default"}
	if err := recorder.BindRecordingService(root, scope); err != nil {
		t.Fatalf("BindRecordingService: %v", err)
	}

	if err := recorder.Flush(); err != nil {
		t.Fatalf("initial Flush: %v", err)
	}
	err := recorder.Finalize(finishedAt)
	if !errors.Is(err, producerErr) || !errors.Is(err, finalWriteErr) {
		t.Fatalf("Finalize error = %v, want producer and final-write identities", err)
	}

	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recorder.recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	assertFailedRuntimeRecordingStatus(t, status.Status, scope, finishedAt)
}

func assertFailedRuntimeRecordingStatus(
	t *testing.T,
	status recordings.RecordingStatusFacts,
	scope recordings.CanonicalEventScope,
	finishedAt time.Time,
) {
	t.Helper()
	if status.Artifact != "recording.json" ||
		status.Scope != scope ||
		status.State != recordings.RecordingFailed ||
		status.AcceptedEvents != 3 ||
		len(status.Failures) != 2 ||
		status.Failures[0].Code != "producer_boundary_failed" ||
		status.Failures[1].Code != "final_flush_failed" ||
		status.FinalizedAt == nil ||
		!status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("composed lifecycle status = %#v", status)
	}
}

func TestReplayRecordingSnapshotWriterPreservesReplayCompatibility(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	path := filepath.Join(t.TempDir(), "recording.json")
	storage := platformreplay.NewLocal(runtime.GOOS)
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		NewReplayRecordingSnapshotWriter(storage.WriteFile),
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, path)
	if err := recorder.BindRecordingService(root, recordings.CanonicalEventScope{
		FactorySessionID: "~default",
	}); err != nil {
		t.Fatalf("BindRecordingService: %v", err)
	}
	if err := recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	artifact, err := replay.Load(
		storage,
		path,
		func(data []byte) (*factorydefinitions.FactorySnapshot, error) {
			return factorydefinitions.NewFactorySnapshot(
				map[string]any{"id": "factory-from-decoder"},
			)
		},
	)
	if err != nil {
		payload, _ := os.ReadFile(path)
		t.Fatalf("Load replay-compatible lifecycle recording: %v\n%s", err, payload)
	}
	if artifact.WallClock == nil ||
		!artifact.WallClock.StartedAt.Equal(startedAt) ||
		!artifact.WallClock.FinishedAt.Equal(finishedAt) ||
		len(artifact.Events) != 2 {
		t.Fatalf(
			"replay artifact wall clock = %#v, events = %d",
			artifact.WallClock,
			len(artifact.Events),
		)
	}
}

func newLifecycleRecorderForTest(
	t *testing.T,
	startedAt time.Time,
	path string,
) *lifecycleRuntimeRecorder {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "factory-1",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	value, err := NewLifecycleRuntimeRecorder(
		time.Hour,
		nil,
		func() time.Time { return startedAt },
		path,
		func(
			factorydefinitions.FactorySnapshotSource,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return snapshot, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLifecycleRuntimeRecorder: %v", err)
	}
	recorder, ok := value.(*lifecycleRuntimeRecorder)
	if !ok {
		t.Fatalf("recorder type = %T", value)
	}
	return recorder
}

func TestLifecycleRuntimeRecorderStopAndIdempotentRecordEvent(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		nil,
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, "recording-stop.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-stop"}
	if err := recorder.BindRecordingService(root, scope); err != nil {
		t.Fatalf("BindRecordingService: %v", err)
	}
	event := factorydefinitions.FactoryEvent{
		Id:   "dup-event",
		Type: factorydefinitions.FactoryEventTypeWorkRequest,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: startedAt,
		},
		Payload: []byte(`{}`),
	}
	recorder.Start(context.Background())
	recorder.RecordEvent(event)
	recorder.RecordEvent(event)
	recorder.Stop()
	recorder.Finish(startedAt.Add(time.Minute))
}
