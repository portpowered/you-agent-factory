package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type archivedCauseStore struct {
	capturedSummaryFake
	testutil.UnavailableWorkerControlStore
	records []recordings.WorkerControlOperationRecord
	listErr error
}

func (f *archivedCauseStore) ListWorkerControlOperations(_ context.Context, _ recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	return f.records, f.listErr
}

func TestTerminalCauseArchivedUsesPhysicalTerminalAttemptAndExactCapture(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"applied", "stale-attempt", "store-loss", "missing-terminal", "wrong-position"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			target := exactCaptureIdentity()
			target.ExpectedAttemptID = "physical-attempt"
			result := workersessions.ControlResult{Session: workersessions.Session{ID: target.WorkerSessionID, State: workersessions.StateTerminated}, Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeApplied, DispatchID: "logical-dispatch"}
			payload, _ := json.Marshal(result)
			fake := &archivedCauseStore{records: []recordings.WorkerControlOperationRecord{{Target: target, Revision: 2, Operation: recordings.WorkerControlOperation{Action: "terminate", Phase: "COMPLETED"}, Result: payload}}}
			fake.snapshot = recordings.WorkerRecordingSnapshot{RecordingID: target.RecordingID, Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: target.WorkerSessionID, Records: []events.Record{{
				ID: events.RecordID{Position: 3}, SourceType: lifecycleSourceType, SourceSequence: terminalSourceSequence, SourceEventID: terminalSourceEventID,
				Payload: []byte(`{"kind":"SESSION","phase":"CANCELED","dispatchId":"physical-attempt","payload":{"status":"TERMINATED"}}`),
			}}}}}
			page := recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID, RecordingGenerationID: target.RecordingGenerationID, OwnerEpoch: target.OwnerEpoch}, Terminal: &recordings.WorkerRecordingTerminal{Position: 3, Status: "TERMINATED"}, Health: recordings.WorkerRecordingStatusComplete,
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
