package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type retainedContinuationStore struct {
	restartRecipeStore
	payload json.RawMessage
	readErr error
}

// The storage collaborator has committed the bytes but lost its response.
// Readback stays independent so disputed or unavailable facts fail closed.
func TestContinuationInputLostAcknowledgementRequiresExactReadback(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "missing", "corrupt", "changed", "read-failure", "conflict"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			r, plan, target := retainedContinuationFixture(t)
			payload, err := encodeContinuationInput(plan, target)
			if err != nil {
				t.Fatal(err)
			}
			store := &retainedContinuationStore{payload: payload}
			writer := &interruptInputStore{writeErr: errors.New("private-write-acknowledgement")}
			switch cell {
			case "missing":
				store.readErr = os.ErrNotExist
			case "corrupt":
				store.payload = json.RawMessage(`{}`)
			case "changed":
				changed := plan
				changed.request.FollowUpInput = "different"
				changed.execution.Execution.UserMessage = "different"
				store.payload, err = encodeContinuationInput(changed, target)
				if err != nil {
					t.Fatal(err)
				}
			case "read-failure":
				store.readErr = errors.New("private-read-detail")
			case "conflict":
				writer.writeErr = recordings.ErrWorkerControlConflict
			}
			r.operations, r.restart = writer, store
			key := recordings.WorkerControlOperationKey{RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID,
				FactorySessionID: target.FactorySessionID, RequestID: "continue/" + plan.request.RequestID}
			err = r.syncContinuationInput(t.Context(), key, plan.request, target, payload)
			if cell == "exact" {
				if err != nil {
					t.Fatalf("exact committed readback refused: %v", err)
				}
				if err := r.requireNewContinuationInput(t.Context(), key, plan.request, target); !errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) {
					t.Fatalf("later retry reused input as admission authority: %v", err)
				}
				return
			}
			want := recordings.ErrWorkerRecordingPersistence
			if cell == "conflict" {
				want = workersessions.ErrContinuationRequestIDConflict
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "private") {
				t.Fatalf("disputed readback was accepted or leaked diagnostics: %v", err)
			}
		})
	}
}

// The archive reader's collaborators supply detached capture facts; these
// cells prove reservation and refusal without an executor or application graph.
func TestContinuationArchivedSourceReservation(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"captured", "selected-foreign-owner", "incomplete", "wrong-scope", "wrong-attempt", "wrong-terminal", "missing-recipe", "unknown", "successor", "unsupported", "policy-error"} {
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
			if cell == "selected-foreign-owner" {
				req.FactorySessionID = "foreign-factory"
				want = workersessions.ErrContinuationSourceNotFound
			}
			r.continuationSupport = archivedContinuationSupport(cell)
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

func archivedContinuationSupport(cell string) *interruptContinuationSupportFake {
	support := &interruptContinuationSupportFake{supported: cell != "unsupported"}
	if cell == "policy-error" {
		support.err = providers.ErrUnknownProvider
	}
	return support
}

// A terminal Runtime-owned source has a control handle, not a direct
// supervision. Captured facts must agree with it before direct reservation.
func TestContinuationLiveFactoryCaptureFencesAdmission(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "reference", "terminal", "control-pending", "journal-pending", "persistence-lost"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req := continuationReservationRequest()
			r := newContinuationSource(t, req)
			delete(r.supervisions, req.SourceWorkerSessionID)
			source := r.sessions[req.SourceWorkerSessionID]
			captured := &archivedContinuationSource{snapshot: continuationSourceSnapshot{
				session: cloneSession(source), execution: continuationValidExecution("dispatch-1"),
				dispatchID: "dispatch-1", direct: true, archived: true,
			}}
			attempt := &runtimeAttempt{}
			r.runtimeAttemptControls = map[string]*runtimeAttempt{req.SourceWorkerSessionID: attempt}
			switch cell {
			case "reference":
				captured.snapshot.session.ProviderSessionAssociation.Reference.ID = "foreign"
			case "terminal":
				captured.snapshot.session.State = workersessions.StateCanceled
			case "control-pending":
				attempt.controlPending = true
			case "journal-pending":
				attempt.forceJournalPending = 1
			case "persistence-lost":
				attempt.controlPersistenceLost = true
			}
			snapshot, err := r.continuationSnapshotLocked(req, captured)
			if cell == "exact" {
				if err != nil || !snapshot.direct || snapshot.executor == nil || snapshot.address != req.SourceWorkerSessionID {
					t.Fatalf("live Factory source lost direct admission: %+v %v", snapshot, err)
				}
			} else {
				want := workersessions.ErrContinuationSourceConflict
				if cell == "reference" || cell == "terminal" {
					want = workersessions.ErrContinuationProviderSessionInvalid
				}
				if !errors.Is(err, want) {
					t.Fatalf("unsafe Factory capture: got %v want %v", err, want)
				}
			}
			if len(r.sessions) != 1 || len(r.supervisions) != 0 {
				t.Fatal("snapshot created a successor or restored source supervision")
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
	case "unsupported", "policy-error":
		return workersessions.ErrContinuationProviderSessionInvalid
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
	for _, cell := range []string{"completed", "failed", "canceled", "terminated", "attempt", "phase", "status", "missing-cause", "unknown-cause", "publication-failure", "corrupt"} {
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
	case "publication-failure":
		terminal.FailureCause = string(workersessions.FailureCauseEventPublicationFailure)
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

// The continuation reservation component selects one immutable source before
// opening a successor. Collaborators here are detached execution facts only.
func TestContinuationScopedReservationPreservesOwnerAndPublicLineage(t *testing.T) {
	t.Parallel()
	req := continuationReservationRequest()
	r := newContinuationSource(t, req)
	source := r.sessions[req.SourceWorkerSessionID]
	delete(r.sessions, req.SourceWorkerSessionID)
	for _, owner := range []string{"factory-a", "factory-b"} {
		address := scopedWorkerAddress(source.ID, owner)
		session := source.Clone()
		session.ProviderSessionAssociation.Reference.ID = "provider-" + owner
		r.sessions[address] = session
		execution := continuationValidExecution("dispatch-1")
		execution.Execution.FactorySessionID = owner
		r.supervisions[address] = newSupervision("dispatch-1", "turn-1", execution)
		r.observations[address] = &observation{factorySessionID: owner}
	}
	if _, err := r.Continue(t.Context(), req); !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) {
		t.Fatalf("unscoped continuation = %v, want ambiguity", err)
	}
	if _, err := r.Get(t.Context(), workersessions.GetRequest{ID: req.SuccessorWorkerSessionID}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("ambiguous continuation opened successor: %v", err)
	}
	req.FactorySessionID = "factory-missing"
	if _, _, err := r.reserveContinuation(req); !errors.Is(err, workersessions.ErrContinuationSourceNotFound) {
		t.Fatalf("foreign scope = %v, want source not found", err)
	}
	req.FactorySessionID = "factory-a"
	replay, owner, err := r.reserveContinuation(req)
	if err != nil || !owner {
		t.Fatalf("selected reservation = %v, owner=%v", err, owner)
	}
	if replay.plan.execution.Execution.FactorySessionID != "factory-a" ||
		replay.plan.execution.Execution.Continuation.ProviderSessionID != "provider-factory-a" {
		t.Fatalf("selected execution uses peer identity: %+v", replay.plan.execution)
	}
	again, owner, err := r.reserveContinuation(req)
	if err != nil || owner || again != replay {
		t.Fatalf("identical reservation did not replay: owner=%v error=%v", owner, err)
	}
	foreign := req
	foreign.FactorySessionID = "factory-b"
	if _, _, err := r.reserveContinuation(foreign); !errors.Is(err, workersessions.ErrContinuationRequestIDConflict) {
		t.Fatalf("request ID reused across owners = %v, want conflict", err)
	}
	assertScopedContinuationLineage(t, r, req, replay.plan)
	r.finishStart()
}

func assertScopedContinuationLineage(t *testing.T, r *registry, req workersessions.ContinueRequest, plan continuePlan) {
	t.Helper()
	r.commitContinuationSessionLinks(plan, plan.sourceAddressOrID())
	selected, err := r.Get(t.Context(), workersessions.GetRequest{ID: req.SourceWorkerSessionID, FactorySessionID: "factory-a"})
	if err != nil || selected.SuccessorWorkerSessionID != req.SuccessorWorkerSessionID {
		t.Fatalf("selected source lineage = %+v, %v", selected, err)
	}
	peer, err := r.Get(t.Context(), workersessions.GetRequest{ID: req.SourceWorkerSessionID, FactorySessionID: "factory-b"})
	if err != nil || peer.SuccessorWorkerSessionID != "" || peer.ProviderSessionAssociation.Reference.ID != "provider-factory-b" {
		t.Fatalf("peer changed = %+v, %v", peer, err)
	}
	successor, err := r.Get(t.Context(), workersessions.GetRequest{ID: req.SuccessorWorkerSessionID})
	if err != nil || successor.PredecessorWorkerSessionID != req.SourceWorkerSessionID {
		t.Fatalf("public successor lineage = %+v, %v", successor, err)
	}
}

func TestContinuationInputScopeMatchesPersistedTarget(t *testing.T) {
	t.Parallel()
	_, plan, target := retainedContinuationFixture(t)
	target.FactorySessionID = "factory-a"
	plan.execution.Execution.FactorySessionID = target.FactorySessionID
	payload, err := encodeContinuationInput(plan, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "factory-a", "factory-b"} {
		req := plan.request
		req.FactorySessionID = scope
		_, err := decodeContinuationInput(payload, req, target)
		if scope == "factory-b" {
			if !errors.Is(err, workersessions.ErrContinuationRequestIDConflict) {
				t.Fatalf("peer scope accepted persisted target: %v", err)
			}
		} else if err != nil {
			t.Fatalf("matching scope %q rejected: %v", scope, err)
		}
	}
}

func TestContinuationLineageRetainsOwnerWhenPeerAppearsAfterReservation(t *testing.T) {
	t.Parallel()
	req := continuationReservationRequest()
	r := newContinuationSource(t, req)
	source := r.sessions[req.SourceWorkerSessionID]
	delete(r.sessions, req.SourceWorkerSessionID)
	selectedAddress := scopedWorkerAddress(source.ID, "factory-a")
	r.sessions[selectedAddress] = source
	execution := continuationValidExecution("dispatch-1")
	execution.Execution.FactorySessionID = "factory-a"
	r.supervisions[selectedAddress] = newSupervision("dispatch-1", "turn-1", execution)
	r.observations[selectedAddress] = &observation{factorySessionID: "factory-a"}
	replay, owner, err := r.reserveContinuation(req)
	if err != nil || !owner {
		t.Fatalf("unique unscoped reservation = %v, owner=%v", err, owner)
	}
	peerAddress := scopedWorkerAddress(source.ID, "factory-b")
	r.sessions[peerAddress] = source.Clone()
	r.observations[peerAddress] = &observation{factorySessionID: "factory-b"}
	r.commitContinuationLineage(replay.plan)
	selected, err := r.Get(t.Context(), workersessions.GetRequest{ID: source.ID, FactorySessionID: "factory-a"})
	if err != nil || selected.SuccessorWorkerSessionID != req.SuccessorWorkerSessionID {
		t.Fatalf("frozen owner lost lineage: %+v, %v", selected, err)
	}
	peer, err := r.Get(t.Context(), workersessions.GetRequest{ID: source.ID, FactorySessionID: "factory-b"})
	if err != nil || peer.SuccessorWorkerSessionID != "" {
		t.Fatalf("late peer received lineage: %+v, %v", peer, err)
	}
	r.finishStart()
}
