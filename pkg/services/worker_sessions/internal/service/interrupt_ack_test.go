package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// The coordinator's store collaborator commits a detached row, then loses
// the acknowledgement. Load supplies either the exact row or a disputed fact.
type interruptAckStore struct {
	interruptInputStore
	failPhase string
	mutate    func(*recordings.WorkerControlOperationRecord)
	loadErr   error
	loads     int
}

func (s *interruptAckStore) AdvanceWorkerControlOperation(ctx context.Context, next recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	accepted, err := s.interruptInputStore.AdvanceWorkerControlOperation(ctx, next, expected)
	if err == nil && next.Operation.Phase == s.failPhase {
		return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
	}
	return accepted, err
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
	for _, phase := range []string{"SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED"} {
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
