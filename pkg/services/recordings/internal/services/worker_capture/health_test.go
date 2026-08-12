package worker_capture

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestReduceWorkerRecordingHealthMatrix(t *testing.T) {
	tests := []struct {
		name   string
		phase  workers.Phase
		status string
	}{
		{name: "completed", phase: workers.PhaseCompleted, status: "COMPLETED"},
		{name: "failed", phase: workers.PhaseFailed, status: "FAILED"},
		{name: "canceled", phase: workers.PhaseCanceled, status: "CANCELED"},
		{name: "terminated", phase: workers.PhaseCanceled, status: "TERMINATED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const sessionID = "health-matrix-worker"
			topic := canonicalWorkerTopic(sessionID)
			records := []events.Record{
				workerRecord(t, topic, 1, "worker_session_lifecycle", sessionID, 1, "started", workers.Draft{
					Kind: workers.KindSession, Phase: workers.PhaseStarted,
					Provenance: lifecycleProvenanceForTest("codex"),
					Payload:    mustJSON(t, workers.SessionPayload{Status: "STARTING", WorkerSessionID: sessionID}),
				}),
				workerRecord(t, topic, 2, "worker_session_lifecycle", sessionID, 2, "terminal", workers.Draft{
					Kind: workers.KindSession, Phase: test.phase,
					Provenance: lifecycleProvenanceForTest("codex"),
					Payload:    mustJSON(t, map[string]string{"status": test.status}),
				}),
			}

			projection, err := (WorkerRecordingCodec{}).ReduceWorkerRecording(WorkerRecordingHistory{
				RecordingID: "health-matrix-recording", WorkerSessionID: sessionID,
				Topic: topic, Records: records,
			})
			if err != nil {
				t.Fatalf("ReduceWorkerRecording() error = %v", err)
			}
			if projection.Status != WorkerRecordingStatusComplete || !projection.Complete {
				t.Fatalf("projection = %#v, want COMPLETE", projection)
			}
			if projection.ExecutionTerminal == nil || projection.ExecutionTerminal.Phase != test.phase {
				t.Fatalf("execution terminal = %#v, want %q", projection.ExecutionTerminal, test.phase)
			}
		})
	}
}

func TestReduceWorkerRecordingClassifiesLossAndMissingTerminal(t *testing.T) {
	const sessionID = "health-loss-worker"
	topic := canonicalWorkerTopic(sessionID)
	opening := workerRecord(t, topic, 1, "worker_session_lifecycle", sessionID, 1, "started", workers.Draft{
		Kind: workers.KindSession, Phase: workers.PhaseStarted,
		Provenance: lifecycleProvenanceForTest("codex"),
		Payload:    mustJSON(t, workers.SessionPayload{Status: "STARTING", WorkerSessionID: sessionID}),
	})

	t.Run("missing terminal is incomplete", func(t *testing.T) {
		projection, err := (WorkerRecordingCodec{}).ReduceWorkerRecording(WorkerRecordingHistory{
			RecordingID: "health-incomplete-recording", WorkerSessionID: sessionID,
			Topic: topic, Records: []events.Record{opening},
		})
		if err != nil {
			t.Fatalf("ReduceWorkerRecording() error = %v", err)
		}
		if projection.Status != WorkerRecordingStatusIncomplete || projection.Complete || projection.ExecutionTerminal != nil {
			t.Fatalf("projection = %#v, want INCOMPLETE without terminal truth", projection)
		}
	})

	t.Run("known terminal with capture loss is degraded", func(t *testing.T) {
		terminal := &WorkerRecordingTerminal{Phase: workers.PhaseCompleted, Status: "COMPLETED"}
		projection, err := (WorkerRecordingCodec{}).ReduceWorkerRecording(WorkerRecordingHistory{
			RecordingID: "health-degraded-recording", WorkerSessionID: sessionID,
			Topic: topic, Failure: "PERSISTENCE_FAILED", ExecutionTerminal: terminal,
			Records: []events.Record{opening},
		})
		if err != nil {
			t.Fatalf("ReduceWorkerRecording() error = %v", err)
		}
		if projection.Status != WorkerRecordingStatusDegraded || projection.Complete {
			t.Fatalf("projection = %#v, want DEGRADED", projection)
		}
		if projection.Terminal != nil || projection.ExecutionTerminal == nil || projection.Degradation != "PERSISTENCE_FAILED" {
			t.Fatalf("projection evidence = %#v, want loss marker and authoritative terminal without fabricated record", projection)
		}
	})

	t.Run("recorded terminal plus loss marker is degraded", func(t *testing.T) {
		terminalRecord := workerRecord(t, topic, 2, "worker_session_lifecycle", sessionID, 2, "terminal", workers.Draft{
			Kind: workers.KindSession, Phase: workers.PhaseCompleted,
			Provenance: lifecycleProvenanceForTest("codex"),
			Payload:    mustJSON(t, map[string]string{"status": "COMPLETED"}),
		})
		projection, err := (WorkerRecordingCodec{}).ReduceWorkerRecording(WorkerRecordingHistory{
			RecordingID: "health-degraded-recording", WorkerSessionID: sessionID,
			Topic: topic, Failure: "FINALIZATION_FAILED", Records: []events.Record{opening, terminalRecord},
		})
		if err != nil {
			t.Fatalf("ReduceWorkerRecording() error = %v", err)
		}
		if projection.Status != WorkerRecordingStatusDegraded || projection.ExecutionTerminal == nil || projection.Terminal == nil {
			t.Fatalf("projection = %#v, want DEGRADED with recorded terminal evidence", projection)
		}
	})
}

func TestReplayWorkerRecordingMapsLegacyStatesAndRejectsUnknownState(t *testing.T) {
	const sessionID = "health-compat-worker"
	topic := canonicalWorkerTopic(sessionID)
	opening := workerRecord(t, topic, 1, "worker_session_lifecycle", sessionID, 1, "started", workers.Draft{
		Kind: workers.KindSession, Phase: workers.PhaseStarted,
		Provenance: lifecycleProvenanceForTest("codex"),
		Payload:    mustJSON(t, workers.SessionPayload{Status: "STARTING", WorkerSessionID: sessionID}),
	})
	terminal := workerRecord(t, topic, 2, "worker_session_lifecycle", sessionID, 2, "terminal", workers.Draft{
		Kind: workers.KindSession, Phase: workers.PhaseCompleted,
		Provenance: lifecycleProvenanceForTest("codex"),
		Payload:    mustJSON(t, map[string]string{"status": "COMPLETED"}),
	})

	legacyComplete, err := (WorkerRecordingCodec{}).ReplayWorkerRecording(WorkerRecordingReplayRequest{
		Snapshot: WorkerRecordingSnapshot{RecordingID: "legacy-complete", Sessions: []WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID, Topic: topic, Status: WorkerRecordingStatusCompleted,
			Records: []events.Record{opening, terminal},
		}}},
	})
	if err != nil {
		t.Fatalf("legacy completed replay error = %v", err)
	}
	if legacyComplete.Projection.Status != WorkerRecordingStatusComplete {
		t.Fatalf("legacy completed projection = %#v, want COMPLETE", legacyComplete.Projection)
	}

	legacyFailed, err := (WorkerRecordingCodec{}).ReplayWorkerRecording(WorkerRecordingReplayRequest{
		Snapshot: WorkerRecordingSnapshot{RecordingID: "legacy-failed", Sessions: []WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID, Topic: topic, Status: WorkerRecordingStatusFailed,
			Records: []events.Record{opening, terminal},
		}}},
	})
	if err != nil {
		t.Fatalf("legacy failed replay error = %v", err)
	}
	if legacyFailed.Projection.Status != WorkerRecordingStatusDegraded {
		t.Fatalf("legacy failed projection = %#v, want DEGRADED", legacyFailed.Projection)
	}

	_, err = (WorkerRecordingCodec{}).ReplayWorkerRecording(WorkerRecordingReplayRequest{
		Snapshot: WorkerRecordingSnapshot{RecordingID: "unknown-status", Sessions: []WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID, Topic: topic, Status: WorkerRecordingStatus("MYSTERY"),
			Records: []events.Record{opening},
		}}},
	})
	if !errors.Is(err, ErrWorkerRecordingCompatibility) {
		t.Fatalf("unknown status error = %v, want compatibility classification", err)
	}
}
