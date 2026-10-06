package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
			if variant == "failed" && (!errors.Is(err, workersessions.ErrForceTerminationUnconfirmed) || strings.Contains(err.Error(), "private process detail")) {
				t.Fatalf("force failure lacks safe public identity: %v", err)
			}
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
	if !errors.Is(got.err, workersessions.ErrForceTerminationUnconfirmed) || !errors.Is(got.err, context.DeadlineExceeded) {
		t.Fatalf("force deadline lost typed failure or cause: %v", got.err)
	}
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
			if !errors.Is(err, workersessions.ErrInvalidControlRecord) || !errors.Is(err, workersessions.ErrInvalidForceControl) || len(store.records) != 0 || owned.calls.Load() != 0 {
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
			installForceContinuationAssociation(r)
			assertForceSuccessorRefused(t.Context(), t, r)
			if !s.controlPersistenceLost || s.forceJournalPending != 0 {
				t.Fatal("journal failure did not leave admission sealed after the operation finished")
			}
		})
	}
}

func installForceContinuationAssociation(r *registry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions["worker"]
	session.ProviderSessionAssociation = &workersessions.ProviderSessionAssociation{
		WorkerSessionID: "worker", DispatchID: "attempt", AttemptID: "attempt",
		Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "captured-provider-session"},
	}
	r.sessions["worker"] = session
}

func assertForceSuccessorRefused(ctx context.Context, t *testing.T, r *registry) {
	t.Helper()
	result, err := r.Continue(ctx, workersessions.ContinueRequest{
		RequestID: "continue-after-force", SourceWorkerSessionID: "worker", SuccessorWorkerSessionID: "successor", FollowUpInput: "continue",
	})
	if !errors.Is(err, workersessions.ErrContinuationSourceConflict) || result.Session.ID != "" {
		t.Fatalf("uncertain force admitted successor: %#v, %v", result, err)
	}
	if _, err := r.Get(ctx, workersessions.GetRequest{ID: "successor"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("refused successor was reserved: %v", err)
	}
}

func TestForceControlJournalAcknowledgementFencesSuccessorAdmission(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	installForceContinuationAssociation(r)
	store.advance = func(ctx context.Context, _ recordings.WorkerControlOperationRecord) error {
		// The exact execution has joined but its durable result has not yet
		// been acknowledged. A public continuation must not reserve a successor.
		assertForceSuccessorRefused(ctx, t, r)
		return nil
	}
	installForceDouble(s, func(ctx context.Context) (bool, error) {
		go r.completeSupervision(context.WithoutCancel(ctx), "worker", s, failedForceDispatch(), errors.New("signal exit"))
		return true, nil
	})
	result, err := r.Terminate(t.Context(), forceRequest())
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || s.forceJournalPending != 0 || s.controlPersistenceLost {
		t.Fatalf("acknowledged force retained an admission gate: %#v, %v", result, err)
	}
	r.mu.Lock()
	_, snapshotErr := r.snapshotContinuationSourceLocked(workersessions.ContinueRequest{SourceWorkerSessionID: "worker"})
	r.mu.Unlock()
	if snapshotErr != nil {
		t.Fatalf("acknowledged force prevented later continuation: %v", snapshotErr)
	}
}

func TestForceControlIntentLossSuppressesRetryButPreservesOrdinaryStop(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	s.retryBudget = 2
	s.attemptsMade = 1
	store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
		return recordings.ErrWorkerRecordingPersistence
	}
	owned := installForceDouble(s, func(context.Context) (bool, error) { return false, errors.New("unconfirmed cleanup") })
	result, err := r.Terminate(t.Context(), forceRequest())
	if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("degraded force = %#v, %v", result, err)
	}
	retryable := failedForceDispatch()
	retryable.Result.FailureMetadata = &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyRetryable, Type: workers.WorkFailureTypeInternalServerError}
	if r.claimRetryAttempt(s, "", retryable, nil) {
		t.Fatal("persistence loss admitted an automatic retry")
	}
	if _, allowed := r.prepareRetryAttempt("worker", s); allowed {
		t.Fatal("preselected retry bypassed persistence-loss fencing")
	}
	for _, requestID := range []string{"kill-request", "another-kill-request"} {
		repeat := forceRequest()
		repeat.RequestID = requestID
		replayed, replayErr := r.Terminate(t.Context(), repeat)
		if !errors.Is(replayErr, recordings.ErrWorkerRecordingPersistence) || replayed.Outcome != workersessions.ControlOutcomeFailed || owned.calls.Load() != 1 {
			t.Fatalf("uncertain force repeated its effect: %#v, %v, signals %d", replayed, replayErr, owned.calls.Load())
		}
	}
	s.installCancel(func() { r.commitControlTerminal("worker", workersessions.StateCanceled); s.signalDone() })
	r.publications["worker"].capture = recordings.WorkerControlTarget{}
	ordinary, stopErr := r.Cancel(t.Context(), workersessions.ControlRequest{ID: "worker"})
	if stopErr != nil || ordinary.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("persistence seal prevented live ordinary stop: %#v, %v", ordinary, stopErr)
	}
}

func TestForceControlFailedEffectWaitsForJournalBeforeRetryDecision(t *testing.T) {
	t.Parallel()
	for _, persistenceLost := range []bool{false, true} {
		t.Run(strconv.FormatBool(persistenceLost), func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			s.retryBudget, s.attemptsMade = 2, 1
			if persistenceLost {
				store.advanceErr = recordings.ErrWorkerRecordingPersistence
			}
			completed := make(chan struct{})
			retryable := failedForceDispatch()
			retryable.Result.FailureMetadata = &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyRetryable, Type: workers.WorkFailureTypeInternalServerError}
			installForceDouble(s, func(ctx context.Context) (bool, error) {
				go func() {
					r.completeSupervision(context.WithoutCancel(ctx), "worker", s, retryable, errors.New("retryable provider failure"))
					close(completed)
				}()
				return false, errors.New("unconfirmed tree cleanup")
			})
			result, err := r.Terminate(t.Context(), forceRequest())
			awaitStopSignal(t, completed)
			if result.Outcome != workersessions.ControlOutcomeFailed || err == nil {
				t.Fatalf("failed effect = %#v, %v", result, err)
			}
			current, getErr := r.Get(t.Context(), workersessions.GetRequest{ID: "worker"})
			wantState := workersessions.StateRunning
			if persistenceLost {
				wantState = workersessions.StateFailed
			}
			if getErr != nil || current.State != wantState || s.retryPending == persistenceLost {
				t.Fatalf("journal retry decision: state %s, retry %t, persistence lost %t, %v", current.State, s.retryPending, persistenceLost, getErr)
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
	if !errors.Is(got.err, workersessions.ErrForceTerminationUnconfirmed) || !errors.Is(got.err, context.DeadlineExceeded) {
		t.Fatalf("force join deadline lost typed failure or cause: %v", got.err)
	}
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

func saveForceResult(t *testing.T, r *registry, store *stopOperationStore, outcome workersessions.ControlOutcome) workersessions.ControlResult {
	t.Helper()
	target, err := r.freezeControlTarget("worker")
	if err != nil {
		t.Fatal(err)
	}
	intent := stopIntent(forceRequest(), workersessions.ControlActionTerminate, target)
	if _, _, err := store.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	result := workersessions.ControlResult{Session: workersessions.Session{ID: "worker", State: workersessions.StateTerminated}, Action: workersessions.ControlActionTerminate, Forced: true, Outcome: outcome, DispatchID: "attempt"}
	var stopErr error
	if outcome == workersessions.ControlOutcomeFailed {
		result.Session.State = workersessions.StateRunning
		stopErr = errRuntimeAttemptControlUnavailable
	}
	if err := r.commitStopResult(t.Context(), intent, result, stopErr); err != nil {
		t.Fatal(err)
	}
	return result
}

func archiveForceFixture(r *registry) *controlCaptureReader {
	target := r.publications["worker"].capture
	reader := &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
		RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID, FactorySessionID: target.FactorySessionID,
		RecordingGenerationID: target.RecordingGenerationID, OwnerEpoch: target.OwnerEpoch,
	}}
	r.logs = &LogReader{reader: reader}
	delete(r.sessions, "worker")
	delete(r.supervisions, "worker")
	delete(r.publications, "worker")
	return reader
}

func TestForceControlRecoversOriginalOutcomeAfterReplacementOrRestart(t *testing.T) {
	t.Parallel()
	for _, archived := range []bool{false, true} {
		for _, outcome := range []workersessions.ControlOutcome{workersessions.ControlOutcomeApplied, workersessions.ControlOutcomeFailed} {
			t.Run(string(outcome)+strconv.FormatBool(archived), func(t *testing.T) {
				t.Parallel()
				r, _, store := newDurableStopFixture(t)
				want := saveForceResult(t, r, store, outcome)
				replacement := newSupervision("replacement", "")
				owned := installForceDouble(replacement, func(context.Context) (bool, error) { t.Error("replay signaled replacement"); return true, nil })
				r.supervisions["worker"] = replacement
				if archived {
					archiveForceFixture(r)
				}
				got, err := r.Terminate(t.Context(), forceRequest())
				if !reflect.DeepEqual(got, want) || (outcome == workersessions.ControlOutcomeApplied && err != nil) ||
					(outcome == workersessions.ControlOutcomeFailed && (!errors.Is(err, errRuntimeAttemptControlUnavailable) || !errors.Is(err, workersessions.ErrForceTerminationUnconfirmed))) || owned.calls.Load() != 0 || len(store.records) != 2 {
					t.Fatalf("recovered outcome = %#v, %v; want %#v", got, err, want)
				}
			})
		}
	}
}

func TestForceControlRecoveryRejectsChangedTupleAndCorruptSavedResult(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"attempt", "epoch", "generation", "scope", "action", "digest", "request", "forced", "result-id", "result-state", "invalid-state", "failure", "unknown-field", "duplicate-id", "aliased-id"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			want := saveForceResult(t, r, store, workersessions.ControlOutcomeApplied)
			owned := installForceDouble(s, func(context.Context) (bool, error) { t.Error("corrupt replay reached effects"); return true, nil })
			req := forceRequest()
			corruptForceReplay(variant, &req, &want, &store.records[1])
			got, err := r.Terminate(t.Context(), req)
			if !errors.Is(err, workersessions.ErrInvalidState) || got.Outcome != workersessions.ControlOutcomeFailed || owned.calls.Load() != 0 || len(store.records) != 2 {
				t.Fatalf("corrupt replay = %#v, %v", got, err)
			}
		})
	}
}

func corruptForceReplay(variant string, req *workersessions.ControlRequest, result *workersessions.ControlResult, record *recordings.WorkerControlOperationRecord) {
	switch variant {
	case "attempt":
		req.ExpectedAttemptID = "replacement"
	case "epoch":
		record.Target.OwnerEpoch = "replacement"
	case "generation":
		record.Target.RecordingGenerationID = "replacement"
	case "scope":
		record.Target.FactorySessionID = "another-factory"
	case "action":
		record.Operation.Action = "terminate"
	case "digest":
		record.Operation.InputDigest = strings.Repeat("0", 64)
	case "request":
		record.Operation.RequestID = "another-request"
	case "failure":
		record.FailureCode = "STOP_FAILED"
	}
	corruptForceReplayPayload(variant, result, record)
}

func corruptForceReplayPayload(variant string, result *workersessions.ControlResult, record *recordings.WorkerControlOperationRecord) {
	switch variant {
	case "forced":
		result.Forced = false
	case "result-id":
		result.Session.ID = "another-worker"
	case "result-state":
		result.Session.State = workersessions.StateRunning
	case "invalid-state":
		result.Session.State = "PRIVATE_STATE"
	}
	if variant == "forced" || variant == "result-id" || variant == "result-state" || variant == "invalid-state" {
		record.Result, _ = json.Marshal(result)
	}
	switch variant {
	case "unknown-field":
		record.Result = append([]byte(`{"private":"secret",`), record.Result[1:]...)
	case "duplicate-id":
		record.Result = []byte(strings.Replace(string(record.Result), `"ID":`, `"ID":"foreign","ID":`, 1))
	case "aliased-id":
		record.Result = []byte(strings.Replace(string(record.Result), `"ID":`, `"id":`, 1))
	}
}

func TestForceControlPendingIntentNeedsItsOriginalLiveCapability(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"live", "replaced", "archived"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			t.Cleanup(s.signalDone)
			target, _ := r.freezeControlTarget("worker")
			intent := stopIntent(forceRequest(), workersessions.ControlActionTerminate, target)
			if _, _, err := store.BeginWorkerControlOperation(t.Context(), intent); err != nil {
				t.Fatal(err)
			}
			owned := installForceDouble(s, func(ctx context.Context) (bool, error) {
				go r.completeSupervision(context.WithoutCancel(ctx), "worker", s, failedForceDispatch(), errors.New("signal exit"))
				return true, nil
			})
			switch variant {
			case "replaced":
				s.dispatchID = "replacement"
			case "archived":
				archiveForceFixture(r)
			}
			result, err := r.Terminate(t.Context(), forceRequest())
			if variant == "live" {
				if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || owned.calls.Load() != 1 || len(store.records) != 2 {
					t.Fatalf("live pending force = %#v, %v", result, err)
				}
			} else if !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || owned.calls.Load() != 0 || len(store.records) != 1 {
				t.Fatalf("ownerless pending force = %#v, %v", result, err)
			}
		})
	}
}

func TestForceControlArchivedRecoveryChecksSelectedCaptureAndScope(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"matching", "generation", "epoch", "scope", "missing", "unreadable"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			r, _, store := newDurableStopFixture(t)
			r.publications["worker"].capture.FactorySessionID = "factory"
			want := saveForceResult(t, r, store, workersessions.ControlOutcomeApplied)
			reader := archiveForceFixture(r)
			req := forceRequest()
			req.FactorySessionID = "factory"
			switch variant {
			case "generation":
				reader.entry.RecordingGenerationID = "new-generation"
			case "epoch":
				reader.entry.OwnerEpoch = "new-owner"
			case "scope":
				req.FactorySessionID = "other-factory"
			case "missing":
				reader.err = os.ErrNotExist
			case "unreadable":
				reader.err = errors.New("private-journal-path")
			}
			got, err := r.Terminate(t.Context(), req)
			if variant == "matching" {
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("archived scoped result = %#v, %v", got, err)
				}
			} else if err == nil || got.Outcome != workersessions.ControlOutcomeFailed || strings.Contains(err.Error(), "private-journal-path") {
				t.Fatalf("archived altered capture = %#v, %v", got, err)
			}
			if len(store.records) != 2 || len(r.sessions) != 0 {
				t.Fatal("archived recovery mutated history or reconstructed execution")
			}
		})
	}
}
