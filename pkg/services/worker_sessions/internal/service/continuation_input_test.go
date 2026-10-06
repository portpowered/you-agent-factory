package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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

// The archive reader's collaborators supply detached capture facts; these
// cells prove reservation and refusal without an executor or application graph.
func TestContinuationArchivedSourceReservation(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"captured", "incomplete", "wrong-scope", "wrong-attempt", "wrong-terminal", "missing-recipe", "unknown", "successor"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req := continuationReservationRequest()
			r := newContinuationSource(t, req)
			ref := r.sessions[req.SourceWorkerSessionID].ProviderSessionAssociation.Reference
			delete(r.sessions, req.SourceWorkerSessionID)
			item := historyCapture(t, req.SourceWorkerSessionID, "", "dispatch-1", true)
			item.Catalog.RecordingID, item.Catalog.RecordingGenerationID, item.Catalog.OwnerEpoch = "recording", "generation", "owner"
			reader := &capturedActivityFake{page: recordings.WorkerCapturedActivityPage{
				Catalog: item.Catalog, Opening: item.Opening, Terminal: item.Terminal, Health: item.Health,
			}}
			r.logs = &LogReader{reader: reader}
			store := &restartRecipeStore{execution: continuationValidExecution("dispatch-1"), reference: ref}
			want := configureArchivedContinuationCell(cell, reader, store)
			r.restart = &retainedContinuationStore{restartRecipeStore: *store, readErr: os.ErrNotExist}
			replay, owner, err := r.reserveContinuation(req)
			if want != nil {
				if !errors.Is(err, want) || replay != nil || owner || len(r.sessions) != 0 || len(r.supervisions) != 0 {
					t.Fatalf("archive refusal changed registry: replay=%v owner=%v err=%v", replay, owner, err)
				}
				return
			}
			if err != nil || !owner || !replay.plan.archived || !replay.plan.direct ||
				replay.plan.execution.Execution.Continuation.ProviderSessionID != ref.ID || len(r.supervisions) != 0 {
				t.Fatalf("archive reservation lost identity or restored supervision: replay=%+v err=%v", replay, err)
			}
			if _, exists := r.sessions[req.SourceWorkerSessionID]; exists {
				t.Fatal("historical source became a live registry session")
			}
		})
	}
}

func TestContinuationCompletedCaptureRejectsMismatchedEvidence(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "opening-only", "incomplete", "scope", "attempt", "predecessor", "reference", "workspace", "model"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			_, plan, target := retainedContinuationFixture(t)
			payload, err := encodeContinuationInput(plan, target)
			if err != nil {
				t.Fatal(err)
			}
			input, err := decodeContinuationInput(payload, plan.request, target)
			if err != nil {
				t.Fatal(err)
			}
			opening := openingSessionPayload(plan.request.SuccessorWorkerSessionID, plan.execution.Execution.Dispatch.DispatchID,
				newContinuationSource(t, plan.request).clock.Now(), plan.execution.Execution, &workers.SessionLineage{
					PredecessorWorkerSessionID: target.WorkerSessionID, PreviousAttemptID: target.ExpectedAttemptID, PreviousDispatchID: target.ExpectedAttemptID,
				})
			page := recordings.WorkerCapturedActivityPage{
				Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: plan.request.SuccessorWorkerSessionID, FactorySessionID: target.FactorySessionID},
				Health:  recordings.WorkerRecordingStatusComplete, Terminal: &recordings.WorkerRecordingTerminal{Status: "COMPLETED"},
			}
			configureCompletedContinuationEvidence(cell, &page, &opening)
			draftPayload, _ := json.Marshal(opening)
			page.Opening.Payload, _ = json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: draftPayload})
			if got := completedContinuationMatches(page, input); got != (cell == "exact") {
				t.Fatalf("completed capture match = %v", got)
			}
		})
	}
}

func configureCompletedContinuationEvidence(cell string, page *recordings.WorkerCapturedActivityPage, opening *workers.SessionPayload) {
	switch cell {
	case "opening-only":
		page.Terminal = nil
	case "incomplete":
		page.Health = recordings.WorkerRecordingStatusIncomplete
	case "scope":
		page.Catalog.FactorySessionID = "foreign"
	case "attempt":
		opening.AttemptID = "foreign-attempt"
	case "predecessor":
		opening.Lineage.PredecessorWorkerSessionID = "foreign-source"
	case "reference":
		opening.Continuation.ID = "foreign-reference"
	case "workspace":
		opening.WorkingDirectory = "foreign-workspace"
	case "model":
		opening.Model = "foreign-model"
	}
}

func configureArchivedContinuationCell(cell string, reader *capturedActivityFake, store *restartRecipeStore) error {
	switch cell {
	case "captured":
		return nil
	case "incomplete":
		reader.page.Health = recordings.WorkerRecordingStatusIncomplete
	case "wrong-scope":
		reader.page.Catalog.FactorySessionID = "foreign"
	case "wrong-attempt":
		store.execution.Execution.Dispatch.DispatchID = "foreign-attempt"
	case "wrong-terminal":
		reader.page.Terminal = &recordings.WorkerRecordingTerminal{Status: "CANCELED"}
	case "missing-recipe":
		store.err = os.ErrNotExist
	case "unknown":
		reader.err = os.ErrNotExist
		return workersessions.ErrContinuationSourceNotFound
	case "successor":
		reader.page.SuccessorWorkerSessionID = "already-admitted"
		return workersessions.ErrContinuationSourceConflict
	}
	return workersessions.ErrContinuationExecutionUnavailable
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
