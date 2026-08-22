package factorysessions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestLifecyclePolicyHelpersCoverStableTransitions(t *testing.T) {
	terminal := []LifecycleStatus{
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated,
	}
	for _, status := range terminal {
		if !isTerminalLifecycleStatus(status) {
			t.Fatalf("isTerminalLifecycleStatus(%q) = false", status)
		}
	}
	for _, status := range []LifecycleStatus{LifecycleStatusQueued, LifecycleStatusRunning, ""} {
		if isTerminalLifecycleStatus(status) {
			t.Fatalf("isTerminalLifecycleStatus(%q) = true", status)
		}
	}

	if !allowsRetryDispatchOnTerminal(LifecycleStatusFailed) || allowsRetryDispatchOnTerminal(LifecycleStatusSucceeded) {
		t.Fatal("retry-dispatch terminal policy is incorrect")
	}
	for _, status := range []LifecycleStatus{LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming} {
		if !allowsInterruptDispatchOnSession(status) {
			t.Fatalf("interrupt-dispatch should be allowed for %q", status)
		}
	}
	if allowsInterruptDispatchOnSession(LifecycleStatusSucceeded) {
		t.Fatal("interrupt-dispatch should be rejected for a terminal session")
	}

	cases := []struct {
		name   string
		status LifecycleStatus
		op     LifecycleControlKind
		want   LifecycleControlOutcome
	}{
		{"empty status", "", LifecycleControlPause, LifecycleControlOutcomeInvalidState},
		{"interrupted resume", LifecycleStatusInterrupted, LifecycleControlResume, LifecycleControlOutcomeAccepted},
		{"failed retry", LifecycleStatusFailed, LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted},
		{"succeeded retry", LifecycleStatusSucceeded, LifecycleControlRetryDispatch, LifecycleControlOutcomeTerminalSession},
		{"canceled cancel", LifecycleStatusCanceled, LifecycleControlCancel, LifecycleControlOutcomeNoOp},
		{"terminated terminate", LifecycleStatusTerminated, LifecycleControlTerminate, LifecycleControlOutcomeNoOp},
		{"terminal pause", LifecycleStatusSucceeded, LifecycleControlPause, LifecycleControlOutcomeTerminalSession},
		{"running pause", LifecycleStatusRunning, LifecycleControlPause, LifecycleControlOutcomeAccepted},
		{"paused pause", LifecycleStatusPaused, LifecycleControlPause, LifecycleControlOutcomeNoOp},
		{"queued pause", LifecycleStatusQueued, LifecycleControlPause, LifecycleControlOutcomeInvalidState},
		{"paused resume", LifecycleStatusPaused, LifecycleControlResume, LifecycleControlOutcomeAccepted},
		{"resuming resume", LifecycleStatusResuming, LifecycleControlResume, LifecycleControlOutcomeNoOp},
		{"queued resume", LifecycleStatusQueued, LifecycleControlResume, LifecycleControlOutcomeInvalidState},
		{"canceling cancel", LifecycleStatusCanceling, LifecycleControlCancel, LifecycleControlOutcomeNoOp},
		{"running cancel", LifecycleStatusRunning, LifecycleControlCancel, LifecycleControlOutcomeAccepted},
		{"failed cancel", LifecycleStatusFailed, LifecycleControlCancel, LifecycleControlOutcomeTerminalSession},
		{"queued terminate", LifecycleStatusQueued, LifecycleControlTerminate, LifecycleControlOutcomeAccepted},
		{"failed terminate", LifecycleStatusFailed, LifecycleControlTerminate, LifecycleControlOutcomeTerminalSession},
		{"awaiting approval", LifecycleStatusAwaitingApproval, LifecycleControlApprove, LifecycleControlOutcomeAccepted},
		{"running approve", LifecycleStatusRunning, LifecycleControlApprove, LifecycleControlOutcomeInvalidState},
		{"running retry", LifecycleStatusRunning, LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted},
		{"queued retry", LifecycleStatusQueued, LifecycleControlRetryDispatch, LifecycleControlOutcomeInvalidState},
		{"unknown operation", LifecycleStatusRunning, LifecycleControlKind("unknown"), LifecycleControlOutcomeInvalidState},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := evaluateLifecycleControl(test.op, test.status); got != test.want {
				t.Fatalf("evaluateLifecycleControl(%q, %q) = %q, want %q", test.op, test.status, got, test.want)
			}
		})
	}
}

func TestLifecycleProjectionHelpersBuildDetachedValues(t *testing.T) {
	withoutEvents := inspectionLinksForSession("session-a", false)
	if withoutEvents.Events != "" || withoutEvents.Results != "/factory-sessions/session-a/results" {
		t.Fatalf("inspection links without events = %#v", withoutEvents)
	}
	withEvents := lifecycleControlLinksForSession("session-a", true)
	if withEvents.Events != "/factory-sessions/session-a/events" || withEvents.Dispatches == "" {
		t.Fatalf("lifecycle control links = %#v", withEvents)
	}
	if got := emptySessionUsage(); got.Resources == nil || len(got.Resources) != 0 {
		t.Fatalf("empty session usage = %#v, want non-nil empty resources", got)
	}

	for _, test := range []struct {
		state string
		want  LifecycleStatus
	}{
		{"RUNNING", LifecycleStatusRunning},
		{" idle ", LifecycleStatusRunning},
		{"PAUSED", LifecycleStatusPaused},
		{"COMPLETED", LifecycleStatusSucceeded},
		{"FAILED", LifecycleStatusFailed},
		{"unknown", ""},
	} {
		if got := lifecycleStatusFromFactoryRuntimeState(test.state); got != test.want {
			t.Fatalf("lifecycleStatusFromFactoryRuntimeState(%q) = %q, want %q", test.state, got, test.want)
		}
	}

	if got := liveLifecycleControlLinksForSession(" session-a "); got.Results != "/factory-sessions/session-a/result" {
		t.Fatalf("live lifecycle links = %#v", got)
	}
	if got := liveLifecycleControlLogFields("session-a", LifecycleControlPause, "ACCEPTED", LifecycleStatusRunning, ControlRequest{}); len(got) != 4 {
		t.Fatalf("log fields without request id = %d, want 4", len(got))
	}
	if got := liveLifecycleControlLogFields("session-a", LifecycleControlPause, "ACCEPTED", "", ControlRequest{RequestID: "request-a"}); len(got) != 4 {
		t.Fatalf("log fields with request id = %d, want 4", len(got))
	}
}

func TestLifecycleOutcomeClassAndEventMaterialization(t *testing.T) {
	if got := lifecycleControlOutcomeClass("", ErrDurableSessionNotFound); got != LifecycleControlOutcomeClassNotFound {
		t.Fatalf("not-found outcome class = %q", got)
	}
	controlErr := &ControlError{Outcome: LifecycleControlOutcomeConflict}
	if got := lifecycleControlOutcomeClass("", controlErr); got != string(LifecycleControlOutcomeConflict) {
		t.Fatalf("control-error outcome class = %q", got)
	}
	if got := lifecycleControlOutcomeClass("", errors.New("boom")); got != "ERROR" {
		t.Fatalf("generic outcome class = %q", got)
	}
	if got := lifecycleControlOutcomeClass("", nil); got != "ERROR" {
		t.Fatalf("empty outcome class = %q", got)
	}

	payload := json.RawMessage(`{"value":1}`)
	raw, err := json.Marshal(factorydefinitions.FactoryEvent{Id: "event-a", Payload: payload})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	stream := materializeEventReadStream(EventReadResult{
		Events: []json.RawMessage{raw, json.RawMessage("{")},
	})
	if len(stream.History) != 1 || stream.History[0].Id != "event-a" || string(stream.History[0].Payload) != string(payload) {
		t.Fatalf("materialized history = %#v", stream.History)
	}
	select {
	case _, ok := <-stream.Events:
		if ok {
			t.Fatal("materialized event stream channel is open")
		}
	default:
		t.Fatal("materialized event stream channel was not closed")
	}

	validation := newValidationError("name", "name is required")
	if validation.Field != "name" || validation.Message != "name is required" {
		t.Fatalf("validation error = %#v", validation)
	}
}

type detachedOperationsServiceFake struct {
	Service

	openRequest            OpenRequest
	openResult             *OpenResult
	asyncStartRequest      StartRequest
	asyncStartResult       AsyncStartResult
	syncStartRequest       StartRequest
	syncStartResult        SyncStartResult
	resumeRequest          ResumeSessionRequest
	resumeResult           AsyncStartResult
	invocationSessionID    string
	invocationRequest      InvocationRequest
	invocationResult       InvocationResult
	activatedFactory       string
	getLiveProjection      SessionProjection
	getDurableProjection   SessionReadResult
	listLiveProjections    []ReadProjection
	listDurableProjections []DurableSessionListSummary
	controlRequest         ControlRequest
	controlResult          LifecycleControlResult
	closeSessionID         string
	resultRead             ResultReadResult
	liveResult             factoryruntime.LiveSessionResult
	partialResult          factoryruntime.PartialSessionResult
	subscriptionRequest    ResponseEventSubscriptionRequest
	subscriptionCursor     *ResponseEventCursor
}

func newDetachedOperationsServiceFake() *detachedOperationsServiceFake {
	return &detachedOperationsServiceFake{
		openResult:         &OpenResult{SessionID: "live-opened"},
		asyncStartResult:   AsyncStartResult{SessionID: "durable-started", Status: "QUEUED"},
		syncStartResult:    SyncStartResult{AsyncStartResult: AsyncStartResult{SessionID: "durable-sync"}, SyncOutcome: "COMPLETED"},
		resumeResult:       AsyncStartResult{SessionID: "durable-recovered", Status: "RUNNING"},
		invocationResult:   InvocationResult{SessionID: "live-existing", Status: InvocationTerminalStatusCompleted},
		controlResult:      LifecycleControlResult{Outcome: LifecycleControlOutcomeAccepted, Status: LifecycleStatusPaused},
		resultRead:         ResultReadResult{SessionID: "durable-existing", ResultStatus: ResultStatusNotReady},
		liveResult:         factoryruntime.LiveSessionResult{SessionID: "live-existing", Status: "SUCCEEDED"},
		partialResult:      factoryruntime.PartialSessionResult{SessionID: "live-existing", Phase: "draft"},
		subscriptionCursor: &ResponseEventCursor{},
	}
}

func (fake *detachedOperationsServiceFake) OpenFactorySession(_ context.Context, request OpenRequest) (*OpenResult, error) {
	fake.openRequest = request
	return fake.openResult, nil
}

func (fake *detachedOperationsServiceFake) StartAsync(_ context.Context, request StartRequest) (AsyncStartResult, error) {
	fake.asyncStartRequest = request
	return fake.asyncStartResult, nil
}

func (fake *detachedOperationsServiceFake) StartSync(_ context.Context, request StartRequest) (SyncStartResult, error) {
	fake.syncStartRequest = request
	return fake.syncStartResult, nil
}

func (fake *detachedOperationsServiceFake) ResumeInterruptedSession(_ context.Context, _ string, request ResumeSessionRequest) (AsyncStartResult, error) {
	fake.resumeRequest = request
	return fake.resumeResult, nil
}

func (fake *detachedOperationsServiceFake) InvokeFactorySession(_ context.Context, sessionID string, request InvocationRequest) (InvocationResult, error) {
	fake.invocationSessionID = sessionID
	fake.invocationRequest = request
	return fake.invocationResult, nil
}

func (fake *detachedOperationsServiceFake) ActivateNamedFactory(_ context.Context, name string) error {
	fake.activatedFactory = name
	return nil
}

func (fake *detachedOperationsServiceFake) GetFactorySession(context.Context, string) (SessionProjection, error) {
	return fake.getLiveProjection, nil
}
func (fake *detachedOperationsServiceFake) GetSession(context.Context, string) (SessionReadResult, error) {
	return fake.getDurableProjection, nil
}
func (fake *detachedOperationsServiceFake) ListFactorySessions(context.Context) ([]ReadProjection, error) {
	return fake.listLiveProjections, nil
}
func (fake *detachedOperationsServiceFake) ListSessions(context.Context, ListSessionsRequest) (ListSessionsResult, error) {
	return ListSessionsResult{DurableSessions: fake.listDurableProjections}, nil
}

func (fake *detachedOperationsServiceFake) PauseLiveFactorySession(_ context.Context, _ string, request ControlRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) ResumeLiveFactorySession(_ context.Context, _ string, request ControlRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) CloseFactorySession(_ context.Context, sessionID string) error {
	fake.closeSessionID = sessionID
	return nil
}
func (fake *detachedOperationsServiceFake) Pause(_ context.Context, _ string, request ControlRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) Resume(_ context.Context, _ string, request ControlRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) Cancel(_ context.Context, _ string, request ControlRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) Terminate(_ context.Context, _ string, request ControlRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request
	return fake.controlResult, nil
}

func (fake *detachedOperationsServiceFake) Approve(_ context.Context, _ string, request ApproveRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request.ControlRequest
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) RetryDispatch(_ context.Context, _ string, request RetryDispatchRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request.ControlRequest
	return fake.controlResult, nil
}
func (fake *detachedOperationsServiceFake) InterruptDispatch(_ context.Context, _ string, request InterruptDispatchRequest) (LifecycleControlResult, error) {
	fake.controlRequest = request.ControlRequest
	return fake.controlResult, nil
}

func (fake *detachedOperationsServiceFake) GetResult(context.Context, string, ResultRequest) (ResultReadResult, error) {
	return fake.resultRead, nil
}
func (fake *detachedOperationsServiceFake) GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error) {
	return fake.liveResult, nil
}
func (fake *detachedOperationsServiceFake) GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error) {
	return fake.partialResult, nil
}
func (fake *detachedOperationsServiceFake) SubscribeFactoryResponseEvents(_ context.Context, request ResponseEventSubscriptionRequest) (*ResponseEventCursor, error) {
	fake.subscriptionRequest = request
	return fake.subscriptionCursor, nil
}

func TestDetachedOperationsBindRejectsMissingOwner(t *testing.T) {
	if operations, err := (&DetachedOperations{}).Bind(nil); operations != nil || !errors.Is(err, ErrDetachedServiceUnavailable) {
		t.Fatalf("DetachedOperations.Bind(nil) = (%#v, %v), want unavailable error and nil operations", operations, err)
	}
}

func TestDetachedOperationsPrepareSyncIsInertAndClonesValues(t *testing.T) {
	fake := newDetachedOperationsServiceFake()
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}

	request := SessionSyncPreparationRequest{
		Start: SessionStartRequest{
			Mode:        SessionOperationModeDurable,
			Correlation: SessionOperationCorrelation{RequestID: "prepare-1"},
			Args:        map[string]any{"nested": map[string]any{"value": "original"}},
			Wait:        SessionOperationWait{TimeoutMillis: 250},
		},
		Wait: SessionOperationWait{CancelOnTimeout: true},
	}
	prepared, err := operations.PrepareSync(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareSync() error = %v", err)
	}
	if !prepared.Request.Synchronous || prepared.Wait.TimeoutMillis != 0 || !prepared.Wait.CancelOnTimeout {
		t.Fatalf("PrepareSync() = %#v, want synchronous request with selected wait values", prepared)
	}
	preparedArgs := prepared.Request.Args["nested"].(map[string]any)
	request.Start.Args["nested"].(map[string]any)["value"] = "mutated"
	if preparedArgs["value"] != "original" {
		t.Fatalf("PrepareSync() retained caller-owned nested map: %#v", preparedArgs)
	}
	if operations.owner != fake {
		t.Fatal("detached operations did not retain the composed owner")
	}
}

func TestDetachedOperationsStartForwardsLiveValues(t *testing.T) {
	fake := newDetachedOperationsServiceFake()
	fake.openResult.Session = &ScopedLiveSessionSummary{
		ID:      "live-opened",
		Runtime: &RuntimeProjection{Status: "RUNNING"},
	}
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}

	liveStart, err := operations.Start(context.Background(), SessionStartRequest{
		Mode:       SessionOperationModeLive,
		FolderPath: "  /workspace  ",
		Target:     &TargetRef{Kind: TargetKindNamed, Name: "demo"},
	})
	if err != nil {
		t.Fatalf("live Start() error = %v", err)
	}
	if liveStart.SessionID != "live-opened" {
		t.Fatalf("live Start() session ID = %q, want live-opened", liveStart.SessionID)
	}
	if fake.openRequest.FolderPath != "/workspace" {
		t.Fatalf("live Start() folder path = %q, want /workspace", fake.openRequest.FolderPath)
	}
	if liveStart.Live == nil {
		t.Fatal("live Start() returned no live result")
	}
	if liveStart.Live.Session == nil {
		t.Fatal("live Start() returned no session view")
	}
	if liveStart.Live.Session.Status != "RUNNING" {
		t.Fatalf("live Start() status = %q, want RUNNING", liveStart.Live.Session.Status)
	}
	if !liveStart.Live.Session.RuntimeAvailable {
		t.Fatal("live Start() should report an available runtime")
	}
}

func TestDetachedOperationsStartForwardsDurableValues(t *testing.T) {
	fake := newDetachedOperationsServiceFake()
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}
	durableStart, err := operations.Start(context.Background(), SessionStartRequest{
		Mode:        SessionOperationModeDurable,
		Correlation: SessionOperationCorrelation{RequestID: "start-1"},
		Input: &work.PreparedInvocationInput{
			NormalizedArguments: &work.NormalizedArguments{
				Arguments: map[string]work.NormalizedArgument{
					"goal": {Values: []string{"ship"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("durable Start() error = %v", err)
	}
	if durableStart.SessionID != "durable-started" {
		t.Fatalf("durable Start() session ID = %q, want durable-started", durableStart.SessionID)
	}
	if fake.asyncStartRequest.EventConsumer != nil {
		t.Fatal("durable Start() installed an event consumer")
	}
	if fake.asyncStartRequest.Args["goal"] != "ship" {
		t.Fatalf("durable Start() goal = %#v, want ship", fake.asyncStartRequest.Args["goal"])
	}
}

func TestDetachedOperationsStartForwardsSynchronousValues(t *testing.T) {
	fake := newDetachedOperationsServiceFake()
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}
	synchronousStart, err := operations.Start(context.Background(), SessionStartRequest{
		Mode:        SessionOperationModeDurable,
		Correlation: SessionOperationCorrelation{RequestID: "sync-start-1"},
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("synchronous Start() error = %v", err)
	}
	if synchronousStart.Sync == nil {
		t.Fatal("synchronous Start() returned no sync result")
	}
	if synchronousStart.SessionID != "durable-sync" {
		t.Fatalf("synchronous Start() session ID = %q, want durable-sync", synchronousStart.SessionID)
	}
	if fake.syncStartRequest.EventConsumer != nil {
		t.Fatal("synchronous Start() installed an event consumer")
	}
}

func TestDetachedOperationsInvokeForwardsPreparedWork(t *testing.T) {
	fake := newDetachedOperationsServiceFake()
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}
	_, err = operations.Invoke(context.Background(), SessionInvokeRequest{
		SessionID:   "live-existing",
		Correlation: SessionOperationCorrelation{RequestID: "invoke-1"},
		Input: &work.PreparedInvocationInput{
			Source: work.InputSourcePositionalText,
		},
		Wait: SessionOperationWait{TimeoutMillis: 500},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if fake.invocationSessionID != "live-existing" || fake.invocationRequest.RequestID == nil || *fake.invocationRequest.RequestID != "invoke-1" || fake.invocationRequest.TimeoutMillis == nil || *fake.invocationRequest.TimeoutMillis != 500 {
		t.Fatalf("Invoke() legacy request = %#v", fake.invocationRequest)
	}
	if fake.invocationRequest.PreparedInvocationInput == nil || fake.invocationRequest.PreparedInvocationInput.Source != work.InputSourcePositionalText {
		t.Fatalf("Invoke() lost prepared Work input: %#v", fake.invocationRequest.PreparedInvocationInput)
	}
}

func TestDetachedOperationsActivateAndSubscribe(t *testing.T) {
	fake := newDetachedOperationsServiceFake()
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}
	ctx := context.Background()
	activated, err := operations.Activate(ctx, SessionActivateRequest{SessionID: "live-existing", FactoryName: "  named-factory  "})
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if !activated.Activated || activated.SessionID != "live-existing" || fake.activatedFactory != "named-factory" {
		t.Fatalf("Activate() result = %#v, factory = %q, want activated trimmed name", activated, fake.activatedFactory)
	}

	subscription, err := operations.Subscribe(ctx, SessionResponseSubscriptionRequest{
		SessionID:     "live-existing",
		AfterSequence: 4,
		DispatchID:    "dispatch-1",
		Kinds:         []ResponseEventKind{ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if subscription.Cursor != fake.subscriptionCursor || fake.subscriptionRequest.AfterSequence != 4 || len(fake.subscriptionRequest.Kinds) != 1 {
		t.Fatalf("Subscribe() = %#v, request = %#v", subscription, fake.subscriptionRequest)
	}
}

func newDetachedReadOperations(t *testing.T) (*detachedOperationsServiceFake, *DetachedOperations) {
	t.Helper()
	fake := newDetachedOperationsServiceFake()
	fake.getLiveProjection = SessionProjection{
		Context: ProjectionContext{
			FactorySessionID: "live-existing",
			Session: &ScopedLiveSessionSummary{
				ID:         "live-existing",
				FolderPath: "/workspace",
				Target:     TargetRef{Kind: TargetKindDefault},
			},
		},
		Runtime: RuntimeProjection{Status: "RUNNING"},
	}
	fake.getDurableProjection = SessionReadResult{
		SessionID:        "durable-existing",
		Status:           LifecycleStatusSucceeded,
		OrchestratorKind: "javascript",
		ResolvedSource:   ResolvedSource{SourceRef: "factory.yaml"},
		SourceHash:       "sha256:source",
	}
	fake.listLiveProjections = []ReadProjection{{
		Context:          ProjectionContext{FactorySessionID: "live-existing"},
		Runtime:          RuntimeProjection{Status: "RUNNING"},
		RuntimeAvailable: true,
	}}
	fake.listDurableProjections = []DurableSessionListSummary{{
		SessionID:        "durable-existing",
		Status:           LifecycleStatusSucceeded,
		OrchestratorKind: "javascript",
		ResolvedSource:   ResolvedSource{SourceRef: "factory.yaml"},
	}}
	operations, err := (&DetachedOperations{}).Bind(fake)
	if err != nil {
		t.Fatalf("DetachedOperations.Bind() error = %v", err)
	}
	return fake, operations
}

func TestDetachedOperationsReadListUsesModeSpecificOwners(t *testing.T) {
	fake, operations := newDetachedReadOperations(t)
	ctx := context.Background()
	live, err := operations.Get(ctx, SessionGetRequest{SessionID: "live-existing", Mode: SessionOperationModeLive})
	if err != nil || live.Session.SessionID != "live-existing" || live.Session.Status != "RUNNING" {
		t.Fatalf("live Get() = %#v, error = %v", live, err)
	}
	durable, err := operations.Get(ctx, SessionGetRequest{SessionID: "durable-existing", Mode: SessionOperationModeDurable})
	if err != nil || durable.Session.SourceRef != "factory.yaml" || durable.Session.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("durable Get() = %#v, error = %v", durable, err)
	}
	listed, err := operations.List(ctx, SessionListRequest{Mode: SessionOperationModeAll})
	if err != nil || len(listed.Sessions) != 2 || listed.Sessions[0].Mode != SessionOperationModeLive || listed.Sessions[1].Mode != SessionOperationModeDurable {
		t.Fatalf("List(all) = %#v, error = %v", listed, err)
	}
	if fake.listLiveProjections[0].Context.FactorySessionID != "live-existing" {
		t.Fatal("List() did not retain the live owner projection identity")
	}
}

func TestDetachedOperationsControlUsesModeSpecificOwners(t *testing.T) {
	fake, operations := newDetachedReadOperations(t)
	ctx := context.Background()
	controlled, err := operations.Control(ctx, SessionControlRequest{
		SessionID: "live-existing",
		Mode:      SessionOperationModeLive,
		Operation: SessionControlPause,
		Correlation: SessionOperationCorrelation{
			RequestID: "control-1",
			TurnID:    "turn-1",
		},
	})
	if err != nil || controlled.Outcome != LifecycleControlOutcomeAccepted || fake.controlRequest.RequestID != "control-1" || fake.controlRequest.TurnID != "turn-1" {
		t.Fatalf("Control(pause) = %#v, request = %#v, error = %v", controlled, fake.controlRequest, err)
	}
	closed, err := operations.Control(ctx, SessionControlRequest{SessionID: "live-existing", Mode: SessionOperationModeLive, Operation: SessionControlClose})
	if err != nil || !closed.Closed || fake.closeSessionID != "live-existing" {
		t.Fatalf("Control(close) = %#v, closed session = %q, error = %v", closed, fake.closeSessionID, err)
	}
	recovered, err := operations.Control(ctx, SessionControlRequest{
		SessionID:   "durable-existing",
		Mode:        SessionOperationModeDurable,
		Operation:   SessionControlRecover,
		Correlation: SessionOperationCorrelation{RequestID: "recover-1"},
	})
	if err != nil || recovered.Recovery == nil || recovered.Recovery.SessionID != "durable-recovered" || fake.resumeRequest.RequestID != "recover-1" {
		t.Fatalf("Control(recover) = %#v, resume request = %#v, error = %v", recovered, fake.resumeRequest, err)
	}
}

func TestDetachedOperationsReadResultMapsDetachedValues(t *testing.T) {
	_, operations := newDetachedReadOperations(t)
	ctx := context.Background()
	read, err := operations.ReadResult(ctx, SessionResultReadRequest{SessionID: "durable-existing", Mode: SessionOperationModeDurable, Request: ResultRequest{Mode: ResultModeFinal}})
	if err != nil || read.Durable == nil || read.Status != string(ResultStatusNotReady) {
		t.Fatalf("ReadResult(durable) = %#v, error = %v", read, err)
	}
	partial, err := operations.ReadResult(ctx, SessionResultReadRequest{SessionID: "live-existing", Mode: SessionOperationModeLive, Request: ResultRequest{Mode: ResultModePartial}})
	if err != nil || partial.Live == nil || partial.Status != "PARTIAL" {
		t.Fatalf("ReadResult(live partial) = %#v, error = %v", partial, err)
	}
}
func TestLiveChangeContractsNormalizeAndClassifyErrors(t *testing.T) {
	assertLiveChangeErrorClassification(t)
	assertLiveChangeNormalization(t)
}

func assertLiveChangeErrorClassification(t *testing.T) {
	t.Helper()
	var nilError *LiveChangeError
	if nilError.Error() != "live change error" || nilError.Unwrap() != nil || nilError.Is(nil) {
		t.Fatal("nil LiveChangeError methods are not stable")
	}
	invalid := NewLiveChangeError(LiveChangeErrorInvalidRequest, "  invalid  ")
	if invalid.Error() != "invalid" || !errors.Is(invalid, ErrLiveChangeInvalidRequest) {
		t.Fatalf("NewLiveChangeError() = %v", invalid)
	}
	notFound := &LiveChangeError{Code: LiveChangeErrorSessionNotFound}
	if !errors.Is(notFound, ErrLiveChangeSessionNotFound) || !errors.Is(notFound, ErrSessionNotFound) {
		t.Fatal("session-not-found live change error lost stable sentinels")
	}
	wrapped := errors.New("cause")
	if !errors.Is((&LiveChangeError{Cause: wrapped}).Unwrap(), wrapped) {
		t.Fatal("live change cause was not retained for local matching")
	}
}

func assertLiveChangeNormalization(t *testing.T) {
	t.Helper()
	normalized, err := NormalizeLiveChangeRequest(LiveChangeRequest{
		RequestID:        " request-a ",
		ExpectedRevision: 2,
		Operation:        "  SET ",
		TargetID:         " target-a ",
		RequestedValue:   json.RawMessage(`{"b":2,"a":1}`),
		Reason:           "  operator   request  ",
	})
	if err != nil {
		t.Fatalf("NormalizeLiveChangeRequest() error = %v", err)
	}
	if normalized.ChangeID != "live-change/request-a" || normalized.Operation != "set" || normalized.Reason != "operator request" || string(normalized.RequestedValue) != `{"a":1,"b":2}` {
		t.Fatalf("normalized live change = %#v", normalized)
	}
	for _, request := range []LiveChangeRequest{
		{ExpectedRevision: -1, RequestID: "id", Operation: "set", TargetID: "target", RequestedValue: json.RawMessage(`1`)},
		{RequestID: "id", TargetID: "target", RequestedValue: json.RawMessage(`1`)},
		{RequestID: "id", Operation: "set", RequestedValue: json.RawMessage(`1`)},
		{RequestID: "id", Operation: "set", TargetID: "target", RequestedValue: json.RawMessage("{")},
	} {
		if _, err := NormalizeLiveChangeRequest(request); err == nil {
			t.Fatalf("NormalizeLiveChangeRequest(%#v) unexpectedly succeeded", request)
		}
	}
}
