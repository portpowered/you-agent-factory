package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newRuntimeForceJournalFixture(t *testing.T) (*registry, *runtimeAttempt, *stopOperationStore) {
	t.Helper()
	r, _, store := newDurableStopFixture(t)
	delete(r.supervisions, "worker")
	a := &runtimeAttempt{registry: r, workerID: "worker", dispatchID: "attempt", attemptID: "attempt",
		key:       workersessions.RuntimeAttemptKey{RuntimeID: "runtime-test", DispatchID: "attempt"},
		completed: make(chan struct{}), cancel: runtimeAttemptNoopCancellation}
	r.runtimeAttemptControls["worker"] = a
	r.runtimeAttemptOwners = map[workersessions.RuntimeAttemptKey]string{a.key: "worker"}
	r.latestRuntimeDispatchIDs = map[string]string{"worker": a.dispatchID}
	return r, a, store
}

func awaitStopCompletion(t *testing.T, completion <-chan error) error {
	t.Helper()
	select {
	case err := <-completion:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("Runtime completion did not return")
		return nil
	}
}

func assertRuntimeForceRetryRefused(ctx context.Context, t *testing.T, r *registry, a *runtimeAttempt) {
	t.Helper()
	req := workersessions.RuntimeAttemptRequest{ID: "retry-worker", AttemptID: "retry-attempt", Key: a.key,
		Execution: runtimeAttemptHandoff(a.dispatchID)}
	attempt, err := r.BeginRuntimeAttempt(ctx, req, r.execution, r.clock, r.scheduler, runtimeAttemptNoopCancellation)
	if attempt != nil || !errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch) {
		t.Fatalf("sealed dispatch admitted retry: %v, %v", attempt, err)
	}
	if _, err := r.Get(ctx, workersessions.GetRequest{ID: req.ID}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("sealed dispatch reserved replacement: %v", err)
	}
}

func TestForceRuntimeJournalAcknowledgementFencesRetryAndReleasesExecutionJoin(t *testing.T) {
	t.Parallel()
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "acknowledged", true: "persistence-lost"}[failure], func(t *testing.T) {
			t.Parallel()
			r, a, store := newRuntimeForceJournalFixture(t)
			completion := make(chan error, 1)
			a.providerControl = &forceAttemptDouble{force: func(ctx context.Context) (bool, error) {
				go func() {
					completion <- a.Complete(context.WithoutCancel(ctx), failedForceDispatch(), errors.New("signal exit"))
				}()
				return true, nil
			}}
			store.advance = func(ctx context.Context, _ recordings.WorkerControlOperationRecord) error {
				// A real coordinator join has finished, but Runtime must not yet
				// return from its completion callback and select another attempt.
				select {
				case <-a.completed:
				default:
					t.Error("force saved result before execution joined")
				}
				select {
				case err := <-completion:
					t.Errorf("Runtime returned before journal acknowledgement: %v", err)
				default:
				}
				assertRuntimeForceRetryRefused(ctx, t, r, a)
				assertRuntimeForceSiblingAdmitted(ctx, t, r)
				if failure {
					return recordings.ErrWorkerRecordingPersistence
				}
				return nil
			}
			result, err := r.Terminate(t.Context(), forceRequest())
			completionErr := awaitStopCompletion(t, completion)
			if result.Session.State != workersessions.StateTerminated {
				t.Fatalf("confirmed force state = %s", result.Session.State)
			}
			if failure {
				if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || !errors.Is(completionErr, recordings.ErrWorkerRecordingPersistence) || result.Outcome != workersessions.ControlOutcomeFailed {
					t.Fatalf("lost acknowledgement = %#v, %v; completion %v", result, err, completionErr)
				}
				assertRuntimeForceRetryRefused(t.Context(), t, r, a)
			} else {
				if err != nil || completionErr != nil || result.Outcome != workersessions.ControlOutcomeApplied {
					t.Fatalf("acknowledged force = %#v, %v; completion %v", result, err, completionErr)
				}
				assertRuntimeForceRetryAdmitted(t, r, a)
			}
		})
	}
}

func assertRuntimeForceSiblingAdmitted(ctx context.Context, t *testing.T, r *registry) {
	t.Helper()
	attempt, err := r.BeginRuntimeAttempt(ctx, workersessions.RuntimeAttemptRequest{
		ID: "sibling-worker", AttemptID: "sibling-attempt",
		Key:       workersessions.RuntimeAttemptKey{RuntimeID: "runtime-test", DispatchID: "sibling-dispatch"},
		Execution: runtimeAttemptHandoff("sibling-dispatch"),
	}, r.execution, r.clock, r.scheduler, runtimeAttemptNoopCancellation)
	if err != nil || attempt == nil {
		t.Fatalf("force journal sealed unrelated dispatch: %v, %v", attempt, err)
	}
	if err := attempt.Complete(ctx, runtimeAttemptCompletedDispatch("sibling-dispatch"), nil); err != nil {
		t.Fatal(err)
	}
}

func assertRuntimeForceRetryAdmitted(t *testing.T, r *registry, a *runtimeAttempt) {
	t.Helper()
	attempt, err := r.BeginRuntimeAttempt(t.Context(), workersessions.RuntimeAttemptRequest{
		ID: "retry-worker", AttemptID: "retry-attempt", Key: a.key, Execution: runtimeAttemptHandoff(a.dispatchID),
	}, r.execution, r.clock, r.scheduler, runtimeAttemptNoopCancellation)
	if err != nil || attempt == nil {
		t.Fatalf("acknowledged dispatch stayed sealed: %v, %v", attempt, err)
	}
	if err := attempt.Complete(t.Context(), runtimeAttemptCompletedDispatch(a.dispatchID), nil); err != nil {
		t.Fatal(err)
	}
}

func TestForceRuntimePersistenceLossAllowsOnlyOneSafetySignalAndOrdinaryStop(t *testing.T) {
	t.Parallel()
	r, a, store := newRuntimeForceJournalFixture(t)
	store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
		return recordings.ErrWorkerRecordingPersistence
	}
	owned := &forceAttemptDouble{force: func(context.Context) (bool, error) { return false, errors.New("unconfirmed tree") }}
	a.providerControl = owned
	for _, requestID := range []string{"kill-request", "second-kill"} {
		req := forceRequest()
		req.RequestID = requestID
		result, err := r.Terminate(t.Context(), req)
		if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StateRunning {
			t.Fatalf("degraded force = %#v, %v", result, err)
		}
	}
	if owned.calls.Load() != 1 {
		t.Fatalf("degraded force signaled %d times", owned.calls.Load())
	}
	assertRuntimeForceRetryRefused(t.Context(), t, r, a)
	completion := make(chan error, 1)
	cancelCalls := 0
	a.cancel = func(ctx context.Context) (workers.WorkstationDispatchCancelOutcome, error) {
		cancelCalls++
		go func() {
			completion <- a.Complete(context.WithoutCancel(ctx), runtimeAttemptCanceledDispatch(a.dispatchID), workers.ErrWorkstationDispatchCanceled)
		}()
		return workers.WorkstationDispatchCancelOutcomeCanceled, nil
	}
	result, err := r.Cancel(t.Context(), workersessions.ControlRequest{ID: "worker"})
	if result.Session.State != workersessions.StateCanceled || !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || cancelCalls != 1 {
		t.Fatalf("ordinary safety stopping was lost: %#v, %v, calls %d", result, err, cancelCalls)
	}
	if err := awaitStopCompletion(t, completion); !errors.Is(err, recordings.ErrWorkerRecordingPersistence) {
		t.Fatalf("Runtime lost its persistence seal: %v", err)
	}
	assertRuntimeForceRetryRefused(t.Context(), t, r, a)
}

func TestForceRuntimeStaleOwnerDoesNotSealReplacementAdmission(t *testing.T) {
	t.Parallel()
	r, id, a, _ := newFrozenRuntimeControlFixture(t)
	target, err := r.freezeControlTarget(id)
	if err != nil {
		t.Fatal(err)
	}
	replaceFrozenRuntimeOwner(r, id, a, "handle")
	finish, err := r.beginFrozenForceJournal(id, target)
	if finish != nil || !errors.Is(err, workersessions.ErrInvalidState) || len(r.runtimeForceJournals) != 0 {
		t.Fatalf("stale journal sealed replacement: admitted %t, %v", finish != nil, err)
	}
}

func TestForceRuntimeFailedSignalKeepsNaturalCompletionAndRetryAfterAcknowledgement(t *testing.T) {
	t.Parallel()
	r, a, store := newRuntimeForceJournalFixture(t)
	completion := make(chan error, 1)
	a.providerControl = &forceAttemptDouble{force: func(ctx context.Context) (bool, error) {
		go func() {
			completion <- a.Complete(context.WithoutCancel(ctx), failedForceDispatch(), errors.New("native failure"))
		}()
		return false, errors.New("unconfirmed tree")
	}}
	store.advance = func(ctx context.Context, _ recordings.WorkerControlOperationRecord) error {
		assertRuntimeForceRetryRefused(ctx, t, r, a)
		select {
		case err := <-completion:
			t.Errorf("natural completion bypassed journal acknowledgement: %v", err)
		default:
		}
		return nil
	}
	result, err := r.Terminate(t.Context(), forceRequest())
	if !errors.Is(err, errRuntimeAttemptControlUnavailable) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("failed force = %#v, %v", result, err)
	}
	if err := awaitStopCompletion(t, completion); err != nil {
		t.Fatalf("acknowledged signal failure sealed normal retry: %v", err)
	}
	current, err := r.Get(t.Context(), workersessions.GetRequest{ID: "worker"})
	if err != nil || current.State != workersessions.StateFailed || a.forceConfirmed || a.controlAction != "" {
		t.Fatalf("failed force rewrote natural result: %#v, %v", current, err)
	}
	assertRuntimeForceRetryAdmitted(t, r, a)
}

func TestForceRuntimeIntentPersistenceLossStopsAndKeepsDispatchSealedAfterCompletion(t *testing.T) {
	t.Parallel()
	r, a, store := newRuntimeForceJournalFixture(t)
	store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
		return recordings.ErrWorkerRecordingPersistence
	}
	completion := make(chan error, 1)
	owned := &forceAttemptDouble{force: func(ctx context.Context) (bool, error) {
		go func() {
			completion <- a.Complete(context.WithoutCancel(ctx), failedForceDispatch(), errors.New("signal exit"))
		}()
		return true, nil
	}}
	a.providerControl = owned
	result, err := r.Terminate(t.Context(), forceRequest())
	completionErr := awaitStopCompletion(t, completion)
	if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || !errors.Is(completionErr, recordings.ErrWorkerRecordingPersistence) ||
		result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StateTerminated || owned.calls.Load() != 1 || len(store.records) != 0 {
		t.Fatalf("unrecorded live stop = %#v, %v; completion %v", result, err, completionErr)
	}
	assertRuntimeForceRetryRefused(t.Context(), t, r, a)
	assertRuntimeForceSiblingAdmitted(t.Context(), t, r)
}

func TestForceRuntimeResolveReturnsOnlyConfirmedCancellation(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"confirmed", "persistence-lost", "unconfirmed"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, a, store := newRuntimeForceJournalFixture(t)
			nativeErr := errors.New("native signal exit")
			original := failedForceDispatch()
			original.Result.Output = "partial output"
			original.Result.StructuredResult = map[string]any{"partial": true}
			original.Result.StructuredResultPresent = true
			original.Result.FailureMetadata = &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyRetryable}
			original.Result.Continuation = &workers.ProviderContinuationRef{ProviderSessionID: "partial"}
			original.ProposedOutput = &workers.ProposedOutput{}
			completion := make(chan forceDispatchResolution, 1)
			a.providerControl = &forceAttemptDouble{force: func(ctx context.Context) (bool, error) {
				go func() {
					result, forced, err := a.Resolve(context.WithoutCancel(ctx), original, nativeErr)
					completion <- forceDispatchResolution{result, forced, err}
				}()
				if variant == "unconfirmed" {
					return false, errors.New("tree not joined")
				}
				return true, nil
			}}
			if variant == "persistence-lost" {
				store.advance = func(context.Context, recordings.WorkerControlOperationRecord) error {
					return recordings.ErrWorkerRecordingPersistence
				}
			}
			_, _ = r.Terminate(t.Context(), forceRequest())
			select {
			case resolved := <-completion:
				assertForceDispatchResolution(t, variant, resolved, nativeErr)
			case <-time.After(30 * time.Second):
				t.Fatal("completion resolution did not join")
			}
		})
	}
}

type forceDispatchResolution struct {
	result workers.WorkstationDispatchResult
	forced bool
	err    error
}

func assertForceDispatchResolution(t *testing.T, variant string, resolved forceDispatchResolution, nativeErr error) {
	t.Helper()
	result := resolved.result
	if resolved.forced != (variant != "unconfirmed") {
		t.Fatalf("force confirmation = %t for %s", resolved.forced, variant)
	}
	if variant == "unconfirmed" {
		if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed || result.Result.Output != "partial output" ||
			result.Result.FailureMetadata == nil || resolved.err != nil {
			t.Fatalf("unconfirmed force changed native completion: %#v, %v", result, resolved.err)
		}
		return
	}
	assertForceCanceledDispatch(t, result)

	if errors.Is(resolved.err, nativeErr) || (variant == "confirmed" && resolved.err != nil) ||
		(variant == "persistence-lost" && !errors.Is(resolved.err, recordings.ErrWorkerRecordingPersistence)) {
		t.Fatalf("completion error = %v for %s", resolved.err, variant)
	}
}

func assertForceCanceledDispatch(t *testing.T, result workers.WorkstationDispatchResult) {
	t.Helper()
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled || result.Result.Outcome != workers.OutcomeCanceled ||
		result.Result.Output != "" || result.Result.StructuredResultPresent || result.Result.StructuredResult != nil ||
		result.Result.FailureMetadata != nil || result.Result.Continuation != nil || result.ProposedOutput != nil || result.Cancellation == nil {
		t.Fatalf("confirmed force retained retry or routing output: %#v", result)
	}
}
