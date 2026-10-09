package service

import (
	"context"
	"encoding/json"
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
			before := assertPerRuntimeAttemptState(t, t.Context(), owner, state)
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
	if after := assertPerRuntimeAttemptState(t, t.Context(), owner, before.State); !reflect.DeepEqual(before, after) {
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
			clock := r.clock
			scheduler := r.scheduler
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
	if session := getCharacterizationSession(t, t.Context(), r, req.ID); session.State != workersessions.StateRunning {
		t.Fatalf("fact advance changed lifecycle: %#v", session)
	}
	scheduler.SetTick(5)
	if err := waitControlledSignal(observer.signals[workersessions.Topic(req.ID, req.Execution.Execution.FactorySessionID)], 30*time.Second); err != nil {
		t.Fatal(err)
	}
	session := getCharacterizationSession(t, t.Context(), r, req.ID)
	if session.State != workersessions.StateFailed || session.Result == nil || session.Result.Cause == nil || session.Result.Cause.Kind != workersessions.FailureCauseTimeout {
		t.Fatalf("selected deadline: %#v, want FAILED/TIMEOUT", session)
	}
	observation, err := r.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: req.ID})
	if err != nil || observation.EndedAt == nil || !observation.EndedAt.Equal(facts.Now()) {
		t.Fatalf("retained terminal facts: %#v, %v", observation, err)
	}
	if session := getCharacterizationSession(t, t.Context(), r, "deadline-peer"); session.State != workersessions.StateRunning {
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
	if session := getCharacterizationSession(t, t.Context(), r, "opening-peer"); session.State != workersessions.StateRunning {
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

// Replay preserves its recorded Worker ID while each Factory Session retains
// its own observation, provider association, controls and source history.
func TestKeyedRuntimeEqualWorkerIdentityAcrossFactorySessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	eventStore := newEventsAppender()
	sink := &perRuntimeAppendCapture{EventsAppender: eventStore}
	owner := preparePerRuntimeAttemptFixture(t, "recording-owner", sink)
	owner.service.retainedReader = eventStore
	replay := preparePerRuntimeAttemptFixture(t, "replay-owner", sink, owner)
	replay.request.ID = owner.request.ID
	for _, fixture := range []*perRuntimeAttemptFixture{owner, replay} {
		fixture.request.Execution.Execution.RecordingID = ""
		fixture.request.ObservationRuntimeID = "fleet-" + fixture.request.Execution.Execution.FactorySessionID
		var err error
		fixture.attempt, err = fixture.service.BeginRuntimeAttempt(ctx, fixture.request, fixture.service.execution,
			coverageClock{now: fixture.clock}, fixture.service.scheduler, fixture.control.cancel)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = fixture.attempt.Complete(ctx, runtimeAttemptCompletedDispatch(perRuntimeLogicalDispatchID), nil)
		})
		fragment := keyedRuntimeProgressFragment(fixture)
		fragment.Continuation.ProviderSessionID = "provider-" + fixture.request.Execution.Execution.FactorySessionID
		if err := fixture.service.PublishRuntimeProgress(ctx, fixture.request.Key, fragment, nil); err != nil {
			t.Fatal(err)
		}
		assertScopedWorkerIdentity(t, fixture, workersessions.StateRunning)
	}
	if _, err := owner.service.Reserve(ctx, workersessions.ReserveRequest{ID: owner.request.ID}); !errors.Is(err, workersessions.ErrSessionAlreadyExists) {
		t.Fatalf("unscoped duplicate reserve = %v", err)
	}
	if _, err := owner.service.Get(ctx, workersessions.GetRequest{ID: owner.request.ID}); !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) {
		t.Fatalf("ambiguous unscoped Get = %v", err)
	}
	assertWorkerAddressCandidates(t, owner, replay)
	assertAmbiguousWorkerControlsHaveNoEffects(t, owner, replay, sink)
	cancelScopedWorker(t, replay)
	assertScopedWorkerIdentity(t, owner, workersessions.StateRunning)
	if err := owner.attempt.Complete(ctx, runtimeAttemptCompletedDispatch(perRuntimeLogicalDispatchID), nil); err != nil {
		t.Fatal(err)
	}
	assertScopedWorkerIdentity(t, owner, workersessions.StateCompleted)
	assertScopedWorkerIdentity(t, replay, workersessions.StateCanceled)
}

func assertAmbiguousWorkerControlsHaveNoEffects(t *testing.T, owner, peer *perRuntimeAttemptFixture, sink *perRuntimeAppendCapture) {
	t.Helper()
	before := sink.requestsFor("")
	ctx := context.Background()
	continued, err := owner.service.Continue(ctx, workersessions.ContinueRequest{
		RequestID: "ambiguous-continue", SourceWorkerSessionID: owner.request.ID,
		SuccessorWorkerSessionID: "ambiguous-successor", FollowUpInput: "follow up",
	})
	if !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) || continued.Session.ID != "" {
		t.Fatalf("ambiguous Continue = %+v, %v", continued, err)
	}
	for _, mode := range []string{"provider", "recorded"} {
		interrupted, err := owner.service.Interrupt(ctx, workersessions.InterruptRequest{
			RequestID: "ambiguous-interrupt-" + mode, SourceWorkerSessionID: owner.request.ID,
			SuccessorWorkerSessionID: "ambiguous-successor", ReplacementMessage: "replace", ResumeMode: mode,
		})
		if !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) || !errors.Is(err, workersessions.ErrInterruptValidation) ||
			interrupted.Phase != workersessions.InterruptPhaseValidation || interrupted.Accepted || interrupted.Source.ID != "" || interrupted.Successor.ID != "" {
			t.Fatalf("ambiguous Interrupt(%s) = %+v, %v", mode, interrupted, err)
		}
	}
	for _, fixture := range []*perRuntimeAttemptFixture{owner, peer} {
		select {
		case <-fixture.control.invoked:
			t.Fatal("ambiguous control invoked a cancellation")
		default:
		}
		assertScopedWorkerIdentity(t, fixture, workersessions.StateRunning)
	}
	if !reflect.DeepEqual(before, sink.requestsFor("")) {
		t.Fatal("ambiguous control published history or an opening")
	}
	if _, err := owner.service.Get(ctx, workersessions.GetRequest{ID: "ambiguous-successor"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("ambiguous control admitted successor: %v", err)
	}
}

func assertWorkerAddressCandidates(t *testing.T, owner, peer *perRuntimeAttemptFixture) {
	t.Helper()
	observation, err := owner.service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: owner.request.ID})
	var ambiguous *workersessions.AmbiguousAddressError
	if !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) || !errors.As(err, &ambiguous) || observation.WorkerSessionID != "" {
		t.Fatalf("ambiguous unscoped observation = %+v, %v", observation, err)
	}
	var want []workersessions.AddressCandidate
	for _, fixture := range []*perRuntimeAttemptFixture{owner, peer} {
		scoped, scopedErr := fixture.service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{
			WorkerSessionID: fixture.request.ID, FactorySessionID: fixture.request.Execution.Execution.FactorySessionID,
		})
		if scopedErr != nil {
			t.Fatal(scopedErr)
		}
		want = append(want, scoped.AddressCandidate())
	}
	if !reflect.DeepEqual(ambiguous.Candidates, workersessions.NewAmbiguousAddressError(want).Candidates) {
		t.Fatalf("candidates = %+v, want %+v", ambiguous.Candidates, want)
	}
	if ambiguous.Candidates[0].WorkID != nil {
		*ambiguous.Candidates[0].WorkID = "changed-by-caller"
	}
	ambiguous.Candidates[0].FactorySessionID = "changed-by-caller"
	_, againErr := owner.service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: owner.request.ID})
	var again *workersessions.AmbiguousAddressError
	if !errors.As(againErr, &again) || !reflect.DeepEqual(again.Candidates, workersessions.NewAmbiguousAddressError(want).Candidates) {
		t.Fatalf("caller mutated retained candidates: %+v, %v", again, againErr)
	}
}

func assertScopedWorkerIdentity(t *testing.T, fixture *perRuntimeAttemptFixture, state workersessions.State) {
	t.Helper()
	ctx := context.Background()
	scope := fixture.request.Execution.Execution.FactorySessionID
	assertScopedWorkerList(t, fixture, scope)
	session, err := fixture.service.Get(ctx, workersessions.GetRequest{ID: fixture.request.ID, FactorySessionID: scope})
	if err != nil || session.ID != fixture.request.ID || session.State != state || session.ProviderSessionAssociation == nil || session.ProviderSessionAssociation.Reference.ID != "provider-"+scope {
		t.Fatalf("scoped session = %#v, %v", session, err)
	}
	observation, err := fixture.service.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.request.ID, FactorySessionID: scope})
	if err != nil || observation.WorkerSessionID != fixture.request.ID || observation.FactorySessionID != scope || observation.State != state || observation.AttemptID != fixture.request.AttemptID {
		t.Fatalf("scoped observation = %#v, %v", observation, err)
	}
	subscription, err := fixture.service.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: fixture.request.ID, FactorySessionID: scope, ReplayOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	opening := subscription.Next(ctx)
	if opening.Kind != workersessions.ObservationDeliveryRecord || opening.Event.Cursor.WorkerSessionID != fixture.request.ID {
		t.Fatalf("scoped opening = %#v", opening)
	}
}

func cancelScopedWorker(t *testing.T, fixture *perRuntimeAttemptFixture) {
	t.Helper()
	controlled := make(chan error, 1)
	go func() {
		result, err := fixture.service.Cancel(context.Background(), workersessions.ControlRequest{ID: fixture.request.ID, FactorySessionID: fixture.request.Execution.Execution.FactorySessionID})
		if err == nil && (result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateCanceled || result.Session.ID != fixture.request.ID) {
			err = fmt.Errorf("scoped Cancel = %#v", result)
		}
		controlled <- err
	}()
	if err := waitControlledSignal(fixture.control.invoked, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := fixture.attempt.Complete(context.Background(), runtimeAttemptCanceledDispatch(perRuntimeLogicalDispatchID), nil); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-controlled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("scoped cancellation did not join")
	}
}

func assertScopedWorkerList(t *testing.T, fixture *perRuntimeAttemptFixture, scope string) {
	t.Helper()
	ctx := context.Background()
	listed, listErr := fixture.service.ListWorkerSessionObservations(ctx, workersessions.ListWorkerSessionObservationsRequest{FactorySessionID: scope, RuntimeID: fixture.request.ObservationRuntimeID})
	if (listErr != nil && !errors.Is(listErr, workersessions.ErrObservationProjectionUnavailable)) || len(listed.Observations) != 1 || listed.Observations[0].WorkerSessionID != fixture.request.ID || listed.Observations[0].FactorySessionID != scope {
		t.Fatalf("scoped list = %#v, %v", listed, listErr)
	}
}

func TestKeyedRuntimeBufferedOutputUsesSourceOwnerAndDoesNotDuplicateStream(t *testing.T) {
	t.Parallel()
	for _, streamed := range []bool{false, true} {
		t.Run(fmt.Sprint(streamed), func(t *testing.T) {
			t.Parallel()
			sink := &perRuntimeAppendCapture{EventsAppender: newEventsAppender()}
			fixture := preparePerRuntimeAttemptFixture(t, "buffered", sink)
			fixture.request.ObservationFactorySessionID = "source-owner"
			attempt, err := fixture.service.BeginRuntimeAttempt(context.Background(), fixture.request, fixture.service.execution, coverageClock{now: fixture.clock}, fixture.service.scheduler, fixture.control.cancel)
			if err != nil {
				t.Fatal(err)
			}
			if streamed {
				request := perRuntimeProgressRequest(fixture)
				request.Draft.Kind = workers.KindMessage
				request.Draft.Phase = workers.PhaseCompleted
				request.Draft.Payload = json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"streamed"}]}`)
				if _, err := fixture.service.PublishRecord(context.Background(), request); err != nil {
					t.Fatal(err)
				}
			}
			result := runtimeAttemptCompletedDispatch(perRuntimeLogicalDispatchID)
			result.Result.Output = "buffered"
			if err := attempt.Complete(context.Background(), result, nil); err != nil {
				t.Fatal(err)
			}
			if err := attempt.Complete(context.Background(), result, nil); err != nil {
				t.Fatal(err)
			}
			records := sink.requestsFor(workersessions.Topic(fixture.request.ID, "source-owner"))
			want := "buffered"
			if streamed {
				want = "streamed"
			}
			assertOneBufferedWorkerMessage(t, records, want)
			if len(sink.requestsFor(workersessions.Topic(fixture.request.ID, fixture.request.Execution.Execution.FactorySessionID))) != 0 {
				t.Fatal("routed correlation leaked to a second source topic")
			}
		})
	}
}

func assertOneBufferedWorkerMessage(t *testing.T, records []events.AppendRequest, want string) {
	t.Helper()
	messages := 0
	for index, record := range records {
		draft := decodePerRuntimeDraft(t, record)
		if draft.Kind == workers.KindMessage {
			messages++
			if index == 0 || index == len(records)-1 {
				t.Fatalf("output is not bracketed by lifecycle records: %#v", records)
			}
			var payload workers.MessagePayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != want {
				t.Fatalf("message payload=%#v", payload)
			}
		}
	}
	if messages != 1 || decodePerRuntimeDraft(t, records[len(records)-1]).Phase != workers.PhaseCompleted {
		t.Fatalf("retained output/terminal=%#v", records)
	}
}
