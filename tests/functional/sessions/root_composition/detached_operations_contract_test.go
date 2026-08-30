package root_composition_test

import (
	"context"
	"errors"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestDetachedOperationsFunctionalContract drives the public detached value
// operations through both execution modes. The owner fake is deliberately
// stateful: assertions verify the translated requests and returned projections
// rather than merely checking that construction succeeds.
func TestDetachedOperationsFunctionalContract(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)

	owner := newFunctionalDetachedOwner()
	operations, err := (&factorysessions.DetachedOperations{}).Bind(owner)
	if err != nil {
		t.Fatalf("bind detached operations: %v", err)
	}
	ctx := context.Background()
	testDetachedStartsAndInvocation(t, ctx, operations, owner)
	testDetachedReadsAndControls(t, ctx, operations, owner)
	testDetachedResultsAndPreparation(t, ctx, operations, owner)
}

func testDetachedStartsAndInvocation(t *testing.T, ctx context.Context, operations factorysessions.DetachedService, owner *functionalDetachedOwner) {
	t.Helper()

	liveStart, err := operations.Start(ctx, factorysessions.SessionStartRequest{
		Mode:       factorysessions.SessionOperationModeLive,
		FolderPath: "  /functional/workspace  ",
		Target:     &factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "live"},
	})
	if err != nil || liveStart.SessionID != "live-functional" || liveStart.Live == nil || liveStart.Live.Session == nil || liveStart.Live.Session.Status != "RUNNING" {
		t.Fatalf("live start = %#v, error = %v", liveStart, err)
	}
	if owner.openRequest.FolderPath != "/functional/workspace" || owner.openRequest.Target == nil || owner.openRequest.Target.Name != "live" {
		t.Fatalf("live start request = %#v, want normalized folder and target", owner.openRequest)
	}

	durableStart, err := operations.Start(ctx, factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "functional-async"},
		Definition:  factorysessions.SessionDefinitionSelection{FactoryID: "factory-a"},
		Input: &work.PreparedInvocationInput{
			NormalizedArguments: &work.NormalizedArguments{
				Arguments: map[string]work.NormalizedArgument{"goal": {Values: []string{"ship"}}},
			},
		},
		Policy:         map[string]any{"retry": map[string]any{"max": 2}},
		RuntimeOptions: &factorysessions.RuntimeOptions{ChildExecutorMode: "functional"},
	})
	if err != nil || durableStart.SessionID != "durable-functional" || durableStart.Async == nil {
		t.Fatalf("durable async start = %#v, error = %v", durableStart, err)
	}
	if owner.asyncStartRequest.RequestID != "functional-async" || owner.asyncStartRequest.Source.FactoryID != "factory-a" || owner.asyncStartRequest.Args["goal"] != "ship" {
		t.Fatalf("durable async request = %#v", owner.asyncStartRequest)
	}

	syncStart, err := operations.Start(ctx, factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "functional-sync"},
		Synchronous: true,
		Wait:        factorysessions.SessionOperationWait{TimeoutMillis: 120, CancelOnTimeout: true},
	})
	if err != nil || syncStart.SessionID != "durable-sync-functional" || syncStart.Sync == nil || syncStart.Status != "COMPLETED" {
		t.Fatalf("durable sync start = %#v, error = %v", syncStart, err)
	}
	if owner.syncStartRequest.Wait == nil || owner.syncStartRequest.Wait.TimeoutMillis == nil || *owner.syncStartRequest.Wait.TimeoutMillis != 120 || !owner.syncStartRequest.Wait.CancelOnTimeout {
		t.Fatalf("durable sync wait = %#v", owner.syncStartRequest.Wait)
	}

	invocation, err := operations.Invoke(ctx, factorysessions.SessionInvokeRequest{
		SessionID:   "live-functional",
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "functional-invoke"},
		Input:       &work.PreparedInvocationInput{Source: work.InputSourcePositionalText},
		Wait:        factorysessions.SessionOperationWait{TimeoutMillis: 500},
	})
	if err != nil || invocation.Status != factorysessions.InvocationTerminalStatusCompleted || owner.invocationSessionID != "live-functional" {
		t.Fatalf("invoke = %#v, error = %v", invocation, err)
	}
	if owner.invocationRequest.RequestID == nil || *owner.invocationRequest.RequestID != "functional-invoke" || owner.invocationRequest.TimeoutMillis == nil || *owner.invocationRequest.TimeoutMillis != 500 {
		t.Fatalf("translated invocation = %#v", owner.invocationRequest)
	}

	activated, err := operations.Activate(ctx, factorysessions.SessionActivateRequest{
		SessionID:  "live-functional",
		Definition: factorysessions.SessionDefinitionSelection{FactoryID: "fallback-factory"},
	})
	if err != nil || !activated.Activated || activated.FactoryName != "fallback-factory" || owner.activatedFactory != "fallback-factory" {
		t.Fatalf("activate = %#v, owner = %q, error = %v", activated, owner.activatedFactory, err)
	}
}

func testDetachedReadsAndControls(t *testing.T, ctx context.Context, operations factorysessions.DetachedService, owner *functionalDetachedOwner) {
	t.Helper()

	live, err := operations.Get(ctx, factorysessions.SessionGetRequest{SessionID: "live-functional", Mode: factorysessions.SessionOperationModeLive})
	if err != nil || live.Session.SessionID != "live-functional" || live.Session.Status != "RUNNING" {
		t.Fatalf("live get = %#v, error = %v", live, err)
	}
	durable, err := operations.Get(ctx, factorysessions.SessionGetRequest{SessionID: "durable-functional", Mode: factorysessions.SessionOperationModeDurable})
	if err != nil || durable.Session.SessionID != "durable-functional" || durable.Session.SourceRef != "factory.js" {
		t.Fatalf("durable get = %#v, error = %v", durable, err)
	}
	listed, err := operations.List(ctx, factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationModeAll, Filters: factorysessions.SessionListFilters{Statuses: []factorysessions.LifecycleStatus{factorysessions.LifecycleStatusSucceeded}}})
	if err != nil || len(listed.Sessions) != 2 || listed.Sessions[0].Mode != factorysessions.SessionOperationModeLive || listed.Sessions[1].Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("list all = %#v, error = %v", listed, err)
	}
	liveListed, err := operations.List(ctx, factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationModeLive})
	if err != nil || len(liveListed.Sessions) != 1 || liveListed.Sessions[0].Mode != factorysessions.SessionOperationModeLive {
		t.Fatalf("list live = %#v, error = %v", liveListed, err)
	}
	durableListed, err := operations.List(ctx, factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationModeDurable})
	if err != nil || len(durableListed.Sessions) != 1 || durableListed.Sessions[0].Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("list durable = %#v, error = %v", durableListed, err)
	}

	for _, test := range []struct {
		name      string
		mode      factorysessions.SessionOperationMode
		operation factorysessions.SessionControlOperation
	}{
		{name: "live pause", mode: factorysessions.SessionOperationModeLive, operation: factorysessions.SessionControlPause},
		{name: "live resume", mode: factorysessions.SessionOperationModeLive, operation: factorysessions.SessionControlResume},
		{name: "live close", mode: factorysessions.SessionOperationModeLive, operation: factorysessions.SessionControlClose},
		{name: "durable pause", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlPause},
		{name: "durable resume", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlResume},
		{name: "durable cancel", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlCancel},
		{name: "durable terminate", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlTerminate},
		{name: "durable approve", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlApprove},
		{name: "durable retry", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlRetryDispatch},
		{name: "durable interrupt", mode: factorysessions.SessionOperationModeDurable, operation: factorysessions.SessionControlInterruptDispatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			sessionID := "live-functional"
			if test.mode == factorysessions.SessionOperationModeDurable {
				sessionID = "durable-functional"
			}
			control, err := operations.Control(ctx, factorysessions.SessionControlRequest{
				SessionID:   sessionID,
				Mode:        test.mode,
				Operation:   test.operation,
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: "functional-control", TurnID: "functional-turn"},
				Approve:     &factorysessions.ApproveRequest{},
				Retry:       &factorysessions.RetryDispatchRequest{},
				Interrupt:   &factorysessions.InterruptDispatchRequest{},
			})
			if err != nil || control.SessionID != sessionID || control.Operation != test.operation {
				t.Fatalf("control = %#v, error = %v", control, err)
			}
		})
	}

	recovered, err := operations.Control(ctx, factorysessions.SessionControlRequest{
		SessionID:   "durable-functional",
		Mode:        factorysessions.SessionOperationModeDurable,
		Operation:   factorysessions.SessionControlRecover,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "functional-recover"},
	})
	if err != nil || recovered.Recovery == nil || recovered.Recovery.SessionID != "durable-recovered-functional" || owner.resumeRequest.RequestID != "functional-recover" {
		t.Fatalf("recover = %#v, request = %#v, error = %v", recovered, owner.resumeRequest, err)
	}
}

func testDetachedResultsAndPreparation(t *testing.T, ctx context.Context, operations factorysessions.DetachedService, owner *functionalDetachedOwner) {
	t.Helper()

	durableResult, err := operations.ReadResult(ctx, factorysessions.SessionResultReadRequest{SessionID: "durable-functional", Mode: factorysessions.SessionOperationModeDurable, Request: factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal}})
	if err != nil || durableResult.Durable == nil || durableResult.Status != string(factorysessions.ResultStatusNotReady) {
		t.Fatalf("durable result = %#v, error = %v", durableResult, err)
	}
	subscription, err := operations.Subscribe(ctx, factorysessions.SessionResponseSubscriptionRequest{
		SessionID:     "live-functional",
		AfterSequence: 4,
		DispatchID:    "dispatch-functional",
		Kinds:         []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil || subscription.Cursor != owner.subscriptionCursor || owner.subscriptionRequest.AfterSequence != 4 || len(owner.subscriptionRequest.Kinds) != 1 {
		t.Fatalf("subscribe = %#v, request = %#v, error = %v", subscription, owner.subscriptionRequest, err)
	}

	nested := map[string]any{"value": "original"}
	prepared, err := operations.PrepareSync(ctx, factorysessions.SessionSyncPreparationRequest{
		Start: factorysessions.SessionStartRequest{
			Mode:        factorysessions.SessionOperationModeDurable,
			Correlation: factorysessions.SessionOperationCorrelation{RequestID: "functional-prepare"},
			Args:        map[string]any{"nested": nested},
		},
		Wait: factorysessions.SessionOperationWait{TimeoutMillis: 75, CancelOnTimeout: true},
	})
	if err != nil || !prepared.Request.Synchronous || prepared.Wait.TimeoutMillis != 75 || !prepared.Wait.CancelOnTimeout {
		t.Fatalf("prepare sync = %#v, error = %v", prepared, err)
	}
	preparedArgs := prepared.Request.Args["nested"].(map[string]any)
	nested["value"] = "changed"
	if preparedArgs["value"] != "original" {
		t.Fatalf("prepared request shares caller input: %#v", preparedArgs)
	}
}

func TestDetachedOperationsFunctionalValidation(t *testing.T) {
	t.Parallel()

	if operations, err := (&factorysessions.DetachedOperations{}).Bind(nil); operations != nil || !errors.Is(err, factorysessions.ErrDetachedServiceUnavailable) {
		t.Fatalf("bind nil = (%#v, %v), want unavailable", operations, err)
	}
	var nilOperations *factorysessions.DetachedOperations
	if _, err := nilOperations.Start(context.Background(), factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeLive}); !errors.Is(err, factorysessions.ErrDetachedServiceUnavailable) {
		t.Fatalf("nil operation start error = %v, want unavailable", err)
	}

	var nilInvocationError *factorysessions.InvocationValidationError
	if nilInvocationError.Error() != "invocation validation error" || (&factorysessions.InvocationValidationError{Message: "message"}).Error() != "message" || (&factorysessions.InvocationValidationError{Field: "field"}).Error() == "" || (&factorysessions.InvocationValidationError{Field: "field", Message: "message"}).Error() != "field: message" {
		t.Fatal("InvocationValidationError error messages are unstable")
	}
	var nilRequestError *factorysessions.DetachedRequestError
	if nilRequestError.Error() != "factory session detached request is invalid" || (&factorysessions.DetachedRequestError{Field: "field"}).Error() == "" || (&factorysessions.DetachedRequestError{Field: "field", Message: "message"}).Error() != "field: message" {
		t.Fatal("DetachedRequestError error messages are unstable")
	}
	if (factorysessions.SessionOperationWait{TimeoutMillis: 75}).SessionOperationTimeout() <= 0 || (factorysessions.SessionOperationWait{}).SessionOperationTimeout() != 0 {
		t.Fatal("session operation timeout conversion is unstable")
	}

	owner := newFunctionalDetachedOwner()
	operations, err := (&factorysessions.DetachedOperations{}).Bind(owner)
	if err != nil {
		t.Fatalf("bind functional owner: %v", err)
	}
	invalidStarts := []factorysessions.SessionStartRequest{
		{Mode: factorysessions.SessionOperationMode("unknown")},
		{Mode: factorysessions.SessionOperationModeDurable},
		{Mode: factorysessions.SessionOperationModeLive, Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}},
	}
	for _, request := range invalidStarts {
		if _, err := operations.Start(context.Background(), request); err == nil {
			t.Fatalf("invalid start %#v unexpectedly succeeded", request)
		}
	}
	if _, err := operations.Invoke(context.Background(), factorysessions.SessionInvokeRequest{}); err == nil {
		t.Fatal("empty invoke unexpectedly succeeded")
	}
	if _, err := operations.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "live", Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}}); err == nil {
		t.Fatal("negative invoke timeout unexpectedly succeeded")
	}
	if _, err := operations.Activate(context.Background(), factorysessions.SessionActivateRequest{SessionID: "live"}); err == nil {
		t.Fatal("empty activate name unexpectedly succeeded")
	}
	if _, err := operations.Get(context.Background(), factorysessions.SessionGetRequest{SessionID: "live", Mode: factorysessions.SessionOperationMode("unknown")}); err == nil {
		t.Fatal("invalid get mode unexpectedly succeeded")
	}
	if _, err := operations.List(context.Background(), factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationMode("unknown")}); err == nil {
		t.Fatal("invalid list mode unexpectedly succeeded")
	}
	if _, err := operations.Control(context.Background(), factorysessions.SessionControlRequest{SessionID: "live", Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlOperation("unknown")}); err == nil {
		t.Fatal("invalid live control unexpectedly succeeded")
	}
	if _, err := operations.Control(context.Background(), factorysessions.SessionControlRequest{SessionID: "live", Mode: factorysessions.SessionOperationModeDurable, Operation: factorysessions.SessionControlOperation("unknown")}); err == nil {
		t.Fatal("invalid durable control unexpectedly succeeded")
	}
	if _, err := operations.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{SessionID: "live", Mode: factorysessions.SessionOperationMode("unknown")}); err == nil {
		t.Fatal("invalid result mode unexpectedly succeeded")
	}
	if _, err := operations.PrepareSync(context.Background(), factorysessions.SessionSyncPreparationRequest{Start: factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeLive}}); err == nil {
		t.Fatal("live prepare sync unexpectedly succeeded")
	}
	if _, err := operations.PrepareSync(context.Background(), factorysessions.SessionSyncPreparationRequest{Start: factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeDurable}}); err == nil {
		t.Fatal("prepare sync without request id unexpectedly succeeded")
	}
	if _, err := operations.PrepareSync(context.Background(), factorysessions.SessionSyncPreparationRequest{Start: factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeDurable, Correlation: factorysessions.SessionOperationCorrelation{RequestID: "id"}, Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}}}); err == nil {
		t.Fatal("prepare sync with negative start timeout unexpectedly succeeded")
	}
	if _, err := operations.Subscribe(context.Background(), factorysessions.SessionResponseSubscriptionRequest{SessionID: "live", AfterSequence: -1}); err == nil {
		t.Fatal("negative subscription sequence unexpectedly succeeded")
	}
}

type functionalDetachedOwner struct {
	factorysessions.Service
	openRequest         factorysessions.OpenRequest
	openResult          *factorysessions.OpenResult
	asyncStartRequest   factorysessions.StartRequest
	asyncStartResult    factorysessions.AsyncStartResult
	syncStartRequest    factorysessions.StartRequest
	syncStartResult     factorysessions.SyncStartResult
	resumeRequest       factorysessions.ResumeSessionRequest
	resumeResult        factorysessions.AsyncStartResult
	invocationSessionID string
	invocationRequest   factorysessions.InvocationRequest
	invocationResult    factorysessions.InvocationResult
	activatedFactory    string
	liveProjection      factorysessions.SessionProjection
	durableProjection   factorysessions.SessionReadResult
	liveProjections     []factorysessions.ReadProjection
	durableProjections  []factorysessions.DurableSessionListSummary
	controlResult       factorysessions.LifecycleControlResult
	closeSessionID      string
	resultRead          factorysessions.ResultReadResult
	liveResultID        string
	partialResultID     string
	liveResult          factorysessions.LiveSessionResult
	partialResult       factorysessions.PartialSessionResult
	subscriptionRequest factorysessions.ResponseEventSubscriptionRequest
	subscriptionCursor  *factorysessions.ResponseEventCursor
}

func newFunctionalDetachedOwner() *functionalDetachedOwner {
	return &functionalDetachedOwner{
		openResult: &factorysessions.OpenResult{
			SessionID: "live-functional",
			Session: &factorysessions.ScopedLiveSessionSummary{
				ID:      "live-functional",
				Runtime: &factorysessions.RuntimeProjection{Status: "RUNNING"},
			},
		},
		asyncStartResult:   factorysessions.AsyncStartResult{SessionID: "durable-functional", Status: "QUEUED"},
		syncStartResult:    factorysessions.SyncStartResult{AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "durable-sync-functional", Status: "COMPLETED"}, SyncOutcome: "COMPLETED"},
		resumeResult:       factorysessions.AsyncStartResult{SessionID: "durable-recovered-functional", Status: "RUNNING"},
		invocationResult:   factorysessions.InvocationResult{SessionID: "live-functional", Status: factorysessions.InvocationTerminalStatusCompleted},
		controlResult:      factorysessions.LifecycleControlResult{Outcome: factorysessions.LifecycleControlOutcomeAccepted, Status: factorysessions.LifecycleStatusPaused},
		resultRead:         factorysessions.ResultReadResult{SessionID: "durable-functional", ResultStatus: factorysessions.ResultStatusNotReady},
		subscriptionCursor: &factorysessions.ResponseEventCursor{},
		liveProjection: factorysessions.SessionProjection{
			Context: factorysessions.ProjectionContext{FactorySessionID: "live-functional", Session: &factorysessions.ScopedLiveSessionSummary{ID: "live-functional"}},
			Runtime: factorysessions.RuntimeProjection{Status: "RUNNING"},
		},
		durableProjection: factorysessions.SessionReadResult{
			SessionID: "durable-functional", Status: factorysessions.LifecycleStatusSucceeded,
			ResolvedSource: factorysessions.ResolvedSource{SourceRef: "factory.js"},
		},
		liveProjections:    []factorysessions.ReadProjection{{Context: factorysessions.ProjectionContext{FactorySessionID: "live-functional"}, Runtime: factorysessions.RuntimeProjection{Status: "RUNNING"}, RuntimeAvailable: true}},
		durableProjections: []factorysessions.DurableSessionListSummary{{SessionID: "durable-functional", Status: factorysessions.LifecycleStatusSucceeded, ResolvedSource: factorysessions.ResolvedSource{SourceRef: "factory.js"}}},
		liveResult:         factorysessions.LiveSessionResult{SessionID: "live-functional", Status: "SUCCEEDED"},
		partialResult:      factorysessions.PartialSessionResult{SessionID: "live-functional", Phase: "draft"},
	}
}

func (owner *functionalDetachedOwner) OpenFactorySession(_ context.Context, request factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
	owner.openRequest = request
	return owner.openResult, nil
}

func (owner *functionalDetachedOwner) StartAsync(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	owner.asyncStartRequest = request
	return owner.asyncStartResult, nil
}

func (owner *functionalDetachedOwner) StartSync(_ context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	owner.syncStartRequest = request
	return owner.syncStartResult, nil
}

func (owner *functionalDetachedOwner) ResumeInterruptedSession(_ context.Context, _ string, request factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	owner.resumeRequest = request
	return owner.resumeResult, nil
}

func (owner *functionalDetachedOwner) InvokeFactorySession(_ context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	owner.invocationSessionID, owner.invocationRequest = sessionID, request
	return owner.invocationResult, nil
}

func (owner *functionalDetachedOwner) ActivateNamedFactory(_ context.Context, name string) error {
	owner.activatedFactory = name
	return nil
}

func (owner *functionalDetachedOwner) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return owner.liveProjection, nil
}

func (owner *functionalDetachedOwner) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	return owner.durableProjection, nil
}

func (owner *functionalDetachedOwner) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	return owner.liveProjections, nil
}

func (owner *functionalDetachedOwner) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return factorysessions.ListSessionsResult{Scope: request.Scope, DurableSessions: owner.durableProjections}, nil
}

func (owner *functionalDetachedOwner) PauseLiveFactorySession(_ context.Context, _ string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) ResumeLiveFactorySession(_ context.Context, _ string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) CloseFactorySession(_ context.Context, sessionID string) error {
	owner.closeSessionID = sessionID
	return nil
}

func (owner *functionalDetachedOwner) Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return owner.controlResult, nil
}

func (owner *functionalDetachedOwner) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return owner.resultRead, nil
}

func (owner *functionalDetachedOwner) GetFactorySessionResult(_ context.Context, sessionID string) (factorysessions.LiveSessionResult, error) {
	owner.liveResultID = sessionID
	return owner.liveResult, nil
}

func (owner *functionalDetachedOwner) GetFactorySessionPartialResult(_ context.Context, sessionID string) (factorysessions.PartialSessionResult, error) {
	owner.partialResultID = sessionID
	return owner.partialResult, nil
}

func (owner *functionalDetachedOwner) SubscribeFactoryResponseEvents(_ context.Context, request factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	owner.subscriptionRequest = request
	return owner.subscriptionCursor, nil
}
