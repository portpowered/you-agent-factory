package inference_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/events"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWSRFT007WorkerRecordingHealthMatrix proves the customer-visible
// recording-health projection at the root-composed Worker recording boundary.
// A legal terminal remains COMPLETE for successful and failed execution, a
// post-handoff capture failure is DEGRADED when the terminal fact is durable,
// and a terminal-less durable prefix remains INCOMPLETE when its degradation
// marker cannot be persisted. The public codec cases cover canceled and
// terminated terminal outcomes without invoking a provider.
//
// WSR-FT-007: COMPLETE/DEGRADED/INCOMPLETE across terminal outcomes, capture
// loss, and missing-terminal histories.
func TestWSRFT007WorkerRecordingHealthMatrix(t *testing.T) {
	tests := []struct {
		name               string
		build              func(*testing.T) recordings.WorkerRecordingSnapshot
		wantStatus         recordings.WorkerRecordingStatus
		wantExecutionPhase workers.Phase
		wantRecords        int
		wantFailure        string
	}{
		{
			name: "successful execution with legal terminal is complete",
			build: func(t *testing.T) recordings.WorkerRecordingSnapshot {
				return runWSRFT007RootRecording(t, "executor_success", 0, 0, false)
			},
			wantStatus:         recordings.WorkerRecordingStatusComplete,
			wantExecutionPhase: workers.PhaseCompleted,
			wantRecords:        -1,
		},
		{
			name: "failed execution with legal terminal is complete",
			build: func(t *testing.T) recordings.WorkerRecordingSnapshot {
				return runWSRFT007RootRecording(t, "executor_failure_no_arcs", 1, 0, false)
			},
			wantStatus:         recordings.WorkerRecordingStatusComplete,
			wantExecutionPhase: workers.PhaseFailed,
			wantRecords:        -1,
		},
		{
			name: "known terminal with lost capture is degraded",
			build: func(t *testing.T) recordings.WorkerRecordingSnapshot {
				return runWSRFT007RootRecording(t, "executor_success", 0, 2, false)
			},
			wantStatus:         recordings.WorkerRecordingStatusDegraded,
			wantExecutionPhase: workers.PhaseCompleted,
			wantRecords:        1,
			wantFailure:        "PERSISTENCE_FAILED",
		},
		{
			name: "terminal-less prefix is incomplete",
			build: func(t *testing.T) recordings.WorkerRecordingSnapshot {
				return runWSRFT007RootRecording(t, "executor_success", 0, 2, true)
			},
			wantStatus:  recordings.WorkerRecordingStatusIncomplete,
			wantRecords: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := test.build(t)
			if len(snapshot.Sessions) != 1 {
				t.Fatalf("durable Worker snapshot = %#v, want one session", snapshot)
			}
			session := snapshot.Sessions[0]
			if session.Status != test.wantStatus {
				t.Fatalf("recording health = %q, want %q; session=%#v", session.Status, test.wantStatus, session)
			}
			if test.wantRecords >= 0 && len(session.Records) != test.wantRecords {
				t.Fatalf("durable records = %d, want %d: %#v", len(session.Records), test.wantRecords, session.Records)
			}
			if session.Failure != test.wantFailure {
				t.Fatalf("recording failure = %q, want %q", session.Failure, test.wantFailure)
			}
			replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
				Snapshot: snapshot,
			})
			if err != nil {
				t.Fatalf("ReplayWorkerRecording() error = %v", err)
			}
			if replayed.Projection.Status != test.wantStatus {
				t.Fatalf("replayed recording health = %q, want %q", replayed.Projection.Status, test.wantStatus)
			}
			if test.wantStatus == recordings.WorkerRecordingStatusIncomplete &&
				replayed.Projection.InterruptionReason != recordings.WorkerRecordingInterruptionProcessStopped {
				t.Fatalf("replayed interruption reason = %q, want %q", replayed.Projection.InterruptionReason, recordings.WorkerRecordingInterruptionProcessStopped)
			}
			if test.wantExecutionPhase == "" {
				if session.ExecutionTerminal != nil {
					t.Fatalf("execution terminal = %#v, want no trustworthy durable terminal", session.ExecutionTerminal)
				}
				return
			}
			if session.ExecutionTerminal == nil || session.ExecutionTerminal.Phase != test.wantExecutionPhase {
				t.Fatalf("execution terminal = %#v, want phase %q", session.ExecutionTerminal, test.wantExecutionPhase)
			}
		})
	}

	assertWSRFT007TerminalOutcomes(t)
}

func assertWSRFT007TerminalOutcomes(t *testing.T) {
	t.Helper()
	for _, test := range []struct {
		name   string
		status string
	}{
		{name: "canceled execution", status: "CANCELED"},
		{name: "terminated execution", status: "TERMINATED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(
				wsrFT007TerminalHistory(t, workers.PhaseCanceled, test.status),
			)
			if err != nil {
				t.Fatalf("ReduceWorkerRecording() error = %v", err)
			}
			if projection.Status != recordings.WorkerRecordingStatusComplete || !projection.Complete {
				t.Fatalf("projection = %#v, want COMPLETE", projection)
			}
			if projection.ExecutionTerminal == nil || projection.ExecutionTerminal.Status != test.status {
				t.Fatalf("execution terminal = %#v, want %q", projection.ExecutionTerminal, test.status)
			}
		})
	}
}

func runWSRFT007RootRecording(
	t *testing.T,
	fixture string,
	exitCode int,
	failPosition events.AggregateSequence,
	failFailureMarker bool,
) recordings.WorkerRecordingSnapshot {
	t.Helper()
	probe := newWSRFT004RecordingProbe(t, false)
	probe.failPosition = failPosition
	probe.failFailureMarker = failFailureMarker
	runner := newWSRFT004ProviderRunner(t, probe)
	dir := wsrFT004FactoryForFixture(t, fixture)
	queueWSRFT004ProviderResult(t, runner, exitCode)
	runSharedInferenceFactory(t, dir, sharedInferenceScenario{
		commandRunner:         runner,
		workerRecordingWriter: probe,
	}, sharedInferenceScenarioTimeout)
	reader := recordings.WorkerRecordingReader(probe)
	recordingID, _ := probe.RecordingIdentity(t)
	snapshot, err := reader.LoadWorkerRecording(t.Context(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(%q) error = %v", recordingID, err)
	}
	return snapshot
}

func wsrFT004FactoryForFixture(t *testing.T, fixture string) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, fixture))
	support.ClearSeedInputs(t, dir)
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	support.WriteAgentConfig(t, dir, "worker", sharedInferenceWithExecutorProvider(
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
		"CODEX",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"WSR-FT-007 recording health"}`))
	return dir
}

func wsrFT007TerminalHistory(t *testing.T, phase workers.Phase, status string) recordings.WorkerRecordingHistory {
	t.Helper()
	const sessionID = "wsr-ft-007-terminal-worker"
	topic := events.Topic("worker-session/" + sessionID + "/events")
	return recordings.WorkerRecordingHistory{
		RecordingID:     recordingIDForWSRFT007(status),
		WorkerSessionID: sessionID,
		Topic:           topic,
		Records: []events.Record{
			wsrFT007OpeningRecord(t, topic, sessionID),
			wsrFT007LifecycleRecord(t, topic, sessionID, 2, 2, "terminal", phase, status),
		},
	}
}

func wsrFT007OpeningRecord(t *testing.T, topic events.Topic, sessionID string) events.Record {
	t.Helper()
	payload, err := json.Marshal(workers.SessionPayload{Status: "STARTING", WorkerSessionID: sessionID})
	if err != nil {
		t.Fatalf("marshal opening payload: %v", err)
	}
	return wsrFT007DraftRecord(t, topic, sessionID, 1, 1, "started", workers.PhaseStarted, payload)
}

func recordingIDForWSRFT007(status string) string {
	return "wsr-ft-007-" + status
}

func wsrFT007LifecycleRecord(
	t *testing.T,
	topic events.Topic,
	sessionID string,
	position events.AggregateSequence,
	sequence events.SourceSequence,
	eventID string,
	phase workers.Phase,
	status string,
) events.Record {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		t.Fatalf("marshal lifecycle payload: %v", err)
	}
	return wsrFT007DraftRecord(t, topic, sessionID, position, sequence, eventID, phase, payload)
}

func wsrFT007DraftRecord(
	t *testing.T,
	topic events.Topic,
	sessionID string,
	position events.AggregateSequence,
	sequence events.SourceSequence,
	eventID string,
	phase workers.Phase,
	payload json.RawMessage,
) events.Record {
	t.Helper()
	draft, err := json.Marshal(workers.Draft{
		Kind: workers.KindSession, Phase: phase,
		Provenance: workers.Provenance{
			Delivery: workers.DeliverySynthesized, Fidelity: workers.FidelityLifecycleOnly,
			NativeEventType: "worker_session_lifecycle", Representation: workers.RepresentationNotification,
		},
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("marshal lifecycle draft: %v", err)
	}
	return events.Record{
		ID:         events.RecordID{Topic: topic, Position: position},
		SourceType: "worker_session_lifecycle", SourceID: events.SourceID(sessionID),
		SourceSequence: sequence, SourceEventID: events.SourceEventID(eventID),
		SchemaID: "workers.draft.v1", Payload: draft,
	}
}
