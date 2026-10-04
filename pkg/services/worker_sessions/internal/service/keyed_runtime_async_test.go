package service

import (
	"context"
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"reflect"
	"sync"
	"testing"
	"time"
)

func cancelEqualPhysicalAttempt(t *testing.T, a *perRuntimeAttemptFixture, controlA *perRuntimeCancellation) {
	t.Helper()
	controlled := make(chan error, 1)
	go func() {
		result, err := a.service.Cancel(context.Background(), workersessions.ControlRequest{ID: a.request.ID})
		if err == nil && (result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateCanceled || result.DispatchID != perRuntimeLogicalDispatchID) {
			err = fmt.Errorf("Cancel(A) = %#v, want APPLIED/CANCELED with logical dispatch", result)
		}
		controlled <- err
	}()
	if err := waitControlledSignal(controlA.invoked, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := a.attempt.Complete(context.Background(), runtimeAttemptCanceledDispatch(perRuntimeLogicalDispatchID), nil); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-controlled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("targeted cancellation did not join its completion")
	}
}

// A different runtime or dispatch does not authorize reusing a Worker identity
// within the same Factory Session, including after its history is retained.
func TestKeyedRuntimeWorkerIdentityDuplicateWithinFactorySession(t *testing.T) {
	t.Parallel()
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprintf("terminal=%t", terminal), func(t *testing.T) {
			t.Parallel()
			eventStore := newEventsAppender()
			sink := &perRuntimeAppendCapture{EventsAppender: eventStore}
			owner := newPerRuntimeAttemptFixture(t, "identity-owner", sink)
			peer := newPerRuntimeAttemptFixture(t, "identity-peer", sink, owner)
			state := workersessions.StateRunning
			if terminal {
				if err := owner.attempt.Complete(context.Background(), runtimeAttemptCompletedDispatch(perRuntimeLogicalDispatchID), nil); err != nil {
					t.Fatal(err)
				}
				state = workersessions.StateCompleted
			}
			before := assertPerRuntimeAttemptState(t, owner, state)
			observation := assertPerRuntimeObservation(t, owner, state)
			appends := sink.requestsFor("")
			topic := workersessions.Topic(owner.request.ID, owner.request.Execution.Execution.FactorySessionID)
			records := readPerRuntimeTopic(t, eventStore, topic)
			duplicate := owner.request
			duplicate.Key = workersessions.RuntimeAttemptKey{RuntimeID: "duplicate-runtime", DispatchID: "duplicate-dispatch"}
			duplicate.Execution = cloneWorkstationDispatchRequest(owner.request.Execution)
			duplicate.Execution.Execution.RuntimeID = duplicate.Key.RuntimeID
			duplicate.Execution.Execution.Dispatch.DispatchID = duplicate.Key.DispatchID
			duplicate.AttemptID = "duplicate-physical"
			attempt, err := owner.service.BeginRuntimeAttempt(context.Background(), duplicate, owner.service.execution, coverageClock{now: owner.clock}, owner.service.scheduler, owner.control.cancel)
			if attempt != nil || !errors.Is(err, workersessions.ErrSessionNotStartable) {
				t.Fatalf("same-session duplicate = %v, %v; want nil/ErrSessionNotStartable", attempt, err)
			}
			assertDuplicateWorkerHistoryUnchanged(t, owner, sink, eventStore, before, observation, appends, records)
			publishPerRuntimeProgress(t, peer, sink)
			if err := peer.attempt.Complete(context.Background(), runtimeAttemptCompletedDispatch(perRuntimeLogicalDispatchID), nil); err != nil {
				t.Fatal(err)
			}
			assertPerRuntimeFirstTerminal(t, peer, sink, workersessions.StateCompleted)
		})
	}
}

func assertDuplicateWorkerHistoryUnchanged(t *testing.T, owner *perRuntimeAttemptFixture, sink *perRuntimeAppendCapture, eventStore events.Service, before workersessions.Session, observation workersessions.Observation, appends []events.AppendRequest, records events.ReadResult) {
	t.Helper()
	if after := assertPerRuntimeAttemptState(t, owner, before.State); !reflect.DeepEqual(before, after) {
		t.Fatalf("duplicate changed retained owner: %#v", after)
	}
	if after := assertPerRuntimeObservation(t, owner, before.State); !reflect.DeepEqual(observation, after) {
		t.Fatalf("duplicate changed retained observation: %#v", after)
	}
	if !reflect.DeepEqual(appends, sink.requestsFor("")) || !reflect.DeepEqual(records, readPerRuntimeTopic(t, eventStore, workersessions.Topic(owner.request.ID, owner.request.Execution.Execution.FactorySessionID))) {
		t.Fatal("duplicate changed source-native publications or retained owner topic")
	}
}

func TestKeyedRuntimeAsyncAdmissionSelectsEffectsAndKeepsDirectScope(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	facts := platformclock.NewDeterministic(time.Date(2042, 1, 2, 3, 4, 5, 0, time.UTC), time.Second)
	scheduler := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
	started := make(chan workers.ExecuteRequest, 1)
	canceled := make(chan struct{})
	execution := coverageExecution{execute: func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		started <- request
		<-ctx.Done()
		close(canceled)
		return workers.ExecuteResult{Correlation: request.Correlation}, ctx.Err()
	}}
	req := workersessions.StartRequest{RequestID: "async-request", ID: "async-worker", Execution: dispatchHandoff("async-physical")}
	req.Execution.Execution.RuntimeID = "async-runtime"
	req.Execution.Execution.Model = "selected-model"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := r.AdmitRuntimeAttemptAsync(ctx, req, execution, facts, scheduler)
	if err != nil || result.Session.State != workersessions.StateRunning {
		t.Fatalf("admit: %#v, %v", result, err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	select {
	case request := <-started:
		if request.Correlation.DispatchID != "async-physical" || request.Correlation.AttemptID != "async-physical" || request.Target.Model.Name != "selected-model" {
			t.Errorf("selected direct request: %#v", request)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("selected execution was not started")
	}
	cancel()
	replay, err := r.AdmitRuntimeAttemptAsync(context.Background(), req, unusedExecution{t: t}, r.clock, r.scheduler)
	if err != nil || !reflect.DeepEqual(replay, result) {
		t.Fatalf("replay changed: %#v, %v", replay, err)
	}
	peer := startSelectedEffectsInvocation(t, r, "async-peer", "success", 2043, "peer-runtime")
	if err := r.CloseRuntimeAttempts(context.Background(), "async-runtime"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(30 * time.Second):
		t.Fatal("scope did not join direct attempt")
	}
	assertAsyncAdmissionTerminalFacts(t, r, req.ID)
	close(peer.release)
	if got := awaitSelectedEffectsInvocation(t, peer); got.Session.State != workersessions.StateCompleted {
		t.Fatalf("peer: %#v", got)
	}
}

func TestKeyedRuntimeAsyncAdmissionRejectsMissingEffectsBeforeReservation(t *testing.T) {
	for _, missing := range []string{"execution", "clock", "scheduler"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			r := newTestRegistry(t)
			var execution workers.Service = unusedExecution{t: t}
			var clock platformclock.Source = r.clock
			var scheduler platformclock.TimerSource = r.scheduler
			want := ErrMissingExecution
			switch missing {
			case "execution":
				execution = nil
			case "clock":
				clock, want = nil, ErrMissingClock
			case "scheduler":
				scheduler, want = nil, ErrMissingScheduler
			}
			req := workersessions.StartRequest{RequestID: "missing-request", ID: "missing-worker", Execution: dispatchHandoff("missing-dispatch")}
			if _, err := r.AdmitRuntimeAttemptAsync(context.Background(), req, execution, clock, scheduler); !errors.Is(err, want) {
				t.Fatalf("missing %s: %v", missing, err)
			}
			if _, err := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID}); !errors.Is(err, workersessions.ErrSessionNotFound) {
				t.Fatalf("reserved rejected session: %v", err)
			}
		})
	}
}

func assertAsyncAdmissionTerminalFacts(t *testing.T, r *registry, id string) {
	t.Helper()
	snapshot, err := r.Get(context.Background(), workersessions.GetRequest{ID: id})
	if err != nil || snapshot.State != workersessions.StateCanceled {
		t.Fatalf("terminal direct attempt: %#v, %v", snapshot, err)
	}
	observation, err := r.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: id})
	if err != nil || observation.StartedAt == nil || !observation.StartedAt.Equal(time.Date(2042, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("selected fact time: %#v, %v", observation, err)
	}
}

func TestKeyedRuntimeAsyncDeadlineUsesSelectedSchedulerAndRetainsFacts(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	req := workersessions.StartRequest{RequestID: "deadline-request", ID: "deadline-worker", Execution: dispatchHandoff("deadline-physical")}
	req.Execution.Execution.RuntimeID = "deadline-runtime"
	req.Execution.Execution.Timeout = 5 * time.Second
	facts := platformclock.NewDeterministic(time.Date(2042, 1, 2, 3, 4, 5, 0, time.UTC), time.Second)
	scheduler := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
	observer := newTerminalAppendObserver(r.events.(events.Service), workersessions.Topic(req.ID, req.Execution.Execution.FactorySessionID))
	r.events = observer
	started := make(chan struct{})
	execution := coverageExecution{execute: func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		close(started)
		<-ctx.Done()
		return workers.ExecuteResult{Correlation: request.Correlation}, ctx.Err()
	}}
	if result, err := r.AdmitRuntimeAttemptAsync(context.Background(), req, execution, facts, scheduler); err != nil || result.Session.State != workersessions.StateRunning {
		t.Fatalf("admission: %#v, %v", result, err)
	}
	if err := waitControlledSignal(started, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	peer := startSelectedEffectsInvocation(t, r, "deadline-peer", "success", 2043, "peer-runtime")
	facts.SetTick(100)
	if session := getCharacterizationSession(t, r, req.ID); session.State != workersessions.StateRunning {
		t.Fatalf("fact advance changed lifecycle: %#v", session)
	}
	scheduler.SetTick(5)
	if err := waitControlledSignal(observer.signals[workersessions.Topic(req.ID, req.Execution.Execution.FactorySessionID)], 30*time.Second); err != nil {
		t.Fatal(err)
	}
	session := getCharacterizationSession(t, r, req.ID)
	if session.State != workersessions.StateFailed || session.Result == nil || session.Result.Cause == nil || session.Result.Cause.Kind != workersessions.FailureCauseTimeout {
		t.Fatalf("selected deadline: %#v, want FAILED/TIMEOUT", session)
	}
	observation, err := r.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: req.ID})
	if err != nil || observation.EndedAt == nil || !observation.EndedAt.Equal(facts.Now()) {
		t.Fatalf("retained terminal facts: %#v, %v", observation, err)
	}
	if session := getCharacterizationSession(t, r, "deadline-peer"); session.State != workersessions.StateRunning {
		t.Fatalf("deadline changed peer: %#v", session)
	}
	close(peer.release)
	if result := awaitSelectedEffectsInvocation(t, peer); result.Session.State != workersessions.StateCompleted {
		t.Fatalf("peer completion: %#v", result)
	}
}

func TestKeyedRuntimeAsyncCloseDuringOpeningRejectsAdmissionAndJoinsCapture(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	recording := newInterruptRecordingService()
	r.recording = recording
	req := workersessions.StartRequest{RequestID: "opening-request", ID: "opening-worker", Execution: dispatchHandoff("opening-physical")}
	req.Execution.Execution.RuntimeID = "opening-runtime"
	req.Execution.Execution.RecordingID = "opening-recording"
	sink := &perRuntimeAppendCapture{EventsAppender: r.events}
	gate := &keyedOpeningGate{EventsAppender: sink, topic: workersessions.Topic(req.ID, req.Execution.Execution.FactorySessionID), entered: make(chan struct{}), release: make(chan struct{})}
	r.events = gate
	logger := &controlClaimLogger{claimed: make(chan struct{}), release: make(chan struct{}), message: "runtime Worker admission closed"}
	close(logger.release)
	r.logger = logger
	var release sync.Once
	unblock := func() { release.Do(func() { close(gate.release) }) }
	t.Cleanup(func() { unblock(); _ = r.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := make(chan asyncStartCompletion, 1)
	go func() {
		result, err := r.AdmitRuntimeAttemptAsync(ctx, req, unusedExecution{t: t}, r.clock, r.scheduler)
		started <- asyncStartCompletion{result: result, err: err}
	}()
	if err := waitControlledSignal(gate.entered, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	peer := startSelectedEffectsInvocation(t, r, "opening-peer", "success", 2043, "peer-runtime")
	closed := make(chan error, 1)
	go func() { closed <- r.CloseRuntimeAttempts(ctx, req.Execution.Execution.RuntimeID) }()
	if err := waitControlledSignal(logger.claimed, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-closed:
		t.Fatalf("close returned before opening/capture joined: %v", err)
	default:
	}
	unblock()
	select {
	case outcome := <-started:
		if !errors.Is(outcome.err, workersessions.ErrStartNotAccepted) || outcome.result.Session.State != workersessions.StateCanceled {
			t.Fatalf("closed opening admission: %#v, %v, result=%#v", outcome, outcome.err, outcome.result.Session.Result.Cause)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	assertAsyncClosedOpeningRetained(t, r, req, sink, recording)
	if session := getCharacterizationSession(t, r, "opening-peer"); session.State != workersessions.StateRunning {
		t.Fatalf("scope close changed peer: %#v", session)
	}
	close(peer.release)
	if result := awaitSelectedEffectsInvocation(t, peer); result.Session.State != workersessions.StateCompleted {
		t.Fatalf("peer completion: %#v", result)
	}
}

func assertAsyncClosedOpeningRetained(t *testing.T, r *registry, req workersessions.StartRequest, sink *perRuntimeAppendCapture, recording *interruptRecordingService) {
	t.Helper()
	appends := sink.requestsFor(workersessions.Topic(req.ID, req.Execution.Execution.FactorySessionID))
	if len(appends) != 2 || appends[0].SourceEventID != "started" || appends[1].SourceEventID != "terminal" {
		t.Fatalf("closed opening history: %#v", appends)
	}
	if closeCalls, terminalCalls := recording.handleFor(t, req.ID).counts(); closeCalls != 1 || terminalCalls != 1 {
		t.Fatalf("capture cleanup: close=%d terminal=%d", closeCalls, terminalCalls)
	}
	replay, err := r.AdmitRuntimeAttemptAsync(context.Background(), req, unusedExecution{t: t}, r.clock, r.scheduler)
	if !errors.Is(err, workersessions.ErrStartNotAccepted) || replay.Session.State != workersessions.StateCanceled || !reflect.DeepEqual(appends, sink.requestsFor(workersessions.Topic(req.ID, req.Execution.Execution.FactorySessionID))) {
		t.Fatalf("rejected admission replay: %#v, %v", replay, err)
	}
}
