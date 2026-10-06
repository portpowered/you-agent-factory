package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type forceAttemptDouble struct {
	force func(context.Context) (bool, error)
	calls atomic.Int32
}

func (f *forceAttemptDouble) ForceKill(ctx context.Context) (bool, error) {
	f.calls.Add(1)
	return f.force(ctx)
}

func forceRequest() workersessions.ControlRequest {
	return workersessions.ControlRequest{ID: "worker", Force: true, RequestID: "kill-request", ExpectedAttemptID: "attempt"}
}

func installForceDouble(s *supervision, force func(context.Context) (bool, error)) *forceAttemptDouble {
	control := &forceAttemptDouble{force: force}
	s.providerAttempt = &providerAttemptControl{control: control}
	return control
}

func failedForceDispatch() workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{DispatchID: "attempt", TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{Outcome: workers.OutcomeFailed}}
}

type forceResponse struct {
	result workersessions.ControlResult
	err    error
}

func awaitForceResponse(t *testing.T, response <-chan forceResponse) forceResponse {
	t.Helper()
	select {
	case result := <-response:
		return result
	case <-time.After(30 * time.Second):
		t.Fatal("force did not return")
		return forceResponse{}
	}
}

func TestForceControlConfirmsTreeThenJoinsWorkerAndRecoversSameResult(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	t.Cleanup(s.signalDone)
	signaled, release := make(chan struct{}), make(chan struct{})
	completion := make(chan struct{})
	owned := installForceDouble(s, func(context.Context) (bool, error) {
		close(signaled)
		<-release
		return true, nil
	})
	ctx, disconnect := context.WithCancel(t.Context())
	response := make(chan forceResponse, 1)
	go func() {
		result, err := r.Terminate(ctx, forceRequest())
		response <- forceResponse{result, err}
	}()
	awaitStopSignal(t, signaled)
	duplicate := make(chan forceResponse, 1)
	go func() {
		result, err := r.Terminate(t.Context(), forceRequest())
		duplicate <- forceResponse{result, err}
	}()
	if len(store.records) != 1 || store.records[0].Operation.Action != "kill" || store.records[0].Operation.Phase != "INTENT" {
		t.Fatal("force signaled before committing kill intent")
	}
	disconnect()
	go func() {
		r.completeSupervision(context.Background(), "worker", s, failedForceDispatch(), errors.New("signal exit"))
		close(completion)
	}()
	select {
	case <-completion:
		t.Fatal("worker completion passed an unconfirmed force")
	case <-response:
		t.Fatal("force returned before tree confirmation")
	default:
	}
	close(release)
	got := awaitForceResponse(t, response)
	awaitStopSignal(t, completion)
	assertConfirmedForce(t, got, store)
	concurrent := awaitForceResponse(t, duplicate)
	assertForceReplay(t, concurrent.result, concurrent.err, got.result, owned.calls.Load(), len(store.records))
	replay, err := r.Terminate(t.Context(), forceRequest())
	assertForceReplay(t, replay, err, got.result, owned.calls.Load(), len(store.records))
}

func assertConfirmedForce(t *testing.T, got forceResponse, store *stopOperationStore) {
	t.Helper()
	if got.err != nil || got.result.Outcome != workersessions.ControlOutcomeApplied || !got.result.Forced || got.result.Session.State != workersessions.StateTerminated {
		t.Fatalf("confirmed force = %#v, %v", got.result, got.err)
	}
	if len(store.records) != 2 || store.records[1].Operation.Phase != "COMPLETED" {
		t.Fatal("joined force did not commit causal result")
	}
	if cause := committedStopCause(store.records, store.records[1].Target, workersessions.StateTerminated); cause == nil || *cause != "OPERATOR_KILL" {
		t.Fatalf("committed force cause = %v", cause)
	}
}

func assertForceReplay(t *testing.T, replay workersessions.ControlResult, err error, original workersessions.ControlResult, calls int32, records int) {
	t.Helper()
	if err != nil || replay.Outcome != original.Outcome || replay.Session.State != original.Session.State || !replay.Forced || calls != 1 || records != 2 {
		t.Fatalf("duplicate force = %#v, %v, signals %d", replay, err, calls)
	}
}

func TestForceControlDeclineAndFailureNeverCancelOrInventTerminality(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"unattached", "retired", "declined", "failed"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			cancelCalls := 0
			s.installCancel(func() { cancelCalls++ })
			owned := installForceDouble(s, func(context.Context) (bool, error) {
				if variant == "failed" {
					return true, errors.New("private process detail")
				}
				return false, nil
			})
			switch variant {
			case "unattached":
				s.providerAttempt.control = nil
			case "retired":
				s.providerAttempt.retired = true
			}
			got, err := r.Terminate(t.Context(), forceRequest())
			assertUnsuccessfulForce(t, variant, got, err, s, cancelCalls)
			if (variant == "unattached" || variant == "retired") && owned.calls.Load() != 0 {
				t.Fatal("unavailable capability signaled")
			}
			if committedStopCause(store.records, store.records[0].Target, workersessions.StateTerminated) != nil {
				t.Fatal("unconfirmed kill invented operator cause")
			}
			// The same exact attempt retains ordinary cancellation after decline.
			s.installCancel(func() { cancelCalls++; r.commitControlTerminal("worker", workersessions.StateCanceled); s.signalDone() })
			// This one-operation store intentionally rejects the new cancel key;
			// use the component's uncaptured path for compatibility proof.
			r.publications["worker"].capture = recordings.WorkerControlTarget{}
			ordinary, err := r.Cancel(t.Context(), workersessions.ControlRequest{ID: "worker"})
			if err != nil || ordinary.Outcome != workersessions.ControlOutcomeApplied || cancelCalls != 1 {
				t.Fatalf("ordinary cancellation after decline = %#v, %v", ordinary, err)
			}
		})
	}
}

func assertUnsuccessfulForce(t *testing.T, variant string, got workersessions.ControlResult, err error, s *supervision, cancels int) {
	t.Helper()
	want := workersessions.ControlOutcomeUnsupported
	if variant == "failed" {
		want = workersessions.ControlOutcomeFailed
		if !errors.Is(err, errRuntimeAttemptControlUnavailable) {
			t.Fatalf("unsafe failure = %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != want || !got.Forced || got.Session.State != workersessions.StateRunning || cancels != 0 || s.requestedAction != "" || s.controlActive || s.forceConfirmed {
		t.Fatalf("unsuccessful force mutated state: %#v, cancel %d", got, cancels)
	}
}

func TestForceControlStaleGenerationAndChangedTupleHaveNoEffects(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"attempt", "slot", "supervision", "capture", "conflict"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			owned := installForceDouble(s, func(context.Context) (bool, error) { return true, nil })
			req := forceRequest()
			if variant == "attempt" {
				req.ExpectedAttemptID = "stale"
			} else {
				store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
					switch variant {
					case "slot":
						s.providerAttempt = &providerAttemptControl{control: owned}
					case "supervision":
						r.supervisions["worker"] = newSupervision("attempt", "")
					case "capture":
						r.publications["worker"].capture.OwnerEpoch = "replacement"
					case "conflict":
						return recordings.ErrWorkerControlConflict
					}
					return nil
				}
			}
			got, err := r.Terminate(t.Context(), req)
			if !errors.Is(err, workersessions.ErrInvalidState) || got.Outcome != workersessions.ControlOutcomeFailed || owned.calls.Load() != 0 || s.controlActive || r.sessions["worker"].State != workersessions.StateRunning {
				t.Fatalf("stale force = %#v, %v, signals %d", got, err, owned.calls.Load())
			}
		})
	}
}

func TestForceControlInjectedDeadlineDoesNotApplyLateSuccess(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
	r.scheduler = clock
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	installForceDouble(s, func(context.Context) (bool, error) {
		close(entered)
		<-release // deliberately broken edge ignores observation cancellation
		close(returned)
		return true, nil
	})
	response := make(chan forceResponse, 1)
	go func() {
		result, err := r.Terminate(t.Context(), forceRequest())
		response <- forceResponse{result, err}
	}()
	awaitStopSignal(t, entered)
	clock.SetTick(10)
	got := awaitForceResponse(t, response)
	close(release)
	awaitStopSignal(t, returned)
	if !errors.Is(got.err, errRuntimeAttemptControlUnavailable) || got.result.Outcome != workersessions.ControlOutcomeFailed || got.result.Session.State != workersessions.StateRunning || s.forceConfirmed || s.forcePending || s.controlActive {
		t.Fatalf("deadline fabricated success = %#v, %v", got.result, got.err)
	}
	if len(store.records) != 2 || store.records[1].Operation.Phase != "FAILED" {
		t.Fatal("deadline did not persist authoritative failure")
	}
}

func TestForceControlRequiresValidTupleBeforeAnyJournalOrEffect(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"request", "attempt", "false-attempt", "cancel", "pause", "resume"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			owned := installForceDouble(s, func(context.Context) (bool, error) { return true, nil })
			req := forceRequest()
			control := r.Terminate
			switch variant {
			case "request":
				req.RequestID = " "
			case "attempt":
				req.ExpectedAttemptID = " "
			case "false-attempt":
				req.Force = false
			case "cancel":
				control = r.Cancel
			case "pause":
				control = r.Pause
			case "resume":
				control = r.Resume
			}
			_, err := control(t.Context(), req)
			if !errors.Is(err, workersessions.ErrInvalidControlRecord) || len(store.records) != 0 || owned.calls.Load() != 0 {
				t.Fatalf("invalid tuple reached effects: %v", err)
			}
		})
	}
}

func TestForceControlRuntimeUsesPhysicalAttemptAndNeverCancellation(t *testing.T) {
	t.Parallel()
	r, id, attempt, cancels := newFrozenRuntimeControlFixture(t)
	owned := &forceAttemptDouble{force: func(ctx context.Context) (bool, error) {
		go func() {
			_ = attempt.Complete(context.WithoutCancel(ctx), failedForceDispatch(), errors.New("signal exit"))
		}()
		return true, nil
	}}
	attempt.observeProviderAttemptControl(owned)
	result, err := r.Terminate(t.Context(), workersessions.ControlRequest{ID: id, RequestID: "runtime-kill", Force: true, ExpectedAttemptID: attempt.attemptID})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.DispatchID != "logical-dispatch" || result.Session.State != workersessions.StateTerminated || *cancels != 0 || owned.calls.Load() != 1 {
		t.Fatalf("Runtime force = %#v, %v, ordinary cancels %d", result, err, *cancels)
	}
}

func TestForceControlPersistenceLossKeepsLiveSafetyAndRefusesDurableSuccess(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"intent", "result"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			if phase == "intent" {
				store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
					return errors.New("private disk detail")
				}
			} else {
				store.advanceErr = errors.New("private disk detail")
			}
			owned := installForceDouble(s, func(ctx context.Context) (bool, error) {
				go r.completeSupervision(context.WithoutCancel(ctx), "worker", s, failedForceDispatch(), errors.New("signal exit"))
				return true, nil
			})
			result, err := r.Terminate(t.Context(), forceRequest())
			if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StateTerminated || owned.calls.Load() != 1 || len(store.failures) != 1 {
				t.Fatalf("degraded force = %#v, %v, signals %d", result, err, owned.calls.Load())
			}
			target, _ := r.freezeControlTarget("worker")
			intent := stopIntent(forceRequest(), workersessions.ControlActionTerminate, target)
			if committedStopCause(store.records, intent.Target, workersessions.StateTerminated) != nil {
				t.Fatal("uncommitted force acquired a durable operator cause")
			}
		})
	}
}

func TestForceControlWorkerJoinDeadlineNeverFabricatesTerminality(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
	r.scheduler = clock
	confirmed := make(chan struct{})
	installForceDouble(s, func(context.Context) (bool, error) {
		close(confirmed)
		return true, nil
	})
	response := make(chan forceResponse, 1)
	go func() {
		result, err := r.Terminate(t.Context(), forceRequest())
		response <- forceResponse{result, err}
	}()
	awaitStopSignal(t, confirmed)
	clock.SetTick(10)
	got := awaitForceResponse(t, response)
	if !errors.Is(got.err, errRuntimeAttemptControlUnavailable) || got.result.Outcome != workersessions.ControlOutcomeFailed || got.result.Session.State != workersessions.StateRunning || len(store.records) != 2 || store.records[1].Operation.Phase != "FAILED" {
		t.Fatalf("unjoined force = %#v, %v", got.result, got.err)
	}
}

func TestForceControlRuntimeRefusesReplacedOwnerAfterIntentSync(t *testing.T) {
	t.Parallel()
	for _, replacement := range []string{"handle", "scope", "dispatch"} {
		t.Run(replacement, func(t *testing.T) {
			t.Parallel()
			r, id, a, cancels := newFrozenRuntimeControlFixture(t)
			owned := &forceAttemptDouble{force: func(context.Context) (bool, error) { return true, nil }}
			a.observeProviderAttemptControl(owned)
			capture := exactCaptureIdentity()
			capture.WorkerSessionID, capture.FactorySessionID = publicWorkerID(id), "factory-a"
			r.publications[id].capture = capture
			r.operations = &stopOperationStore{begin: func(context.Context, recordings.WorkerControlOperationRecord) error {
				replaceFrozenRuntimeOwner(r, id, a, replacement)
				return nil
			}}
			result, err := r.Terminate(t.Context(), workersessions.ControlRequest{ID: id, Force: true, RequestID: "runtime-kill", ExpectedAttemptID: a.attemptID})
			if !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || owned.calls.Load() != 0 || *cancels != 0 || a.controlPending {
				t.Fatalf("stale Runtime force = %#v, %v, signals %d", result, err, owned.calls.Load())
			}
		})
	}
}

func TestForceControlFailurePreservesNaturalWorkerCompletion(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	completed := make(chan struct{})
	installForceDouble(s, func(ctx context.Context) (bool, error) {
		go func() {
			r.completeSupervision(context.WithoutCancel(ctx), "worker", s, failedForceDispatch(), errors.New("native failure"))
			close(completed)
		}()
		return false, errors.New("private signal failure")
	})
	result, err := r.Terminate(t.Context(), forceRequest())
	awaitStopSignal(t, completed)
	current, _ := r.Get(t.Context(), workersessions.GetRequest{ID: "worker"})
	if !errors.Is(err, errRuntimeAttemptControlUnavailable) || result.Outcome != workersessions.ControlOutcomeFailed || current.State != workersessions.StateFailed || s.forceConfirmed || s.requestedAction != "" {
		t.Fatalf("force failure rewrote natural completion: %#v, %v, current %s", result, err, current.State)
	}
	if len(store.records) != 2 || store.records[1].Operation.Phase != "FAILED" {
		t.Fatal("failed kill became a committed causal success")
	}
}
