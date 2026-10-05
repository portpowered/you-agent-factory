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
	input         json.RawMessage
	writeErr      error
	readErr       error
	corrupt       bool
	readKeys      []recordings.WorkerControlOperationKey
	ignoreLoadKey bool // Inject a corrupt store response at the coordinator boundary.
}

func (s *interruptInputStore) LoadWorkerControlOperation(_ context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	if len(s.records) == 0 {
		return recordings.WorkerControlOperationRecord{}, os.ErrNotExist
	}
	record := s.records[len(s.records)-1]
	if !s.ignoreLoadKey && interruptOperationKey(record) != key {
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

func (s *interruptInputStore) ReadWorkerControlInput(_ context.Context, key recordings.WorkerControlOperationKey, ref string) (json.RawMessage, error) {
	s.readKeys = append(s.readKeys, key)
	if ref != "scoped-input" {
		return nil, os.ErrNotExist
	}
	if s.corrupt {
		return json.RawMessage(`{}`), nil
	}
	return append(json.RawMessage(nil), s.input...), s.readErr
}

func TestInterruptJournalReplayRequiresExactCapturedInput(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"intact", "missing", "corrupt", "changed-input", "read-failure", "missing-ref", "foreign-ref", "unknown-version", "recorded-mode", "wrong-request", "wrong-worker", "wrong-attempt", "missing-attempt"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseValidation, false)
			if err := r.commitInterruptResult(t.Context(), operation, result, workersessions.ErrInterruptSourceConflict); err != nil {
				t.Fatal(err)
			}
			mutateInterruptCapturedInput(scenario, store)
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
			want := recordings.ErrWorkerRecordingPersistence
			if scenario == "intact" {
				want = workersessions.ErrInterruptSourceConflict
				if !reflect.DeepEqual(replayed, result) {
					t.Fatalf("intact captured result changed: %#v", replayed)
				}
			}
			if !found || !errors.Is(replayErr, want) || replayed.Accepted || calls != 0 || strings.Contains(replayErr.Error(), "private-input-path") {
				t.Fatalf("captured replay=%#v found=%v err=%v effects=%d", replayed, found, replayErr, calls)
			}
			if len(store.records) != 2 {
				t.Fatalf("read-only replay appended %d records", len(store.records))
			}
			for _, key := range store.readKeys {
				if key != interruptOperationKey(*operation) {
					t.Fatalf("input read crossed scoped key: %#v", key)
				}
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func mutateInterruptCapturedInput(scenario string, store *interruptInputStore) {
	record := &store.records[len(store.records)-1]
	switch scenario {
	case "missing":
		store.readErr = os.ErrNotExist
	case "corrupt":
		store.corrupt = true
	case "changed-input":
		var input workersessions.InterruptRequest
		_ = json.Unmarshal(store.input, &input)
		input.ReplacementMessage = "different replacement"
		store.input, _ = json.Marshal(input)
	case "read-failure":
		store.readErr = errors.New("private-input-path")
	case "missing-ref":
		record.InputArtifactRef = ""
	case "foreign-ref":
		record.InputArtifactRef = "foreign-profile-input"
	case "unknown-version":
		record.Operation.Version++
	case "recorded-mode":
		record.Operation.ResumeMode = "recorded"
	case "wrong-request":
		record.Operation.RequestID = "other-request"
		store.ignoreLoadKey = true
	case "wrong-worker":
		record.Operation.WorkerSessionID = "other-worker"
	case "wrong-attempt":
		record.Operation.ExpectedAttemptID = "other-attempt"
	case "missing-attempt":
		record.Operation.ExpectedAttemptID = ""
	}
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
		assertPendingInterruptPhaseFacts(t, req, record)
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
	if replayed.Source.SuccessorWorkerSessionID != outcome.result.Source.SuccessorWorkerSessionID || replayed.Successor.PredecessorWorkerSessionID != outcome.result.Successor.PredecessorWorkerSessionID {
		t.Fatalf("journal replay lost accepted lineage: got=%#v original=%#v", replayed, outcome.result)
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

func assertPendingInterruptPhaseFacts(t *testing.T, req workersessions.InterruptRequest, record recordings.WorkerControlOperationRecord) {
	t.Helper()
	if record.Operation.Phase != "SOURCE_STOPPED" && record.Operation.Phase != "SUCCESSOR_ADMITTED" {
		return
	}
	pending, pendingErr := decodeInterruptOutcome(req, record)
	if !errors.Is(pendingErr, workersessions.ErrInterruptExecutionUnavailable) || pending.Source.State != workersessions.StateCanceled || pending.Phase != workersessions.InterruptPhaseSuccessorAdmission {
		t.Fatalf("synced phase lost joined source: phase=%s result=%#v err=%v", record.Operation.Phase, pending, pendingErr)
	}
	if (record.Operation.Phase == "SUCCESSOR_ADMITTED") != pending.Accepted {
		t.Fatalf("synced admission fact differs from phase: phase=%s result=%#v", record.Operation.Phase, pending)
	}
}

func TestInterruptJoinedSourceSnapshotSyncFailurePreventsSuccessorAdmission(t *testing.T) {
	t.Parallel()
	fixture := newInterruptRaceCharacterizationFixture(t, "stopped-snapshot-fault", false)
	r := fixture.registry.(*registry)
	store := &interruptInputStore{stopOperationStore: stopOperationStore{advanceErr: errors.New("private-sync-path")}}
	r.operations = store
	capture := exactCaptureIdentity()
	capture.WorkerSessionID, capture.FactorySessionID = fixture.sourceID, ""
	pub := r.publicationFor(fixture.sourceID)
	pub.mu.Lock()
	pub.capture = capture
	pub.mu.Unlock()
	req := workersessions.InterruptRequest{RequestID: "snapshot-fault", SourceWorkerSessionID: fixture.sourceID, SuccessorWorkerSessionID: fixture.successorID, ReplacementMessage: "replacement"}
	outcomes := startInterruptCharacterization(t, fixture.registry, req)
	fixture.boundary.waitCancellation(t, fixture.sourceDispatch)
	fixture.boundary.releaseCancellation(fixture.sourceDispatch)
	fixture.boundary.waitReturned(t, fixture.sourceDispatch)
	outcome := <-outcomes
	<-fixture.sourceResult
	if !errors.Is(outcome.err, recordings.ErrWorkerRecordingPersistence) || outcome.result.Accepted || outcome.result.Source.State != workersessions.StateCanceled || outcome.result.Phase != workersessions.InterruptPhaseSuccessorAdmission || strings.Contains(outcome.err.Error(), "private-sync-path") {
		t.Fatalf("stopped snapshot sync failure=%#v err=%v", outcome.result, outcome.err)
	}
	assertNoSuccessor(t, r, fixture.successorID)
	assertBoundaryEffects(t, fixture.boundary, 1, 1, "stopped snapshot persistence barrier")
	if len(store.records) != 1 || store.records[0].Operation.Phase != "INTENT" || len(store.records[0].Result) != 0 {
		t.Fatalf("unacknowledged stopped facts entered committed history: %#v", store.records)
	}
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
		record.Result = nil // Legacy phase-only row, with no accepted snapshot.
		record.FailureCode = ""
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

// Only the coordinator is real; each parallel cell owns its operation store,
// capture and cancel collaborator. Host restart is an integration-owned edge.
func TestInterruptJournalReplayPreservesCommittedPendingFactsWithoutEffects(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"SOURCE_STOPPED", "SUCCESSOR_ADMITTED"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			result := interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, false)
			result.Source = workersessions.Session{ID: "worker", State: workersessions.StateCanceled}
			if err := r.commitInterruptPhase(t.Context(), operation, "SOURCE_STOPPED", result, nil); err != nil {
				t.Fatal(err)
			}
			if phase == "SUCCESSOR_ADMITTED" {
				result.Accepted = true
				result.Source.SuccessorWorkerSessionID = "successor"
				result.Successor = workersessions.Session{ID: "successor", State: workersessions.StateRunning, PredecessorWorkerSessionID: "worker"}
				if err := r.commitInterruptPhase(t.Context(), operation, phase, result, nil); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			before := len(store.records)
			for range 2 {
				replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
				if !found || !errors.Is(replayErr, workersessions.ErrInterruptExecutionUnavailable) || !reflect.DeepEqual(replayed, result) {
					t.Fatalf("pending facts=%#v found=%v err=%v want=%#v", replayed, found, replayErr, result)
				}
				var typed *workersessions.InterruptError
				if !errors.As(replayErr, &typed) || !reflect.DeepEqual(typed.Result, result) || typed.Phase != result.Phase {
					t.Fatalf("pending error lost facts: %#v", typed)
				}
			}
			if calls != 0 || len(store.records) != before {
				t.Fatalf("pending recovery repeated effects: cancel=%d rows=%d", calls, len(store.records))
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func TestInterruptPendingSnapshotRejectsInconsistentOrPrivateFacts(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"malformed", "running-source", "unexpected-successor", "accepted-before-admission", "wrong-phase", "failure-code", "failure-causes", "private-model", "unaccepted-admission"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			req := workersessions.InterruptRequest{RequestID: "request", SourceWorkerSessionID: "worker", SuccessorWorkerSessionID: "successor"}
			outcome := durableInterruptOutcome{InterruptResult: interruptResult(req, workersessions.InterruptPhaseSuccessorAdmission, false)}
			outcome.Source = workersessions.Session{ID: "worker", State: workersessions.StateCanceled}
			record := recordings.WorkerControlOperationRecord{Operation: recordings.WorkerControlOperation{Phase: "SOURCE_STOPPED"}}
			mutatePendingInterruptSnapshot(scenario, &record, &outcome)
			record.Result, _ = json.Marshal(outcome)
			if scenario == "malformed" {
				record.Result = json.RawMessage(`{`)
			}
			result, err := decodePendingInterruptOutcome(req, record)
			if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || result.Accepted || result.Phase != workersessions.InterruptPhaseValidation || strings.Contains(err.Error(), "private-pending-model") {
				t.Fatalf("invalid pending result=%#v err=%v", result, err)
			}
		})
	}
}

func mutatePendingInterruptSnapshot(scenario string, record *recordings.WorkerControlOperationRecord, outcome *durableInterruptOutcome) {
	switch scenario {
	case "running-source":
		outcome.Source.State = workersessions.StateRunning
	case "unexpected-successor":
		outcome.Successor = workersessions.Session{ID: "successor", State: workersessions.StateRunning}
	case "accepted-before-admission":
		outcome.Accepted = true
	case "wrong-phase":
		outcome.Phase = workersessions.InterruptPhaseSourceCancellation
	case "failure-code":
		record.FailureCode = "private-pending-model"
	case "failure-causes":
		outcome.FailureCauses = []string{"EXECUTION_UNAVAILABLE"}
	case "private-model":
		private := "private-pending-model"
		outcome.Source.Model = &private
	case "unaccepted-admission":
		record.Operation.Phase = "SUCCESSOR_ADMITTED"
	}
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
	for _, scenario := range []string{"unknown-cause", "duplicate-cause", "unknown-phase", "wrong-source", "source-not-stopped", "wrong-successor", "unknown-state", "successor-not-admitted", "success-with-failure", "success-with-code", "source-self-predecessor", "source-cycle", "foreign-source-successor", "foreign-successor-predecessor", "successor-self-link", "successor-cycle", "absent-successor-lineage"} {
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
	default:
		mutateCommittedInterruptLineage(scenario, outcome)
	}
}

func mutateCommittedInterruptLineage(scenario string, outcome *durableInterruptOutcome) {
	switch scenario {
	case "source-self-predecessor":
		outcome.Source.PredecessorWorkerSessionID = outcome.Source.ID
	case "source-cycle":
		outcome.Source.PredecessorWorkerSessionID = outcome.Successor.ID
	case "foreign-source-successor":
		outcome.Source.SuccessorWorkerSessionID = "other"
	case "foreign-successor-predecessor":
		outcome.Successor.PredecessorWorkerSessionID = "other"
	case "successor-self-link":
		outcome.Successor.SuccessorWorkerSessionID = outcome.Successor.ID
	case "successor-cycle":
		outcome.Successor.SuccessorWorkerSessionID = outcome.Source.ID
	case "absent-successor-lineage":
		outcome.Accepted = false
		outcome.Successor = workersessions.Session{PredecessorWorkerSessionID: outcome.Source.ID}
	}
}

func TestInterruptJournalReplayPreservesAcceptedLineageWithoutPrivateMetadata(t *testing.T) {
	t.Parallel()
	r, plan, store := newDurableInterruptFixture(t)
	operation, err := r.beginInterruptIntent(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"SOURCE_STOPPED", "SUCCESSOR_ADMITTED"} {
		if err := r.advanceInterruptPhase(t.Context(), operation, phase); err != nil {
			t.Fatal(err)
		}
	}
	private := "private-provider-secret"
	result := interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, true)
	result.Source = workersessions.Session{
		ID: "worker", State: workersessions.StateCanceled, Model: &private,
		PredecessorWorkerSessionID: "earlier-worker", SuccessorWorkerSessionID: "successor",
	}
	result.Successor = workersessions.Session{
		ID: "successor", State: workersessions.StateRunning, ReasoningEffort: &private,
		PredecessorWorkerSessionID: "worker",
	}
	if err := r.commitInterruptResult(t.Context(), operation, result, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(operation.Result), private) {
		t.Fatal("private session metadata reached the journal")
	}
	// Later session mutations must not replace the snapshot accepted by this key.
	r.mu.Lock()
	source := r.sessions["worker"]
	source.SuccessorWorkerSessionID = "later-worker"
	r.sessions["worker"] = source
	r.mu.Unlock()
	want := result.Clone()
	want.Source.Model = nil
	want.Successor.ReasoningEffort = nil
	replayed, found, err := r.replayDurableInterrupt(t.Context(), plan.request)
	if err != nil || !found || !reflect.DeepEqual(replayed, want) {
		t.Fatalf("accepted snapshot changed: got=%#v want=%#v found=%v err=%v", replayed, want, found, err)
	}
	replayed.Source.PredecessorWorkerSessionID = "caller-mutation"
	refetched, _, err := r.replayDurableInterrupt(t.Context(), plan.request)
	if err != nil || !reflect.DeepEqual(refetched, want) || len(store.records) != 4 {
		t.Fatalf("retry mutated snapshot or journal: got=%#v err=%v records=%d", refetched, err, len(store.records))
	}
	assertNoSuccessor(t, r, "successor")
}

func TestInterruptJournalReplayRejectsPrivateSessionContentWithoutEffects(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"COMPLETED", "FAILED"} {
		for _, owner := range []string{"source", "successor"} {
			for _, field := range []string{"model", "reasoning", "terminal", "association"} {
				t.Run(phase+"/"+owner+"/"+field, func(t *testing.T) {
					t.Parallel()
					r, plan, store := newDurableInterruptFixture(t)
					operation, err := r.beginInterruptIntent(t.Context(), plan)
					if err != nil {
						t.Fatal(err)
					}
					outcome := durableInterruptOutcome{InterruptResult: interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, true)}
					outcome.Source = workersessions.Session{ID: "worker", State: workersessions.StateCanceled}
					outcome.Successor = workersessions.Session{ID: "successor", State: workersessions.StateRunning}
					operation.Operation.Phase = phase
					if phase == "FAILED" {
						outcome.Accepted = false
						operation.FailureCode = string(outcome.Phase)
						outcome.FailureCauses = []string{"SUCCESSOR_ADMISSION_FAILED"}
					}
					session := &outcome.Source
					if owner == "successor" {
						session = &outcome.Successor
					}
					injectPrivateInterruptSessionContent(field, session)
					operation.Result, _ = json.Marshal(outcome)
					store.records = []recordings.WorkerControlOperationRecord{operation.Detached()}
					calls := 0
					plan.supervision.installCancel(func() { calls++ })
					result, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
					want := interruptResult(plan.request, workersessions.InterruptPhaseValidation, false)
					var typed *workersessions.InterruptError
					if !found || !errors.Is(replayErr, recordings.ErrWorkerRecordingPersistence) || !errors.As(replayErr, &typed) || !reflect.DeepEqual(result, want) || !reflect.DeepEqual(typed.Result, want) {
						t.Fatalf("private content replay=%#v found=%v err=%v", result, found, replayErr)
					}
					assertPrivateInterruptContentAbsent(t, typed, replayErr)
					if calls != 0 || len(store.records) != 1 {
						t.Fatalf("refusal had effects: cancellations=%d journal rows=%d", calls, len(store.records))
					}
					assertNoSuccessor(t, r, "successor")
				})
			}
		}
	}
}

func assertPrivateInterruptContentAbsent(t *testing.T, typed *workersessions.InterruptError, err error) {
	t.Helper()
	encoded, marshalErr := json.Marshal(typed.Result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), "private-provider-secret") || strings.Contains(err.Error(), "private-provider-secret") {
		t.Fatal("private journal content reached the response")
	}
}

func injectPrivateInterruptSessionContent(field string, session *workersessions.Session) {
	private := "private-provider-secret"
	switch field {
	case "model":
		session.Model = &private
	case "reasoning":
		session.ReasoningEffort = &private
	case "terminal":
		session.Result = &workersessions.TerminalResult{Cause: &workersessions.FailureCause{Detail: private}}
	case "association":
		session.ProviderSessionAssociation = &workersessions.ProviderSessionAssociation{WorkerSessionID: private}
	}
}
