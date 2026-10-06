package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type retainedContinuationStore struct {
	restartRecipeStore
	payload json.RawMessage
	readErr error
}

func (s *retainedContinuationStore) ReadWorkerContinuationInput(context.Context, recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	return s.payload, s.readErr
}

// The admission driver observes retained input through its storage collaborator.
// No executor is installed: every retained or unreadable tuple must return
// before invocation preparation, even when this host has no replay cache.
func TestContinuationRetainedInputNeverRepeatsUncertainAdmission(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "changed-input", "changed-successor", "corrupt", "empty", "read-failure"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			r, plan, target := retainedContinuationFixture(t)
			payload, err := encodeContinuationInput(plan, target)
			if err != nil {
				t.Fatal(err)
			}
			store := &retainedContinuationStore{payload: payload}
			r.restart = store
			writes := &interruptInputStore{}
			r.operations = writes
			want := workersessions.ErrContinuationExecutionUnavailable
			switch cell {
			case "changed-input":
				plan.request.FollowUpInput = "changed input"
				plan.execution.Execution.UserMessage = plan.request.FollowUpInput
				want = workersessions.ErrContinuationRequestIDConflict
			case "changed-successor":
				plan.request.SuccessorWorkerSessionID = "other-successor"
				plan.execution.Execution.Dispatch.DispatchID = continuationDispatchID(target.ExpectedAttemptID, "other-successor")
				want = workersessions.ErrContinuationRequestIDConflict
			case "corrupt":
				store.payload = json.RawMessage(`{"version":1}`)
				want = recordings.ErrWorkerRecordingPersistence
			case "empty":
				// A successful read with no bytes is corruption, not absence.
				store.payload = nil
				want = recordings.ErrWorkerRecordingPersistence
			case "read-failure":
				store.readErr = errors.New("private-storage-path-and-secret")
				want = recordings.ErrWorkerRecordingPersistence
			}
			_, err = r.continueReserved(plan)
			if !errors.Is(err, want) || len(writes.input) != 0 || len(r.continueReplays) != 0 {
				t.Fatalf("retained input reached admission: error=%v persisted=%s", err, writes.input)
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatalf("storage diagnostic leaked: %v", err)
			}
		})
	}
}

func retainedContinuationFixture(t *testing.T) (*registry, continuePlan, recordings.WorkerControlTarget) {
	t.Helper()
	req := continuationReservationRequest()
	r := newContinuationSource(t, req)
	target := recordings.WorkerControlTarget{
		WorkerSessionID: req.SourceWorkerSessionID, RecordingID: "recording",
		RecordingGenerationID: "generation", OwnerEpoch: "owner", ExpectedAttemptID: "dispatch-1",
	}
	capture := target
	capture.ExpectedAttemptID = ""
	r.publications[req.SourceWorkerSessionID] = &publication{capture: capture}
	r.logs = &LogReader{reader: &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
		WorkerSessionID: target.WorkerSessionID, RecordingID: target.RecordingID,
		RecordingGenerationID: target.RecordingGenerationID, OwnerEpoch: target.OwnerEpoch,
	}}}
	ref := r.sessions[req.SourceWorkerSessionID].ProviderSessionAssociation.Reference
	plan := continuePlan{
		request: req, direct: true, lineage: &workers.SessionLineage{PreviousAttemptID: target.ExpectedAttemptID},
		execution: continuationExecution(continuationValidExecution(target.ExpectedAttemptID),
			continuationDispatchID(target.ExpectedAttemptID, req.SuccessorWorkerSessionID), req.FollowUpInput, ref),
	}
	return r, plan, target
}
