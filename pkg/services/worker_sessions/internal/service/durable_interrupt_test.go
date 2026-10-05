package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type interruptInputStore struct {
	stopOperationStore
	input    json.RawMessage
	writeErr error
	readErr  error
	corrupt  bool
}

func (s *interruptInputStore) LoadWorkerControlOperation(_ context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	if len(s.records) == 0 {
		return recordings.WorkerControlOperationRecord{}, os.ErrNotExist
	}
	record := s.records[len(s.records)-1]
	if interruptOperationKey(record) != key {
		return recordings.WorkerControlOperationRecord{}, os.ErrNotExist
	}
	return record.Detached(), nil
}

func (s *interruptInputStore) PersistWorkerControlInput(_ context.Context, _ recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if s.writeErr != nil {
		return "", s.writeErr
	}
	s.input = append(json.RawMessage(nil), input...)
	return "scoped-input", nil
}

func (s *interruptInputStore) ReadWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, string) (json.RawMessage, error) {
	if s.corrupt {
		return json.RawMessage(`{}`), nil
	}
	return append(json.RawMessage(nil), s.input...), s.readErr
}

func newDurableInterruptFixture(t *testing.T) (*registry, interruptPlan, *interruptInputStore) {
	t.Helper()
	r, s, _ := newDurableStopFixture(t)
	store := &interruptInputStore{}
	r.operations = store
	return r, interruptPlan{
		request:    workersessions.InterruptRequest{RequestID: "interrupt-request", SourceWorkerSessionID: "worker", SuccessorWorkerSessionID: "successor", ReplacementMessage: "exact replacement"},
		dispatchID: s.dispatchID, supervision: s,
	}, store
}

func TestInterruptDurableInputRefusesBeforeCancellation(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"write", "read", "corrupt", "intent", "conflict", "secret", "escaped-secret"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			private := errors.New("private-provider-secret")
			want := configureInterruptInputFailure(&plan, store, failure, private)
			result, err := r.runInterrupt(plan)
			if !errors.Is(err, want) || result.Phase != workersessions.InterruptPhaseValidation || calls != 0 || result.Source.State != workersessions.StateRunning {
				t.Fatalf("preflight result=%#v err=%v cancellation calls=%d", result, err, calls)
			}
			if strings.Contains(err.Error(), private.Error()) || len(store.records) != 0 {
				t.Fatal("refused operation leaked diagnostics or committed intent")
			}
			if strings.Contains(failure, "secret") && len(store.input) != 0 {
				t.Fatal("secret reached input store")
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func configureInterruptInputFailure(plan *interruptPlan, store *interruptInputStore, failure string, private error) error {
	switch failure {
	case "write":
		store.writeErr = private
	case "read":
		store.readErr = private
	case "corrupt":
		store.corrupt = true
	case "intent":
		store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error { return private }
	case "conflict":
		store.writeErr = recordings.ErrWorkerControlConflict
		return workersessions.ErrInterruptRequestIDConflict
	default:
		secret := "private-provider-secret"
		if failure == "escaped-secret" {
			secret = "private\nsecret\""
		}
		plan.execution.Execution.EnvVars = map[string]string{"TOKEN": secret}
		plan.request.ReplacementMessage = "prefix " + secret
		return recordings.ErrInvalidRecordingRedactionRequest
	}
	return recordings.ErrWorkerRecordingPersistence
}

func TestInterruptDurableIntentRechecksAttemptAfterSync(t *testing.T) {
	t.Parallel()
	r, plan, store := newDurableInterruptFixture(t)
	calls := 0
	plan.supervision.installCancel(func() { calls++ })
	store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
		plan.supervision.dispatchID = "new-attempt"
		return nil
	}
	result, err := r.runInterrupt(plan)
	if !errors.Is(err, workersessions.ErrInterruptSourceConflict) || result.Phase != workersessions.InterruptPhaseValidation || calls != 0 {
		t.Fatalf("stale interrupt=%#v err=%v effects=%d", result, err, calls)
	}
	if len(store.records) != 2 || store.records[1].Operation.Phase != "FAILED" || store.records[1].Target.ExpectedAttemptID != plan.dispatchID {
		t.Fatalf("stale facts=%#v", store.records)
	}
	replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
	if !found || !errors.Is(replayErr, workersessions.ErrInterruptSourceConflict) || !reflect.DeepEqual(replayed, result) || calls != 0 {
		t.Fatalf("stale failure replay=%#v found=%v err=%v effects=%d", replayed, found, replayErr, calls)
	}
}

func TestInterruptDurableOperationJoinsBeforeSuccessorAndReplays(t *testing.T) {
	t.Parallel()
	fixture := newInterruptRaceCharacterizationFixture(t, "durable-phases", false)
	r := fixture.registry.(*registry)
	store := &interruptInputStore{}
	r.operations = store
	capture := exactCaptureIdentity()
	capture.WorkerSessionID = fixture.sourceID
	capture.FactorySessionID = ""
	pub := r.publicationFor(fixture.sourceID)
	pub.mu.Lock()
	pub.capture = capture
	pub.mu.Unlock()
	req := workersessions.InterruptRequest{
		RequestID: "durable-request", SourceWorkerSessionID: fixture.sourceID,
		SuccessorWorkerSessionID: fixture.successorID, ReplacementMessage: "exact replacement",
	}
	outcomes := startInterruptCharacterization(t, fixture.registry, req)
	fixture.boundary.waitCancellation(t, fixture.sourceDispatch)
	// The command boundary remains held until this signal releases its callback.
	assertBoundaryEffects(t, fixture.boundary, 1, 1, "source join barrier")
	fixture.boundary.releaseCancellation(fixture.sourceDispatch)
	fixture.boundary.waitReturned(t, fixture.sourceDispatch)
	outcome := <-outcomes
	source := <-fixture.sourceResult
	assertInterruptWins(t, fixture, outcome, source)
	var phases []string
	for _, record := range store.records {
		phases = append(phases, record.Operation.Phase)
	}
	if !reflect.DeepEqual(phases, []string{"INTENT", "SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED"}) {
		t.Fatalf("committed phases=%v", phases)
	}
	replayed, err := fixture.registry.Interrupt(t.Context(), req)
	if err != nil || !reflect.DeepEqual(replayed, outcome.result) {
		t.Fatalf("retry=%#v err=%v", replayed, err)
	}
	assertBoundaryEffects(t, fixture.boundary, 2, 1, "durable replay")
	r.mu.Lock()
	r.interruptReplays = nil
	r.mu.Unlock()
	replayed, err = fixture.registry.Interrupt(t.Context(), req)
	if err != nil || !replayed.Accepted || replayed.Source.State != outcome.result.Source.State || replayed.Successor.ID != fixture.successorID {
		t.Fatalf("journal replay=%#v err=%v", replayed, err)
	}
	assertBoundaryEffects(t, fixture.boundary, 2, 1, "journal replay without memory")
	// Catalog lookup is the read-only boundary after live handles are gone.
	r.mu.Lock()
	delete(r.publications, fixture.sourceID)
	delete(r.sessions, fixture.sourceID)
	delete(r.supervisions, fixture.sourceID)
	r.mu.Unlock()
	r.logs = &LogReader{reader: &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
		RecordingID: capture.RecordingID, WorkerSessionID: capture.WorkerSessionID,
		RecordingGenerationID: capture.RecordingGenerationID, OwnerEpoch: capture.OwnerEpoch,
	}}}
	replayed, err = fixture.registry.Interrupt(t.Context(), req)
	if err != nil || !replayed.Accepted || replayed.Source.State != workersessions.StateCanceled {
		t.Fatalf("catalog journal replay=%#v err=%v", replayed, err)
	}
	assertBoundaryEffects(t, fixture.boundary, 2, 1, "catalog replay without live handles")
}

func TestInterruptDurablePhasesPreserveReservedIdentityAndSafeFailure(t *testing.T) {
	t.Parallel()
	r, plan, store := newDurableInterruptFixture(t)
	operation, err := r.beginInterruptIntent(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	var input workersessions.InterruptRequest
	if json.Unmarshal(store.input, &input) != nil || input != plan.request || operation.Operation.SuccessorWorkerSessionID != "successor" || operation.InputArtifactRef != "scoped-input" {
		t.Fatalf("captured input=%#v operation=%#v", input, operation)
	}
	if err := r.advanceInterruptPhase(t.Context(), operation, "SOURCE_STOPPED"); err != nil {
		t.Fatal(err)
	}
	privateModel := "private-provider-secret"
	result := workersessions.InterruptResult{
		RequestID: plan.request.RequestID, SourceWorkerSessionID: "worker", SuccessorWorkerSessionID: "successor", Phase: workersessions.InterruptPhaseSuccessorAdmission,
		Source: workersessions.Session{ID: "worker", State: workersessions.StateCanceled, Model: &privateModel},
	}
	if err := r.commitInterruptResult(t.Context(), operation, result, errors.New("private-provider-secret")); err != nil {
		t.Fatal(err)
	}
	var phases []string
	for _, record := range store.records {
		phases = append(phases, record.Operation.Phase)
		if record.Operation.SuccessorWorkerSessionID != "successor" || strings.Contains(string(record.Result), "private-provider-secret") {
			t.Fatal("phase changed reservation or leaked data")
		}
	}
	if !reflect.DeepEqual(phases, []string{"INTENT", "SOURCE_STOPPED", "FAILED"}) || operation.FailureCode != string(result.Phase) {
		t.Fatalf("phases=%v operation=%#v", phases, operation)
	}
}

func TestInterruptJournalReplayPreservesFailureAndRefusesUnsafeStages(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"admission-failure", "pending", "changed-message", "changed-successor", "generation", "owner", "malformed", "wrong-result", "wrong-code"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.advanceInterruptPhase(t.Context(), operation, "SOURCE_STOPPED"); err != nil {
				t.Fatal(err)
			}
			result := interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, false)
			result.Source = workersessions.Session{ID: "worker", State: workersessions.StateCanceled}
			if err := r.commitInterruptResult(t.Context(), operation, result, workersessions.ErrInterruptSuccessorAdmissionFailed); err != nil {
				t.Fatal(err)
			}
			want := mutateInterruptReplayFixture(scenario, &plan.request, &store.records[len(store.records)-1])
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			replayed, found, err := r.replayDurableInterrupt(t.Context(), plan.request)
			if !found || !errors.Is(err, want) || calls != 0 {
				t.Fatalf("found=%v replay=%#v err=%v effects=%d", found, replayed, err, calls)
			}
			if scenario == "admission-failure" && !reflect.DeepEqual(replayed, result) {
				t.Fatalf("partial snapshot changed: %#v", replayed)
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func mutateInterruptReplayFixture(scenario string, req *workersessions.InterruptRequest, record *recordings.WorkerControlOperationRecord) error {
	switch scenario {
	case "admission-failure":
		return workersessions.ErrInterruptSuccessorAdmissionFailed
	case "pending":
		record.Operation.Phase = "SOURCE_STOPPED"
		return workersessions.ErrInterruptExecutionUnavailable
	case "changed-message":
		req.ReplacementMessage += " changed"
		return workersessions.ErrInterruptRequestIDConflict
	case "changed-successor":
		req.SuccessorWorkerSessionID = "different-successor"
		return workersessions.ErrInterruptRequestIDConflict
	case "generation":
		record.Target.RecordingGenerationID = "different-generation"
		return workersessions.ErrInterruptSourceConflict
	case "owner":
		record.Target.OwnerEpoch = "different-owner"
		return workersessions.ErrInterruptSourceConflict
	case "malformed":
		record.Result = json.RawMessage(`{`)
	case "wrong-result":
		var result workersessions.InterruptResult
		_ = json.Unmarshal(record.Result, &result)
		result.SuccessorWorkerSessionID = "other"
		record.Result, _ = json.Marshal(result)
	case "wrong-code":
		record.FailureCode = "untrusted-private-diagnostic"
	}
	return recordings.ErrWorkerRecordingPersistence
}

func TestInterruptJournalReplayPreservesTypedFailureWithoutPrivateDiagnostics(t *testing.T) {
	t.Parallel()
	for _, identity := range interruptFailureIdentities() {
		t.Run(identity.code, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseValidation, false)
			original := newInterruptError(result.Phase, result, errors.Join(identity.cause, errors.New("private-provider-secret")))
			if err := r.commitInterruptResult(t.Context(), operation, result, original); err != nil {
				t.Fatal(err)
			}
			replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
			var typed *workersessions.InterruptError
			if !found || !errors.Is(replayErr, identity.cause) || !errors.As(replayErr, &typed) || !reflect.DeepEqual(replayed, result) || !reflect.DeepEqual(typed.Result, result) {
				t.Fatalf("failure replay=%#v found=%v err=%v", replayed, found, replayErr)
			}
			if strings.Contains(replayErr.Error(), "private-provider-secret") || strings.Contains(string(store.records[len(store.records)-1].Result), "private-provider-secret") {
				t.Fatal("private diagnostics reached durable failure replay")
			}
			assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
		})
	}
}

func TestInterruptJournalReplayRejectsInconsistentCommittedFacts(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unknown-cause", "duplicate-cause", "unknown-phase", "wrong-source", "source-not-stopped", "wrong-successor", "unknown-state", "successor-not-admitted", "success-with-failure", "success-with-code"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			outcome := durableInterruptOutcome{InterruptResult: interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, true)}
			outcome.Source = workersessions.Session{ID: "worker", State: workersessions.StateCanceled}
			outcome.Successor = workersessions.Session{ID: "successor", State: workersessions.StateRunning}
			operation.Operation.Phase = "COMPLETED"
			mutateCommittedInterruptFacts(scenario, &outcome, operation)
			operation.Result, _ = json.Marshal(outcome)
			store.records = []recordings.WorkerControlOperationRecord{operation.Detached()}
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			result, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
			if !found || !errors.Is(replayErr, recordings.ErrWorkerRecordingPersistence) || result.Accepted || calls != 0 {
				t.Fatalf("invalid committed replay=%#v found=%v err=%v effects=%d", result, found, replayErr, calls)
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func TestInterruptJournalReplayRetainsJoinedAdmissionCausesAndLegacyFailure(t *testing.T) {
	t.Parallel()
	r, plan, store := newDurableInterruptFixture(t)
	operation, err := r.beginInterruptIntent(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.advanceInterruptPhase(t.Context(), operation, "SOURCE_STOPPED"); err != nil {
		t.Fatal(err)
	}
	result := interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, false)
	result.Source = workersessions.Session{ID: "worker", State: workersessions.StateCanceled}
	cause := errors.Join(workersessions.ErrInterruptSuccessorAdmissionFailed, workersessions.ErrContinuationSuccessorConflict, errors.New("private-provider-secret"))
	if err := r.commitInterruptResult(t.Context(), operation, result, newInterruptError(result.Phase, result, cause)); err != nil {
		t.Fatal(err)
	}
	replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
	if !found || !reflect.DeepEqual(replayed, result) || !errors.Is(replayErr, workersessions.ErrInterruptSuccessorAdmissionFailed) || !errors.Is(replayErr, workersessions.ErrContinuationSuccessorConflict) || strings.Contains(replayErr.Error(), "private-provider-secret") {
		t.Fatalf("joined admission replay=%#v found=%v err=%v", replayed, found, replayErr)
	}
	// Pre-extension journals have only the safe result and stable phase.
	store.records[len(store.records)-1].Result, _ = json.Marshal(result)
	replayed, found, replayErr = r.replayDurableInterrupt(t.Context(), plan.request)
	if !found || !reflect.DeepEqual(replayed, result) || !errors.Is(replayErr, workersessions.ErrInterruptSuccessorAdmissionFailed) {
		t.Fatalf("legacy admission replay=%#v found=%v err=%v", replayed, found, replayErr)
	}
	assertNoSuccessor(t, r, "successor")
}

func mutateCommittedInterruptFacts(scenario string, outcome *durableInterruptOutcome, operation *recordings.WorkerControlOperationRecord) {
	switch scenario {
	case "unknown-cause", "duplicate-cause":
		operation.Operation.Phase = "FAILED"
		operation.FailureCode = string(outcome.Phase)
		outcome.Accepted = false
		outcome.FailureCauses = []string{"SOURCE_CONFLICT", "SOURCE_CONFLICT"}
		if scenario == "unknown-cause" {
			outcome.FailureCauses = []string{"private-untrusted-diagnostic"}
		}
	case "unknown-phase":
		outcome.Phase = "UNKNOWN"
	case "wrong-source":
		outcome.Source.ID = "other"
	case "source-not-stopped":
		outcome.Source.State = workersessions.StateRunning
	case "wrong-successor":
		outcome.Successor.ID = "other"
	case "unknown-state":
		outcome.Successor.State = "UNKNOWN"
	case "successor-not-admitted":
		outcome.Successor.State = workersessions.StateReserved
	case "success-with-failure":
		outcome.FailureCauses = []string{"SOURCE_CONFLICT"}
	case "success-with-code":
		operation.FailureCode = string(outcome.Phase)
	}
}
