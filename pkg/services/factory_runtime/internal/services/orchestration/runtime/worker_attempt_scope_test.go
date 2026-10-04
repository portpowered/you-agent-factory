package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (service *runtimeWorkerSessionsService) CloseRuntimeAttempts(context.Context, string) error {
	return nil
}

type scopedWorkerReadProbe struct {
	workersessions.Service
	scopes []string
	err    error
}

type scopedWorkerControlProbe struct {
	workersessions.Service
	requests []workersessions.ControlRequest
	gets     []workersessions.GetRequest
}

func (probe *scopedWorkerControlProbe) Get(_ context.Context, req workersessions.GetRequest) (workersessions.Session, error) {
	probe.gets = append(probe.gets, req)
	return workersessions.Session{ID: req.ID, State: workersessions.StateCompleted}, nil
}

func TestWorkerSessionRetrySelectionCarriesOwningFactoryScope(t *testing.T) {
	t.Parallel()
	probe := &scopedWorkerControlProbe{}
	if !terminalWorkerSessionRequiresRetry(context.Background(), probe, "recorded-worker", " replay-session ") {
		t.Fatal("selected terminal worker was not recognized")
	}
	if len(probe.gets) != 1 || probe.gets[0].ID != "recorded-worker" || probe.gets[0].FactorySessionID != "replay-session" {
		t.Fatalf("retry selection lost owning scope: %+v", probe.gets)
	}
}

func (probe *scopedWorkerControlProbe) Cancel(_ context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	probe.requests = append(probe.requests, req)
	return workersessions.ControlResult{Outcome: workersessions.ControlOutcomeNoop}, nil
}

func TestWorkerSessionControlFanoutCarriesCapturedFactoryScope(t *testing.T) {
	t.Parallel()
	probe := &scopedWorkerControlProbe{}
	captured := capturedWorkerSessionControlTargets{
		turnID: "turn-1", factorySessionID: "replay-session", workerSessionIDs: []string{"worker-original", "worker-peer"},
	}
	result := fanOutWorkerSessionControl(context.Background(), probe, captured, factory.WorkerSessionControlActionCancel, "control-1")
	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeNoOp || len(probe.requests) != 2 {
		t.Fatalf("fanout = %+v, requests=%+v", result, probe.requests)
	}
	for index, request := range probe.requests {
		if request.FactorySessionID != "replay-session" || request.ID != captured.workerSessionIDs[index] || request.RequestID != "control-1" {
			t.Fatalf("child request lost captured owner: %+v", request)
		}
	}
}

func TestWorkerSessionControlCapturesCanonicalOwnerBeforeFanout(t *testing.T) {
	t.Parallel()
	probe := &scopedWorkerControlProbe{}
	instance, ledger, err := newTestFactoryWithScriptedLedger(withNet(buildMoveControlNet()), withWorkerSessions(probe), withWorkflowContext(&factory_context.FactoryContext{SessionID: "execution-owner"}))
	if err != nil {
		t.Fatal(err)
	}
	runtime := instance.(*factoryImpl)
	runtime.cfg.publicSessionID = "public-durable-session"
	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 1, "association", "turn-1", "recorded-worker"))
	result := runtime.controlAssociatedWorkerSessions(context.Background(), "turn-1", "control-1", factory.WorkerSessionControlActionCancel, factory.ControlOutcomeAccepted)
	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeNoOp || len(probe.requests) != 1 || probe.requests[0].FactorySessionID != "execution-owner" {
		t.Fatalf("control lost canonical owner: result=%+v requests=%+v", result, probe.requests)
	}
}

func (probe *scopedWorkerReadProbe) GetObservationByWorkerSessionID(_ context.Context, req workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	probe.scopes = append(probe.scopes, req.FactorySessionID)
	return workersessions.Observation{}, probe.err
}

func (probe *scopedWorkerReadProbe) ReadTranscript(_ context.Context, req workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	probe.scopes = append(probe.scopes, req.FactorySessionID)
	return workersessions.ReadTranscriptResult{}, probe.err
}

func (probe *scopedWorkerReadProbe) StreamObservationsByWorkerSessionID(_ context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	probe.scopes = append(probe.scopes, req.FactorySessionID)
	return workersessions.ObservationSubscription{}, probe.err
}

func TestWorkerSessionReadFallbackCarriesOwningFactoryScope(t *testing.T) {
	t.Parallel()
	for _, requested := range []string{"", " recording-session ", "replay-session"} {
		t.Run(requested, func(t *testing.T) {
			t.Parallel()
			probe := &scopedWorkerReadProbe{err: errors.New("selected read effect")}
			reader := &recordedWorkerSessionObservation{Service: probe, factorySessionID: "recording-session"}
			ctx := context.Background()
			_, getErr := reader.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: requested,
			})
			_, transcriptErr := reader.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: requested,
			})
			_, streamErr := reader.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: requested,
			})
			wantErr, wantCalls := probe.err, 3
			if requested == "replay-session" {
				wantErr, wantCalls = workersessions.ErrObservationSessionNotFound, 0
			}
			for _, err := range []error{getErr, transcriptErr, streamErr} {
				if !errors.Is(err, wantErr) {
					t.Errorf("read error = %v, want %v", err, wantErr)
				}
			}
			if len(probe.scopes) != wantCalls {
				t.Fatalf("read calls = %d, want %d", len(probe.scopes), wantCalls)
			}
			for _, scope := range probe.scopes {
				if scope != "recording-session" {
					t.Errorf("selected read scope = %q, want recording-session", scope)
				}
			}
		})
	}
}

func TestInvokeWorkerRuntimeAttemptUsesSelectedEffectsAndResumeIdentity(t *testing.T) {
	t.Parallel()
	for _, outcome := range []workersessions.State{workersessions.StateCompleted, workersessions.StateFailed, workersessions.StateCanceled} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)
			scheduler := platformclock.NewDeterministic(time.Unix(0, 0), time.Second)
			execution := &testWorkstationBoundary{}
			probe := &runtimeInvocationProbe{Service: &fakeWorkerSessionsService{}, outcome: outcome}
			ledger := &recordingfixtures.ScriptedRuntimeLedger{}
			f := &factoryImpl{cfg: &runtimeConfig{workerSessions: struct{ workersessions.Service }{probe}, workerAttempts: probe, workerExecution: execution,
				clock: clock, workerAttemptScheduler: scheduler, runtimeID: "runtime-selected", publicSessionID: "factory-selected"}, eventHistory: ledger}
			result, err := f.InvokeWorker(context.Background(), factory.InvokeWorkerRequest{DispatchID: "child", Prompt: "run", MaxAttempts: 3, RecordingID: "recording-selected"})
			if err != nil {
				t.Fatal(err)
			}
			assertRuntimeInvocationIdentity(t, probe.request)
			if probe.execution != execution || probe.clock != clock || probe.scheduler != scheduler || probe.retry.MaxAttempts != 3 {
				t.Fatal("runtime invocation did not receive selected execution, facts, deadlines and retry budget")
			}
			want := map[workersessions.State]factory.InvokeWorkerOutcome{workersessions.StateCompleted: factory.InvokeWorkerOutcomeCompleted,
				workersessions.StateFailed: factory.InvokeWorkerOutcomeFailed, workersessions.StateCanceled: factory.InvokeWorkerOutcomeCanceled}[outcome]
			if result.DispatchID != "child" || result.WorkerSessionID != "child/resume/1" || result.Outcome != want {
				t.Fatalf("mapped result = %#v, want %s with retained identities", result, want)
			}
			associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
			if len(associations) != 1 || associations[0].DispatchID != "child" || associations[0].WorkerSessionID != "child/resume/1" {
				t.Fatalf("associations = %#v", associations)
			}
		})
	}
}

func assertRuntimeInvocationIdentity(t *testing.T, request workersessions.RuntimeAttemptRequest) {
	t.Helper()
	if request.Key != (workersessions.RuntimeAttemptKey{RuntimeID: "runtime-selected", DispatchID: "child"}) ||
		request.ID != "child/resume/1" || request.AttemptID != "child/resume/1" ||
		request.Execution.Execution.Dispatch.DispatchID != "child" {
		t.Fatalf("runtime admission lost logical/physical identity: %#v", request)
	}
	if request.Execution.Execution.FactorySessionID != "factory-selected" || request.Execution.Execution.RecordingID != "recording-selected" {
		t.Fatalf("runtime invocation lost session/recording: %#v", request.Execution.Execution)
	}
}

func TestInvokeWorkerRuntimeAttemptMissingCapabilityDoesNotReserve(t *testing.T) {
	t.Parallel()
	probe := &runtimeInvocationProbe{Service: &fakeWorkerSessionsService{}}
	// Embedding only the public service intentionally omits the runtime capability.
	sessions := struct{ workersessions.Service }{probe}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	f := &factoryImpl{cfg: &runtimeConfig{workerSessions: sessions}, eventHistory: ledger}
	if _, err := f.InvokeWorker(context.Background(), factory.InvokeWorkerRequest{DispatchID: "child", Prompt: "run"}); !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("missing runtime capability = %v", err)
	}
	if probe.reservations != 0 || len(ledger.DispatchWorkerSessionAssociationsSnapshot()) != 0 {
		t.Fatal("missing capability reserved identity or published an association")
	}
}

type runtimeInvocationProbe struct {
	factory.WorkerAttemptOpener
	workersessions.Service
	request      workersessions.RuntimeAttemptRequest
	retry        workersessions.RetryPolicy
	execution    workers.Service
	clock        platformclock.Source
	scheduler    platformclock.TimerSource
	outcome      workersessions.State
	reservations int
}

func (p *runtimeInvocationProbe) Reserve(_ context.Context, req workersessions.ReserveRequest) (workersessions.Session, error) {
	p.reservations++
	if req.ID == "child" {
		return workersessions.Session{}, workersessions.ErrSessionAlreadyExists
	}
	return workersessions.Session{ID: req.ID, State: workersessions.StateReserved}, nil
}

func (p *runtimeInvocationProbe) InvokeRuntimeSession(_ context.Context, req workersessions.RuntimeAttemptRequest, retry workersessions.RetryPolicy, execution workers.Service, clock platformclock.Source, scheduler platformclock.TimerSource) (workersessions.InvokeSessionResult, error) {
	p.request, p.retry, p.execution, p.clock, p.scheduler = req, retry, execution, clock, scheduler
	return workersessions.InvokeSessionResult{Session: workersessions.Session{ID: req.ID, State: p.outcome}}, nil
}

// This boundary supplies a canceled admission result; Worker Sessions' own
// component tests prove the control/command race. Here the observer is Runtime's
// reservation/association/invocation order and outward result mapping.
type associationWindowSessions struct {
	factory.WorkerAttemptOpener
	workersessions.Service
	reserved chan workersessions.ReserveRequest
	invoked  chan workersessions.RuntimeAttemptRequest
}

func (s *associationWindowSessions) Reserve(_ context.Context, req workersessions.ReserveRequest) (workersessions.Session, error) {
	s.reserved <- req
	return workersessions.Session{ID: req.ID, State: workersessions.StateReserved}, nil
}

func (s *associationWindowSessions) InvokeRuntimeSession(_ context.Context, req workersessions.RuntimeAttemptRequest, _ workersessions.RetryPolicy, _ workers.Service, _ platformclock.Source, _ platformclock.TimerSource) (workersessions.InvokeSessionResult, error) {
	s.invoked <- req
	return workersessions.InvokeSessionResult{Session: workersessions.Session{ID: req.ID, State: workersessions.StateCanceled}}, nil
}

func assertInvokeWorkerReservationWindow(t *testing.T, sessions *associationWindowSessions, ledger *blockingAssociationLedger) {
	t.Helper()
	select {
	case reserved := <-sessions.reserved:
		associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
		if reserved.ID != "dispatch-1" || len(associations) != 1 || associations[0].DispatchID != "dispatch-1" || associations[0].WorkerSessionID != reserved.ID {
			t.Fatalf("association did not retain the already-reserved identity: %#v, %#v", reserved, associations)
		}
	default:
		t.Fatal("association published before identity reservation")
	}
	select {
	case invoked := <-sessions.invoked:
		t.Fatalf("invocation began before association publication returned: %#v", invoked)
	default:
	}
}

func TestWorkerSessionFleetSourceUsesCanonicalExecutionOwner(t *testing.T) {
	t.Parallel()
	probe := &scopedWorkerListProbe{}
	runtime := &factoryImpl{cfg: &runtimeConfig{workerSessions: probe, publicSessionID: "public-durable-session", runtimeID: "execution-runtime", workflowContext: &factory_context.FactoryContext{SessionID: "execution-owner"}}}
	view := runtime.WorkerSessionsObservationForSession("public-durable-session")
	_, err := view.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{Scope: workersessions.ObservationScopeFactory, MaxResults: 2, NextToken: "cursor"})
	if err != nil || probe.request.RuntimeID != "execution-runtime" || probe.request.MaxResults != 2 || probe.request.NextToken != "cursor" || probe.request.Scope != workersessions.ObservationScopeFactory {
		t.Fatalf("scoped fleet request = %#v, %v", probe.request, err)
	}
}

type scopedWorkerListProbe struct {
	workersessions.Service
	request workersessions.ListWorkerSessionObservationsRequest
}

func (probe *scopedWorkerListProbe) ListWorkerSessionObservations(_ context.Context, req workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	probe.request = req
	return workersessions.ListWorkerSessionObservationsResult{}, nil
}
