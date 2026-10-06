package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type archivedCauseStore struct {
	capturedSummaryFake
	unavailableWorkerControlStore
	records []recordings.WorkerControlOperationRecord
	listErr error
}

func (f *archivedCauseStore) ListWorkerControlOperations(_ context.Context, _ recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	return f.records, f.listErr
}

func TestTerminalCauseArchivedUsesPhysicalTerminalAttemptAndExactCapture(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"applied", "stale-attempt", "store-loss", "missing-terminal", "wrong-position", "unknown-field", "aliased-id", "duplicate-id", "terminal-duplicate-attempt", "terminal-aliased-attempt", "terminal-unknown-field", "terminal-duplicate-status", "terminal-wrong-status", "terminal-wrong-phase"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			target := exactCaptureIdentity()
			target.ExpectedAttemptID = "physical-attempt"
			result := workersessions.ControlResult{Session: workersessions.Session{ID: target.WorkerSessionID, State: workersessions.StateTerminated}, Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeApplied, DispatchID: "logical-dispatch"}
			payload, _ := json.Marshal(result)
			fake := &archivedCauseStore{records: []recordings.WorkerControlOperationRecord{{Target: target, Revision: 2, Operation: recordings.WorkerControlOperation{Version: 1, WorkerSessionID: target.WorkerSessionID, ExpectedAttemptID: target.ExpectedAttemptID, Action: "terminate", Phase: "COMPLETED"}, Result: payload}}}
			fake.snapshot = recordings.WorkerRecordingSnapshot{RecordingID: target.RecordingID, Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: target.WorkerSessionID, Records: []events.Record{{
				ID: events.RecordID{Position: 3}, SourceType: lifecycleSourceType, SourceSequence: terminalSourceSequence, SourceEventID: terminalSourceEventID,
				Payload: []byte(`{"kind":"SESSION","phase":"CANCELED","dispatchId":"physical-attempt","payload":{"status":"TERMINATED"}}`),
			}}}}}
			page := recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{FactorySessionID: target.FactorySessionID, CommittedPosition: 3, RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID, RecordingGenerationID: target.RecordingGenerationID, OwnerEpoch: target.OwnerEpoch}, Terminal: &recordings.WorkerRecordingTerminal{Position: 3, Status: "TERMINATED"}, Health: recordings.WorkerRecordingStatusComplete,
				Opening: events.Record{Payload: []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"worker","attemptId":"provider-association-attempt","factorySessionId":"factory"}}`)},
			}
			switch variant {
			case "stale-attempt":
				fake.records[0].Target.ExpectedAttemptID = "logical-dispatch"
			case "store-loss":
				fake.listErr = errors.New("unavailable")
			case "missing-terminal":
				fake.snapshot.Sessions[0].Records = nil
			case "wrong-position":
				page.Terminal.Position = 2
			case "unknown-field":
				fake.records[0].Result = append([]byte(`{"private":"private-stop-detail",`), payload[1:]...)
			case "aliased-id":
				fake.records[0].Result = []byte(strings.Replace(string(payload), `"ID":`, `"id":`, 1))
			case "duplicate-id":
				fake.records[0].Result = []byte(strings.Replace(string(payload), `"ID":`, `"ID":"foreign","ID":`, 1))
			}
			if strings.HasPrefix(variant, "terminal-") {
				fake.snapshot.Sessions[0].Records[0].Payload = corruptArchivedTerminal(variant)
			}
			reader := &LogReader{reader: fake}
			fake.page = page
			observation, err := reader.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: target.WorkerSessionID, FactorySessionID: target.FactorySessionID})
			if err != nil {
				t.Fatal(err)
			}
			cause := observation.TerminalCause
			if variant == "applied" {
				if cause == nil || *cause != "OPERATOR_TERMINATE" {
					t.Fatalf("archived cause=%v", cause)
				}
			} else if cause != nil {
				t.Fatalf("invalid archived evidence claimed %s", *cause)
			}
		})
	}
}

func corruptArchivedTerminal(variant string) []byte {
	payload := `{"kind":"SESSION","phase":"CANCELED","dispatchId":"physical-attempt","payload":{"status":"TERMINATED"}}`
	switch variant {
	case "terminal-duplicate-attempt":
		payload = strings.Replace(payload, `"dispatchId":`, `"dispatchId":"foreign","dispatchId":`, 1)
	case "terminal-aliased-attempt":
		payload = strings.Replace(payload, `"dispatchId":`, `"DispatchId":`, 1)
	case "terminal-unknown-field":
		payload = strings.Replace(payload, `"kind":`, `"private":"private-terminal-detail","kind":`, 1)
	case "terminal-duplicate-status":
		payload = strings.Replace(payload, `"status":`, `"status":"CANCELED","status":`, 1)
	case "terminal-wrong-status":
		payload = strings.Replace(payload, `"TERMINATED"`, `"CANCELED"`, 1)
	case "terminal-wrong-phase":
		payload = strings.Replace(payload, `"CANCELED"`, `"COMPLETED"`, 1)
	}
	return []byte(payload)
}
