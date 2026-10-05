package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestInterruptFailedReplayCannotClaimAdmissionBeforeSourceJoin(t *testing.T) {
	t.Parallel()
	for _, phase := range []workersessions.InterruptPhase{workersessions.InterruptPhaseValidation, workersessions.InterruptPhaseSourceCancellation} {
		for _, fact := range []string{"successor", "source-link"} {
			t.Run(string(phase)+"/"+fact, func(t *testing.T) {
				t.Parallel()
				r, plan, store := newDurableInterruptFixture(t)
				operation, err := r.beginInterruptIntent(t.Context(), plan)
				if err != nil {
					t.Fatal(err)
				}
				result := r.interruptResultSnapshot(plan.request, phase, false)
				if fact == "successor" {
					result.Successor = workersessions.Session{ID: plan.request.SuccessorWorkerSessionID, State: workersessions.StateRunning}
				} else {
					result.Source.SuccessorWorkerSessionID = plan.request.SuccessorWorkerSessionID
				}
				operation.Operation.Phase = "FAILED"
				operation.FailureCode = string(phase)
				operation.Result, _ = json.Marshal(durableInterruptOutcome{InterruptResult: result})
				store.records = []recordings.WorkerControlOperationRecord{operation.Detached()}
				calls := 0
				plan.supervision.installCancel(func() { calls++ })
				replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
				if !found || !errors.Is(replayErr, recordings.ErrWorkerRecordingPersistence) || replayed.Accepted || replayed.Successor.ID != "" || calls != 0 || len(store.records) != 1 {
					t.Fatalf("contradictory failure replay=%#v found=%v err=%v effects=%d", replayed, found, replayErr, calls)
				}
				assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
			})
		}
	}
}

type interruptPendingCaptureReader struct {
	entry     recordings.WorkerSessionCatalogEntry
	page      recordings.WorkerCapturedActivityPage
	lookupErr error
	readErr   error
	lookups   []string
	reads     []recordings.WorkerCapturedActivityRequest
}

func (f *interruptPendingCaptureReader) LookupWorkerSessionCapture(_ context.Context, id string) (recordings.WorkerSessionCatalogEntry, error) {
	f.lookups = append(f.lookups, id)
	return f.entry, f.lookupErr
}

func (f *interruptPendingCaptureReader) ReadWorkerCapturedActivity(_ context.Context, req recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	f.reads = append(f.reads, req)
	return f.page, f.readErr
}

func TestInterruptPendingAdmissionInspectsExactOpeningWithoutRestoringAuthority(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing", "matching", "growing", "lookup-failure", "read-failure", "foreign-worker", "foreign-scope", "foreign-owner", "missing-generation", "changed-generation", "malformed", "wrong-attempt", "wrong-predecessor", "wrong-source-attempt",
		"shadow-draft-alias", "shadow-draft-duplicate", "shadow-draft-unknown", "shadow-worker-alias", "shadow-attempt-duplicate", "shadow-lineage-alias", "shadow-lineage-duplicate", "shadow-opening-unknown"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			want := interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, false)
			want.Source = workersessions.Session{ID: plan.request.SourceWorkerSessionID, State: workersessions.StateCanceled}
			if err := r.commitInterruptPhase(t.Context(), operation, "SOURCE_STOPPED", want, nil); err != nil {
				t.Fatal(err)
			}
			reader, cause := pendingInterruptCaptureFixture(scenario, plan, operation.Target)
			r.logs = &LogReader{reader: reader}
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			for range 2 {
				got, found, err := r.replayDurableInterrupt(t.Context(), plan.request)
				if !found || !errors.Is(err, cause) || !reflect.DeepEqual(got, want) || strings.Contains(err.Error(), "private-capture") {
					t.Fatalf("pending admission replay=%#v found=%v err=%v want=%v", got, found, err, cause)
				}
			}
			if calls != 0 || len(store.records) != 2 || !reflect.DeepEqual(reader.lookups, []string{plan.request.SuccessorWorkerSessionID, plan.request.SuccessorWorkerSessionID}) {
				t.Fatalf("recovery repeated effects or crossed identity: cancel=%d rows=%d reads=%v", calls, len(store.records), reader.lookups)
			}
			for _, req := range reader.reads {
				if req.WorkerSessionID != plan.request.SuccessorWorkerSessionID || req.Limit != 1 {
					t.Fatalf("unbounded/foreign activity read: %#v", req)
				}
			}
			assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
		})
	}
}

func pendingInterruptCaptureFixture(scenario string, plan interruptPlan, target recordings.WorkerControlTarget) (*interruptPendingCaptureReader, error) {
	entry := recordings.WorkerSessionCatalogEntry{WorkerSessionID: plan.request.SuccessorWorkerSessionID, RecordingID: "successor-recording", RecordingGenerationID: "successor-generation", OwnerEpoch: target.OwnerEpoch}
	dispatch := continuationDispatchID(target.ExpectedAttemptID, plan.request.SuccessorWorkerSessionID)
	opening := workers.SessionPayload{WorkerSessionID: entry.WorkerSessionID, RecordingID: entry.RecordingID, DispatchID: dispatch, AttemptID: dispatch,
		Lineage: &workers.SessionLineage{PredecessorWorkerSessionID: plan.request.SourceWorkerSessionID, PreviousDispatchID: target.ExpectedAttemptID, PreviousAttemptID: target.ExpectedAttemptID}}
	f := &interruptPendingCaptureReader{entry: entry, page: recordings.WorkerCapturedActivityPage{Catalog: entry}}
	cause := workersessions.ErrInterruptExecutionUnavailable
	switch scenario {
	case "missing":
		f.lookupErr = os.ErrNotExist
	case "growing":
		f.page.Catalog.CommittedPosition++
	case "lookup-failure":
		f.lookupErr = errors.New("private-capture-path")
		cause = recordings.ErrWorkerRecordingPersistence
	case "read-failure":
		f.readErr = errors.New("private-capture-path")
		cause = recordings.ErrWorkerRecordingPersistence
	case "foreign-worker":
		f.entry.WorkerSessionID = "foreign"
		cause = workersessions.ErrInterruptSourceConflict
	case "foreign-scope":
		f.entry.FactorySessionID = "foreign"
		cause = workersessions.ErrInterruptSourceConflict
	case "foreign-owner":
		f.entry.OwnerEpoch = "foreign"
		cause = workersessions.ErrInterruptSourceConflict
	case "missing-generation":
		f.entry.RecordingGenerationID = ""
		cause = recordings.ErrWorkerRecordingPersistence
	case "changed-generation":
		f.page.Catalog.RecordingGenerationID = "later"
		cause = recordings.ErrWorkerRecordingPersistence
	case "malformed":
		cause = recordings.ErrWorkerRecordingPersistence
	case "wrong-attempt":
		opening.AttemptID = "later"
		cause = workersessions.ErrInterruptSourceConflict
	case "wrong-predecessor":
		opening.Lineage.PredecessorWorkerSessionID = "foreign"
		cause = workersessions.ErrInterruptSourceConflict
	case "wrong-source-attempt":
		opening.Lineage.PreviousAttemptID = "later"
		cause = workersessions.ErrInterruptSourceConflict
	}
	payload, _ := json.Marshal(opening)
	draft, _ := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: payload})
	shadowed := shadowPendingInterruptOpening(scenario, payload, draft)
	if !reflect.DeepEqual(shadowed, draft) {
		draft = shadowed
		cause = recordings.ErrWorkerRecordingPersistence
	}
	f.page.Opening = events.Record{Payload: draft}
	return f, cause
}

// Earlier private or foreign values are concealed by a later canonical field
// under permissive JSON decoding. Exercise the coordinator, not just decoding.
func shadowPendingInterruptOpening(scenario string, payload, draft []byte) []byte {
	var prefix string
	switch scenario {
	case "malformed":
		return []byte(`{"private-capture":"corrupt"}`)
	case "shadow-draft-alias":
		return append([]byte(`{"Kind":"private-capture",`), draft[1:]...)
	case "shadow-draft-duplicate":
		return append([]byte(`{"kind":"private-capture",`), draft[1:]...)
	case "shadow-draft-unknown":
		return append([]byte(`{"private-capture":"secret",`), draft[1:]...)
	case "shadow-worker-alias":
		prefix = `{"WorkerSessionId":"private-capture",`
	case "shadow-attempt-duplicate":
		prefix = `{"attemptId":"private-capture",`
	case "shadow-lineage-alias":
		payload = []byte(strings.Replace(string(payload), `"lineage":{`, `"lineage":{"PredecessorWorkerSessionId":"private-capture",`, 1))
	case "shadow-lineage-duplicate":
		payload = []byte(strings.Replace(string(payload), `"lineage":{`, `"lineage":{"predecessorWorkerSessionId":"private-capture",`, 1))
	case "shadow-opening-unknown":
		prefix = `{"private-capture":"secret",`
	default:
		return draft
	}
	if prefix != "" {
		payload = append([]byte(prefix), payload[1:]...)
	}
	encoded, _ := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: payload})
	return encoded
}

// The coordinator's store collaborator commits a detached row, then loses
// the acknowledgement. Load supplies either the exact row or a disputed fact.
type interruptAckStore struct {
	interruptInputStore
	failPhase        string
	mutate           func(*recordings.WorkerControlOperationRecord)
	loadErr          error
	loads            int
	rejectCompletion bool
	conflictPhase    string
}

func (s *interruptAckStore) BeginWorkerControlOperation(ctx context.Context, intent recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	accepted, created, err := s.interruptInputStore.BeginWorkerControlOperation(ctx, intent)
	if err == nil && s.failPhase == "INTENT" {
		return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerRecordingPersistence
	}
	return accepted, created, err
}

func (s *interruptAckStore) AdvanceWorkerControlOperation(ctx context.Context, next recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	if s.rejectCompletion && next.Operation.Phase == "COMPLETED" {
		return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
	}
	accepted, err := s.interruptInputStore.AdvanceWorkerControlOperation(ctx, next, expected)
	if err == nil && next.Operation.Phase == s.conflictPhase {
		return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerControlConflict
	}
	if err == nil && next.Operation.Phase == s.failPhase {
		return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
	}
	return accepted, err
}

// Only the coordinator is real. Its store and execution collaborators are
// controlled; a delivered host restart belongs to I-T4, not this component.
func TestInterruptSyncedAdmissionSurvivesMissingCompletionAndReplayCache(t *testing.T) {
	t.Parallel()
	fixture := newInterruptRaceCharacterizationFixture(t, "missing-completion", false)
	r := fixture.registry.(*registry)
	store := &interruptAckStore{rejectCompletion: true}
	r.operations = store
	capture := exactCaptureIdentity()
	capture.WorkerSessionID, capture.FactorySessionID = fixture.sourceID, ""
	pub := r.publicationFor(fixture.sourceID)
	pub.mu.Lock()
	pub.capture = capture
	pub.mu.Unlock()
	req := workersessions.InterruptRequest{RequestID: "missing-completion", SourceWorkerSessionID: fixture.sourceID, SuccessorWorkerSessionID: fixture.successorID, ReplacementMessage: "replacement"}
	outcomes := startInterruptCharacterization(t, fixture.registry, req)
	fixture.boundary.waitCancellation(t, fixture.sourceDispatch)
	assertBoundaryEffects(t, fixture.boundary, 1, 1, "source join barrier")
	fixture.boundary.releaseCancellation(fixture.sourceDispatch)
	fixture.boundary.waitReturned(t, fixture.sourceDispatch)
	outcome := <-outcomes
	source := <-fixture.sourceResult
	if !errors.Is(outcome.err, recordings.ErrWorkerRecordingPersistence) || !outcome.result.Accepted || len(store.records) != 3 {
		t.Fatalf("missing completion outcome=%#v err=%v rows=%d", outcome.result, outcome.err, len(store.records))
	}
	// Join the fake successor before removing live handles; the durable reply
	// must retain its admission state even though it has since completed.
	completed := outcome
	completed.err = nil
	assertInterruptWins(t, fixture, completed, source)
	want, err := decodeInterruptOutcome(req, store.records[2])
	if err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.interruptReplays = nil
	delete(r.publications, fixture.sourceID)
	delete(r.sessions, fixture.sourceID)
	delete(r.supervisions, fixture.sourceID)
	delete(r.sessions, fixture.successorID)
	r.mu.Unlock()
	r.logs = &LogReader{reader: &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
		RecordingID: capture.RecordingID, WorkerSessionID: capture.WorkerSessionID,
		RecordingGenerationID: capture.RecordingGenerationID, OwnerEpoch: capture.OwnerEpoch,
	}}}
	for range 2 {
		replayed, err := r.Interrupt(t.Context(), req)
		if err != nil || !reflect.DeepEqual(replayed, want) || len(store.records) != 3 {
			t.Fatalf("durable admission replay=%#v err=%v rows=%d", replayed, err, len(store.records))
		}
		replayed.Source.SuccessorWorkerSessionID = "caller-mutation"
	}
	assertBoundaryEffects(t, fixture.boundary, 2, 1, "admission replay without cache")
}

func (s *interruptAckStore) LoadWorkerControlOperation(ctx context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	s.loads++
	if s.loadErr != nil {
		return recordings.WorkerControlOperationRecord{}, s.loadErr
	}
	record, err := s.interruptInputStore.LoadWorkerControlOperation(ctx, key)
	if err == nil && s.mutate != nil {
		s.mutate(&record)
	}
	return record, err
}

func TestInterruptUncertainPhaseAcknowledgementJoinsAndAdmitsOnce(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"INTENT", "SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			fixture := newInterruptRaceCharacterizationFixture(t, "uncertain-"+phase, false)
			r := fixture.registry.(*registry)
			store := &interruptAckStore{failPhase: phase}
			r.operations = store
			capture := exactCaptureIdentity()
			capture.WorkerSessionID, capture.FactorySessionID = fixture.sourceID, ""
			pub := r.publicationFor(fixture.sourceID)
			pub.mu.Lock()
			pub.capture = capture
			pub.mu.Unlock()
			req := workersessions.InterruptRequest{RequestID: "uncertain-request", SourceWorkerSessionID: fixture.sourceID, SuccessorWorkerSessionID: fixture.successorID, ReplacementMessage: "replacement"}
			outcomes := startInterruptCharacterization(t, fixture.registry, req)
			fixture.boundary.waitCancellation(t, fixture.sourceDispatch)
			assertBoundaryEffects(t, fixture.boundary, 1, 1, "source still joining")
			fixture.boundary.releaseCancellation(fixture.sourceDispatch)
			fixture.boundary.waitReturned(t, fixture.sourceDispatch)
			outcome := <-outcomes
			source := <-fixture.sourceResult
			assertInterruptWins(t, fixture, outcome, source)
			if store.loads != 2 || len(store.records) != 4 {
				// One initial lookup and one reconciliation; no phase re-append.
				t.Fatalf("loads=%d rows=%d", store.loads, len(store.records))
			}
			r.mu.Lock()
			r.interruptReplays = nil
			r.mu.Unlock()
			replayed, err := r.Interrupt(t.Context(), req)
			if err != nil || !replayed.Accepted || replayed.Successor.ID != req.SuccessorWorkerSessionID {
				t.Fatalf("reconciled outcome replay=%#v err=%v", replayed, err)
			}
			assertBoundaryEffects(t, fixture.boundary, 2, 1, "reconciled replay")
		})
	}
}

func TestInterruptUncertainPhaseRefusesDisputedReload(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"unavailable", "recording", "worker", "scope", "generation", "epoch", "attempt", "revision", "phase", "digest", "request", "successor", "mode", "input-ref", "result", "failure"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			r, plan, original := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			before := operation.Detached()
			store := &interruptAckStore{interruptInputStore: *original, failPhase: "SOURCE_STOPPED"}
			if field == "unavailable" {
				store.loadErr = recordings.ErrWorkerRecordingPersistence
			} else {
				store.mutate = func(record *recordings.WorkerControlOperationRecord) { disputeInterruptPhase(field, record) }
			}
			r.operations = store
			err = r.advanceInterruptPhase(t.Context(), operation, "SOURCE_STOPPED")
			if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || !reflect.DeepEqual(*operation, before) || store.loads != 1 || len(store.records) != 2 {
				t.Fatalf("disputed %s: operation=%#v err=%v loads=%d rows=%d", field, operation, err, store.loads, len(store.records))
			}
			assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
		})
	}
}

func TestInterruptPhaseConflictDoesNotReloadOrAuthorizeEffects(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			r, plan, original := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			store := &interruptAckStore{interruptInputStore: *original, conflictPhase: phase}
			r.operations = store
			for _, prior := range []string{"SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED"} {
				before := operation.Detached()
				err = r.advanceInterruptPhase(t.Context(), operation, prior)
				if prior == phase {
					if !errors.Is(err, recordings.ErrWorkerControlConflict) || !reflect.DeepEqual(*operation, before) || store.loads != 0 {
						t.Fatalf("conflict gained authority: phase=%s err=%v loads=%d operation=%#v", phase, err, store.loads, operation)
					}
					assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestInterruptUncertainIntentRefusesDisputedReloadBeforeEffects(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"unavailable", "recording", "worker", "scope", "generation", "epoch", "attempt", "revision", "phase", "digest", "request", "successor", "mode", "input-ref", "result", "failure"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			r, plan, original := newDurableInterruptFixture(t)
			store := &interruptAckStore{interruptInputStore: *original, failPhase: "INTENT"}
			if field == "unavailable" {
				store.loadErr = recordings.ErrWorkerRecordingPersistence
			} else {
				store.mutate = func(record *recordings.WorkerControlOperationRecord) { disputeInterruptPhase(field, record) }
			}
			r.operations = store
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			result, err := r.runInterrupt(plan)
			if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || result.Phase != workersessions.InterruptPhaseValidation || result.Accepted || calls != 0 || store.loads != 1 || len(store.records) != 1 {
				t.Fatalf("disputed intent %s: result=%#v err=%v cancels=%d loads=%d rows=%d", field, result, err, calls, store.loads, len(store.records))
			}
			if result.Source.State != workersessions.StateRunning {
				t.Fatalf("uncertain intent stopped source: %#v", result.Source)
			}
			assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
		})
	}
}

func TestInterruptIntentConflictDoesNotReloadOrAuthorizeEffects(t *testing.T) {
	t.Parallel()
	r, plan, original := newDurableInterruptFixture(t)
	store := &interruptAckStore{interruptInputStore: *original}
	store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
		return recordings.ErrWorkerControlConflict
	}
	r.operations = store
	result, err := r.runInterrupt(plan)
	if !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) || result.Source.State != workersessions.StateRunning || store.loads != 0 || len(store.records) != 0 {
		t.Fatalf("conflict gained authority: result=%#v err=%v loads=%d rows=%d", result, err, store.loads, len(store.records))
	}
	assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
}

func TestInterruptReconciledIntentRechecksAttemptBeforeEffects(t *testing.T) {
	t.Parallel()
	r, plan, original := newDurableInterruptFixture(t)
	store := &interruptAckStore{interruptInputStore: *original, failPhase: "INTENT"}
	store.mutate = func(*recordings.WorkerControlOperationRecord) {
		plan.supervision.dispatchID = "later-attempt"
	}
	r.operations = store
	calls := 0
	plan.supervision.installCancel(func() { calls++ })
	result, err := r.runInterrupt(plan)
	if !errors.Is(err, workersessions.ErrInterruptSourceConflict) || result.Phase != workersessions.InterruptPhaseValidation || calls != 0 || len(store.records) != 2 || store.records[1].Operation.Phase != "FAILED" {
		t.Fatalf("reconciled intent lost fence: result=%#v err=%v cancels=%d rows=%d", result, err, calls, len(store.records))
	}
	assertNoSuccessor(t, r, plan.request.SuccessorWorkerSessionID)
}

func disputeInterruptPhase(field string, record *recordings.WorkerControlOperationRecord) {
	switch field {
	case "recording", "worker", "scope", "generation", "epoch":
		changeCaptureIdentity(&record.Target, field)
	case "attempt":
		record.Target.ExpectedAttemptID = "different-attempt"
	case "revision":
		record.Revision++
	case "phase":
		record.Operation.Phase = "SUCCESSOR_ADMITTED"
	case "digest":
		record.Operation.InputDigest = "different-digest"
	case "request":
		record.Operation.RequestID = "different-request"
	case "successor":
		record.Operation.SuccessorWorkerSessionID = "different-successor"
	case "mode":
		record.Operation.ResumeMode = "recorded"
	case "input-ref":
		record.InputArtifactRef = "different-input"
	case "result":
		record.Result = []byte(`{"different":"snapshot"}`)
	case "failure":
		record.FailureCode = "different-failure"
	}
}
