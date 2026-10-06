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
	for _, cell := range []string{"exact", "failed", "canceled", "terminated", "active", "opening-only", "incomplete", "scope", "attempt", "predecessor", "reference", "workspace", "model"} {
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
			if got := completedContinuationMatches(page, input); got != (cell == "exact" || cell == "failed" || cell == "canceled" || cell == "terminated") {
				t.Fatalf("completed capture match = %v", got)
			}
		})
	}
}

func configureCompletedContinuationEvidence(cell string, page *recordings.WorkerCapturedActivityPage, opening *workers.SessionPayload) {
	switch cell {
	case "failed", "canceled", "terminated", "active":
		page.Terminal.Status = map[string]string{"failed": "FAILED", "canceled": "CANCELED", "terminated": "TERMINATED", "active": "RUNNING"}[cell]
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

func TestContinuationTerminalResultPreservesOutcomeAndRejectsCorruption(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"completed", "failed", "canceled", "terminated", "attempt", "phase", "status", "missing-cause", "unknown-cause", "corrupt"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			state, result, encoded := continuationTerminalFixture(t, cell)
			got, err := continuationTerminalResult(encoded, string(state), "successor-attempt")
			assertContinuationTerminalResult(t, cell, state, result, got, err)
		})
	}
}

func continuationTerminalFixture(t *testing.T, cell string) (workersessions.State, workersessions.TerminalResult, json.RawMessage) {
	t.Helper()
	state := workersessions.StateFailed
	switch cell {
	case "completed":
		state = workersessions.StateCompleted
	case "canceled":
		state = workersessions.StateCanceled
	case "terminated":
		state = workersessions.StateTerminated
	}
	result := workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	if state == workersessions.StateFailed {
		result = workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed,
			Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic, Detail: "recorded failure"}}
	}
	draft, err := terminalDraft(state, result, "successor-attempt")
	if err != nil {
		t.Fatal(err)
	}
	var terminal terminalSessionPayload
	_ = json.Unmarshal(draft.Payload, &terminal)
	switch cell {
	case "attempt":
		draft.DispatchID = "foreign-attempt"
	case "phase":
		draft.Phase = workers.PhaseCompleted
	case "status":
		terminal.Status = "COMPLETED"
	case "missing-cause":
		terminal.FailureCause = ""
	case "unknown-cause":
		terminal.FailureCause = "UNKNOWN"
	}
	draft.Payload, _ = json.Marshal(terminal)
	encoded, _ := json.Marshal(draft)
	if cell == "corrupt" {
		encoded = []byte(`{`)
	}
	return state, result, encoded
}

func assertContinuationTerminalResult(t *testing.T, cell string, state workersessions.State, want workersessions.TerminalResult, got *workersessions.TerminalResult, err error) {
	t.Helper()
	valid := false
	switch cell {
	case "completed", "failed", "canceled", "terminated":
		valid = true
	}
	if !valid {
		if got != nil || !errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) {
			t.Fatalf("unsafe terminal: result=%+v err=%v", got, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if state == workersessions.StateCanceled || state == workersessions.StateTerminated {
		if got != nil {
			t.Fatalf("control outcome invented a result: %+v", got)
		}
		return
	}
	if got == nil || got.Outcome != want.Outcome || got.Validate() != nil {
		t.Fatalf("terminal outcome lost: %+v", got)
	}
	if cell == "failed" && (got.Cause.Kind != want.Cause.Kind || got.Cause.Detail != want.Cause.Detail) {
		t.Fatalf("terminal failure lost: %+v", got)
	}
}
