package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestChildWorkerExecutor_UsesNativeAndNarrowerTimeoutBounds(t *testing.T) {
	request := factory.JavaScriptChildExecutionRequest{
		Prompt:           "wait for the provider",
		ExecutorProvider: "SCRIPT_WRAP",
		ModelProvider:    "antigravity",
		Model:            "gemini-3.6-flash-medium",
	}

	for _, test := range []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{
			name: "native default",
			want: providers.DefaultAntigravityPrintTimeout,
		},
		{
			name:       "narrow configured bound",
			configured: 25 * time.Millisecond,
			want:       25 * time.Millisecond,
		},
		{
			name:       "wider configured bound uses native default",
			configured: 6 * time.Minute,
			want:       providers.DefaultAntigravityPrintTimeout,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
			}}
			executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)
			executor.maxWorkerDuration = test.configured

			if _, err := executor.Execute(context.Background(), request); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if invoker.request.Target.Timeout != test.want {
				t.Fatalf("child timeout = %s, want %s", invoker.request.Target.Timeout, test.want)
			}
		})
	}
}

func TestJavaScriptRuntimeService_ChildPolicySetsNarrowerWorkerTimeout(t *testing.T) {
	maxWorkerDurationMs := int64(23)
	policy := factory.DefaultJavaScriptPolicy()
	policy.MaxWorkerDurationMs = &maxWorkerDurationMs

	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
	}}
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	service.SetDirectWorkerExecution(invoker)

	hooks := service.childExecutorHooks(ChildExecutorModeLive, "timeout-policy-session")
	if hooks.NewChildExecutor == nil {
		t.Fatal("child executor hook = nil")
	}
	_, err := hooks.NewChildExecutor("timeout-policy-session", newChildRecordSink(), policy).Execute(
		context.Background(),
		factory.JavaScriptChildExecutionRequest{
			Prompt:           "wait for the provider",
			ExecutorProvider: "SCRIPT_WRAP",
			ModelProvider:    "antigravity",
			Model:            "gemini-3.6-flash-medium",
		},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if invoker.request.Target.Timeout != 23*time.Millisecond {
		t.Fatalf("policy child timeout = %s, want 23ms", invoker.request.Target.Timeout)
	}
}

func TestChildWorkerExecutor_TimeoutReleasesCapacityAndSuppressesLateSuccess(t *testing.T) {
	late := newLateChildExecution()
	defer late.releaseAfterCancel()

	sink := newChildRecordSink()
	workerReleases := 0
	executor := newTestChildWorkerExecutor(late, sink, func(_, _ string) func() {
		return func() { workerReleases++ }
	})
	executor.maxWorkerDuration = 20 * time.Millisecond

	capacity := make(chan struct{}, 1)
	capacity <- struct{}{}
	executor.resourceLeaseAcquirer = func(_ context.Context, _ factory.ResourceCapacityLeaseRequest) (*childResourceLease, error) {
		select {
		case <-capacity:
			return &childResourceLease{release: func() { capacity <- struct{}{} }}, nil
		default:
			return nil, context.DeadlineExceeded
		}
	}

	var completed workers.ExecuteResult
	var completeErr error
	completeCalls := 0
	executor.attemptStarter = func(_ context.Context, _ workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error) {
		return func(_ context.Context, result workers.ExecuteResult, err error) error {
			completeCalls++
			completed = result
			completeErr = err
			return nil
		}, nil
	}

	var progressMu sync.Mutex
	var progress []workers.ProgressFragment
	executor.publish = func(_ string, fragment workers.ProgressFragment) {
		progressMu.Lock()
		progress = append(progress, fragment)
		progressMu.Unlock()
	}

	result, err := executor.Execute(context.Background(), timeoutChildRequest())
	if err == nil {
		t.Fatal("Execute error = nil, want the typed timeout to reject the child")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "antigravity") ||
		!strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("timeout error = %q, want provider and timeout reason", err)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}

	waitForChildTimeoutSignal(t, late.canceled, "provider cancellation")
	late.releaseAfterCancel()
	waitForChildTimeoutSignal(t, late.returned, "late provider return")
	assertTimeoutChildTerminal(t, sink)
	if completeCalls != 1 || completed.Failure == nil || completed.Failure.Type != workers.WorkFailureTypeTimeout || completeErr == nil {
		t.Fatalf("completed attempt = calls:%d result:%#v err:%v, want one typed timeout", completeCalls, completed, completeErr)
	}
	if workerReleases != 1 {
		t.Fatalf("worker dispatch releases = %d, want exactly one", workerReleases)
	}
	assertTimeoutCapacityReleased(t, capacity)
	progressMu.Lock()
	deferredProgress := append([]workers.ProgressFragment(nil), progress...)
	progressMu.Unlock()
	if len(deferredProgress) != 1 || deferredProgress[0].Kind != workers.FailedFragmentKind ||
		deferredProgress[0].Metadata["work_failure_type"] != string(workers.WorkFailureTypeTimeout) {
		t.Fatalf("terminal progress = %#v, want exactly one typed timeout", deferredProgress)
	}
}

func timeoutChildRequest() factory.JavaScriptChildExecutionRequest {
	return factory.JavaScriptChildExecutionRequest{
		Prompt:           "wait for Antigravity",
		ExecutorProvider: "SCRIPT_WRAP",
		ModelProvider:    "antigravity",
		Model:            "gemini-3.6-flash-medium",
		ResourceID:       "reviewers",
	}
}

func assertTimeoutChildTerminal(t *testing.T, sink *childRecordSink) {
	t.Helper()
	terminalCount := 0
	for _, record := range sink.records {
		if record.ChildDispatch != nil && record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusFailed {
			terminalCount++
		}
	}
	if terminalCount != 1 {
		t.Fatalf("failed child records = %d, want exactly one timeout terminal", terminalCount)
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Provider != "antigravity" {
		t.Fatalf("terminal provider = %q, want antigravity", terminal.Provider)
	}
	if terminal.FailureClassification != workers.WorkFailureTypeTimeout {
		t.Fatalf("terminal failure type = %q, want timeout", terminal.FailureClassification)
	}
	if terminal.FailureDetail == nil || terminal.FailureDetail.Reason != workers.WorkFailureTypeTimeout ||
		!strings.Contains(terminal.FailureDetail.Message, "timed out") {
		t.Fatalf("terminal failure detail = %#v, want typed timeout reason", terminal.FailureDetail)
	}
	if terminal.Retryable == nil || *terminal.Retryable {
		t.Fatalf("terminal retryable = %#v, want false", terminal.Retryable)
	}
}

func assertTimeoutCapacityReleased(t *testing.T, capacity chan struct{}) {
	t.Helper()
	select {
	case <-capacity:
		capacity <- struct{}{}
	default:
		t.Fatal("child capacity was not released for a subsequent dispatch")
	}
}

type lateChildExecution struct {
	canceled     chan struct{}
	release      chan struct{}
	returned     chan struct{}
	cancelOnce   sync.Once
	releaseOnce  sync.Once
	returnedOnce sync.Once
}

func newLateChildExecution() *lateChildExecution {
	return &lateChildExecution{
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
		returned: make(chan struct{}),
	}
}

func (e *lateChildExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	<-ctx.Done()
	e.cancelOnce.Do(func() { close(e.canceled) })
	<-e.release
	if request.Input.ProgressPublisher != nil {
		request.Input.ProgressPublisher(workers.ProgressFragment{
			Kind:    workers.CompletedFragmentKind,
			Type:    "COMPLETED",
			Payload: "late success",
		})
	}
	e.returnedOnce.Do(func() { close(e.returned) })
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
	}, nil
}

func (e *lateChildExecution) releaseAfterCancel() {
	e.releaseOnce.Do(func() { close(e.release) })
}

func waitForChildTimeoutSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}

func TestIsTerminalLifecycleStatus(t *testing.T) {
	t.Parallel()
	terminal := []LifecycleStatus{
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated,
	}
	for _, status := range terminal {
		if !IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be terminal", status)
		}
		if status != LifecycleStatusFailed && AllowsRetryDispatchOnTerminal(status) {
			t.Fatalf("retry-dispatch should be rejected on terminal status %q", status)
		}
	}
	if !AllowsRetryDispatchOnTerminal(LifecycleStatusFailed) {
		t.Fatal("retry-dispatch should remain allowed on FAILED terminal sessions")
	}
	active := []LifecycleStatus{
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleStatusCanceling,
	}
	for _, status := range active {
		if IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be active", status)
		}
	}
}

func TestEvaluateLifecycleControl_ValidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		operation LifecycleControlKind
		status    LifecycleStatus
		want      LifecycleControlOutcome
	}{
		{LifecycleControlPause, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlPause, LifecycleStatusPaused, LifecycleControlOutcomeNoOp},
		{LifecycleControlResume, LifecycleStatusPaused, LifecycleControlOutcomeAccepted},
		{LifecycleControlResume, LifecycleStatusInterrupted, LifecycleControlOutcomeAccepted},
		{LifecycleControlResume, LifecycleStatusRunning, LifecycleControlOutcomeNoOp},
		{LifecycleControlCancel, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlCancel, LifecycleStatusCanceling, LifecycleControlOutcomeNoOp},
		{LifecycleControlTerminate, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlApprove, LifecycleStatusAwaitingApproval, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusFailed, LifecycleControlOutcomeAccepted},
	}
	for _, tc := range cases {
		got := EvaluateLifecycleControl(tc.operation, tc.status)
		if got != tc.want {
			t.Fatalf("%s on %s = %q, want %q", tc.operation, tc.status, got, tc.want)
		}
	}
}

func TestEvaluateLifecycleControl_InvalidAndTerminal(t *testing.T) {
	t.Parallel()
	if got := EvaluateLifecycleControl(LifecycleControlPause, LifecycleStatusAwaitingApproval); got != LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %q, want INVALID_STATE", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlRetryDispatch, LifecycleStatusSucceeded); got != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on succeeded = %q, want TERMINAL_SESSION", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlCancel, LifecycleStatusCanceled); got != LifecycleControlOutcomeNoOp {
		t.Fatalf("cancel on canceled = %q, want NO_OP", got)
	}
}

func TestNormalizeRetryDispatchRequest_RequiresDispatchID(t *testing.T) {
	t.Parallel()
	_, err := NormalizeRetryDispatchRequest(RetryDispatchRequest{})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestControlIdempotencyTupleHash_IsStable(t *testing.T) {
	t.Parallel()
	retry := RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "req-retry-001"},
		DispatchID:     "disp-js-success-002",
	}
	first, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry, InterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry, InterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatalf("hash mismatch: %q vs %q", first, second)
	}
}

func TestCheckControlRequestIDReplay_Conflict(t *testing.T) {
	t.Parallel()
	err := CheckControlRequestIDReplay("req-1", "sha256:abc", "sha256:def")
	if !errors.Is(err, ErrControlRequestIDConflict) {
		t.Fatalf("error = %v, want ErrControlRequestIDConflict", err)
	}
}

func TestJavaScriptRuntimeService_ControlWrappersAndDetailReaders(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions["dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] = newJavaScriptRuntimeRunningControlState(now)

	t.Run("detail readers and running controls", func(t *testing.T) {
		testJavaScriptRuntimeServiceRunningControlWrappers(t, service)
	})
	t.Run("approve awaiting session", func(t *testing.T) {
		testJavaScriptRuntimeServiceApproveAwaitingSession(t, service)
	})
	t.Run("retry failed dispatch", func(t *testing.T) {
		testJavaScriptRuntimeServiceRetryFailedDispatch(t, service)
	})
}

func newJavaScriptRuntimeRunningControlState(now time.Time) *runtimeSessionState {
	return &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			ResolvedSource: ResolvedSource{
				SourceRef: "inline",
			},
			Links: InspectionLinksForSession("dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{
			{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1},
		},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"disp-1": {DispatchStatusQueued, DispatchStatusFailed},
		},
		dispatchJavaScript: map[string]DispatchJavaScriptProjection{
			"disp-1": {TaskLabel: "child"},
		},
		artifacts: []ArtifactSummary{
			{ID: "art-1"},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{
				SessionID:        "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Status:           LifecycleStatusRunning,
				OrchestratorKind: interfaces.OrchestratorKindJavaScript,
				Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			},
			ResultReadResult{
				SessionID:     "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SessionStatus: LifecycleStatusRunning,
				ResultStatus:  ResultStatusNotReady,
				Availability: &ResultAvailabilityDetail{
					Reason:    "RESULT_NOT_READY",
					Message:   "Session is still running.",
					Retryable: true,
				},
			},
		),
	}
}

func testJavaScriptRuntimeServiceRunningControlWrappers(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	if _, err := service.GetDispatch(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "disp-1"); err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if _, err := service.ListArtifacts(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if _, err := service.GetArtifact(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "art-1"); err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	listed, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(listed.LiveSessions) != 1 {
		t.Fatalf("live sessions = %#v, want one session", listed.LiveSessions)
	}

	if _, err := service.Pause(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, err := service.Resume(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := service.Terminate(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
}

func testJavaScriptRuntimeServiceApproveAwaitingSession(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	service.sessions["dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Status:           LifecycleStatusAwaitingApproval,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{},
			Links:            InspectionLinksForSession("dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SessionStatus: LifecycleStatusAwaitingApproval,
			ResultStatus:  ResultStatusNotReady,
		},
	}
	if _, err := service.Approve(context.Background(), "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ApproveRequest{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
}

func testJavaScriptRuntimeServiceRetryFailedDispatch(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	service.sessions["dur-sess-cccccccccccccccccccccccccccccccc"] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-cccccccccccccccccccccccccccccccc",
			Status:           LifecycleStatusFailed,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{},
			Links:            InspectionLinksForSession("dur-sess-cccccccccccccccccccccccccccccccc", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-cccccccccccccccccccccccccccccccc",
			SessionStatus: LifecycleStatusFailed,
			ResultStatus:  ResultStatusUnavailable,
		},
		dispatches: []DispatchSummary{
			{ID: "disp-retry", Status: DispatchStatusFailed, Attempt: 2},
		},
	}
	if _, err := service.RetryDispatch(context.Background(), "dur-sess-cccccccccccccccccccccccccccccccc", RetryDispatchRequest{DispatchID: "disp-retry"}); err != nil {
		t.Fatalf("RetryDispatch: %v", err)
	}
}

func TestProjectedLifecycleControlStatus_PrefersCanonicalBracketStatus(t *testing.T) {
	t.Parallel()
	status := ProjectedLifecycleControlStatus("PAUSED", "RUNNING")
	if status != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", status)
	}
}

func TestProjectedLifecycleControlStatus_FallsBackToFactoryRuntimeState(t *testing.T) {
	t.Parallel()
	if got := ProjectedLifecycleControlStatus("", "PAUSED"); got != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", got)
	}
	if got := ProjectedLifecycleControlStatus("", "RUNNING"); got != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", got)
	}
}

func TestFactoryStateToLifecycleStatus_MapsLiveFactoryStates(t *testing.T) {
	t.Parallel()

	for state, want := range map[string]LifecycleStatus{
		"IDLE": LifecycleStatusRunning, "RUNNING": LifecycleStatusRunning,
		"PAUSED": LifecycleStatusPaused, "COMPLETED": LifecycleStatusSucceeded,
		"FAILED": LifecycleStatusFailed,
	} {
		if got := LifecycleStatusFromFactoryRuntimeState(state); got != want {
			t.Fatalf("state %q = %q, want %q", state, got, want)
		}
	}
}

func TestLiveLifecycleControlResponse_BuildsTypedPauseOutcome(t *testing.T) {
	t.Parallel()

	result := LifecycleControlResult{
		SessionID: "~default", Operation: LifecycleControlPause,
		Outcome: LifecycleControlOutcomeAccepted, Status: LifecycleStatusPaused,
		Links: LiveLifecycleControlLinksForSession("~default"),
	}
	if result.SessionID != "~default" || result.Operation != LifecycleControlPause ||
		result.Outcome != LifecycleControlOutcomeAccepted || result.Status != LifecycleStatusPaused {
		t.Fatalf("result = %#v, want accepted live pause outcome", result)
	}
	if result.Links.Session != "/factory-sessions/~default" {
		t.Fatalf("links = %#v, want /factory-sessions/~default", result.Links)
	}
}

func TestJavaScriptRuntimeService_InterruptAcceptedBeforeChildCompletion_RecordsObservedCancellation(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-race-runtime-001"
	dispatchID := "dispatch-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop before provider completion"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}
	if interruptResult.DispatchID != dispatchID {
		t.Fatalf("dispatchId = %q, want %q", interruptResult.DispatchID, dispatchID)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "operator stop before provider completion" {
		t.Fatalf("failureDetail = %#v, want operator stop before provider completion", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, dispatchID)
	if payload.Reason != "operator stop before provider completion" {
		t.Fatalf("event reason = %q, want operator stop before provider completion", payload.Reason)
	}
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}

	replayed, err := ReplayDispatchProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 || replayed[0].Status != DispatchStatusInterrupted {
		t.Fatalf("replayed dispatches = %#v, want one interrupted dispatch", replayed)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime regression keeps late-result suppression assertions together on one scenario.
func TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-late-runtime-001"
	dispatchID := "dispatch-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	lateRecords := []factory.JavaScriptRuntimeRecord{{
		Kind: factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &factory.JavaScriptChildDispatchRecord{
			DispatchID:         dispatchID,
			Status:             factory.JavaScriptChildDispatchStatusCompleted,
			Label:              "summarize-findings",
			ArtifactRef:        factory.FormatArtifactURI(sessionID, "child-artifact-late"),
			ProviderSessionRef: "provider-session-late",
			Provider:           "mock",
		},
	}}
	if err := applyRuntimeTerminalOutcome(service, sessionID, factory.JavaScriptRuntimeOutcome{
		OK:      true,
		Records: lateRecords,
		Value:   factory.TypedValue{JSON: json.RawMessage(`{"label":"agent-run-fake-child"}`)},
	}); err != nil {
		t.Fatalf("ApplyRuntimeTerminalOutcomeForTests: %v", err)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch after late completion: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED after late completion", dispatch.Status)
	}
	if len(dispatch.OutputArtifactIDs) != 0 {
		t.Fatalf("outputArtifactIds = %#v, want suppressed late child output", dispatch.OutputArtifactIDs)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "provider-session-late" {
		t.Fatalf("providerSessionRefs = %#v, want late diagnostic preserved", dispatch.ProviderSessionRefs)
	}

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after late completion: %v", err)
	}
	if session.Status != LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED after late completion", session.Status)
	}
	if session.Progress != nil && session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after suppression", session.Progress.CompletedDispatches)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusUnavailable) {
		t.Fatalf("resultSummary = %#v, want UNAVAILABLE after late completion suppression", session.ResultSummary)
	}

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult after late completion: %v", err)
	}
	if result.SessionStatus != LifecycleStatusInterrupted || result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("result = status %q session %q, want UNAVAILABLE/INTERRUPTED", result.ResultStatus, result.SessionStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "SESSION_INTERRUPTED" {
		t.Fatalf("availability = %#v, want SESSION_INTERRUPTED", result.Availability)
	}

	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.DispatchID == dispatchID && artifact.Kind == "CHILD_RESULT" {
			t.Fatalf("artifact = %#v, want late child output suppressed", artifact)
		}
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if !containsEventType(events.Events, "DISPATCH_INTERRUPTED") {
		t.Fatal("DISPATCH_INTERRUPTED event missing after late completion merge")
	}
}

func TestJavaScriptRuntimeService_InterruptRunningDispatch_PreservesObservedCancellationAtRecordTime(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-observed-status-001"
	now := time.Date(2026, 6, 20, 18, 0, 0, 0, time.UTC)
	service.sessions[sessionID] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Phase:            "execute",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			Links:            InspectionLinksForSession(sessionID, true),
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{{
			ID:     "dispatch-1",
			Status: DispatchStatusRunning,
			Phase:  "execute",
			Label:  "summarize-findings",
		}},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"dispatch-1": {DispatchStatusQueued, DispatchStatusRunning},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning, OrchestratorKind: interfaces.OrchestratorKindJavaScript},
			ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning, ResultStatus: ResultStatusNotReady},
		),
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "cancellation observed while running"},
		DispatchID:     "dispatch-1",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "dispatch-1")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "cancellation observed while running" {
		t.Fatalf("reason = %q, want cancellation observed while running", payload.Reason)
	}
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines this restart integration test keeps the pre-restart and post-restart observable read assertions together.
// pkgmaintcheck:ignore-cyclomatic-complexity each assertion validates one durable partial-result field across the restart boundary.
func TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	sessionID := "dur-sess-paused-restart-001"
	dispatchID := "dispatch-completed-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "completed child"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	service.mu.Lock()
	state := service.sessions[sessionID]
	state.dispatches[0].Status = DispatchStatusCompleted
	state.dispatches[0].OutputArtifactIDs = []string{"artifact-1"}
	state.dispatchStatusTransitions[dispatchID] = []DispatchStatus{
		DispatchStatusQueued,
		DispatchStatusRunning,
		DispatchStatusCompleted,
	}
	state.artifacts = []ArtifactSummary{{
		ID:         "artifact-1",
		Kind:       "CHILD_RESULT",
		Visibility: "session",
		DispatchID: dispatchID,
	}}
	state.session.ArtifactCount = 1
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	service.mu.Unlock()

	paused, err := service.Pause(context.Background(), sessionID, ControlRequest{RequestID: "pause-restart-001"})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Status != LifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", paused.Status)
	}

	wantSession, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession before restart: %v", err)
	}
	wantResult, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("GetResult before restart: %v", err)
	}
	wantDispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before restart: %v", err)
	}
	wantEvents, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before restart: %v", err)
	}

	restarted := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	gotSession, err := restarted.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after restart: %v", err)
	}
	gotResult, err := restarted.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("GetResult after restart: %v", err)
	}
	gotDispatches, err := restarted.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after restart: %v", err)
	}
	gotEvents, err := restarted.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after restart: %v", err)
	}

	if !reflect.DeepEqual(gotSession, wantSession) {
		t.Fatalf("session changed across restart:\ngot  %#v\nwant %#v", gotSession, wantSession)
	}
	if !reflect.DeepEqual(gotResult, wantResult) || gotResult.ResultStatus != ResultStatusNotReady || gotResult.Availability == nil {
		t.Fatalf("result changed across restart: got %#v want %#v", gotResult, wantResult)
	}
	if !reflect.DeepEqual(gotDispatches, wantDispatches) {
		t.Fatalf("dispatches changed across restart: got %#v want %#v", gotDispatches, wantDispatches)
	}
	if len(gotEvents.Events) != len(wantEvents.Events) {
		t.Fatalf("event count changed across restart: got %d want %d", len(gotEvents.Events), len(wantEvents.Events))
	}
	for index := range wantEvents.Events {
		var gotEvent, wantEvent any
		if err := json.Unmarshal(gotEvents.Events[index], &gotEvent); err != nil {
			t.Fatalf("decode restarted event %d: %v", index, err)
		}
		if err := json.Unmarshal(wantEvents.Events[index], &wantEvent); err != nil {
			t.Fatalf("decode live event %d: %v", index, err)
		}
		if !reflect.DeepEqual(gotEvent, wantEvent) {
			t.Fatalf("event %d changed across restart: got %#v want %#v", index, gotEvent, wantEvent)
		}
	}
}

func TestJavaScriptRuntimeService_PausePersistenceFailureKeepsRunningProjection(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("pause persistence unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	sessionID := "dur-sess-pause-persist-failure-001"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, "dispatch-1", "running child"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}
	cancelCalls := 0
	service.mu.Lock()
	service.sessions[sessionID].runCancel = func() { cancelCalls++ }
	service.mu.Unlock()

	_, err := service.Pause(context.Background(), sessionID, ControlRequest{})
	if err == nil || !strings.Contains(err.Error(), "persist durable session snapshot") {
		t.Fatalf("Pause error = %v, want persistence failure", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil {
		t.Fatalf("GetSession: %v", readErr)
	}
	if read.Status != LifecycleStatusRunning || read.Lifecycle == nil || read.Lifecycle.PausedAt != nil {
		t.Fatalf("session after rejected pause = %#v, want unchanged RUNNING projection", read)
	}
	if _, err := service.Cancel(context.Background(), sessionID, ControlRequest{}); err != nil {
		t.Fatalf("Cancel after rejected pause: %v", err)
	}
	if cancelCalls != 1 {
		t.Fatalf("cancel calls after rejected pause = %d, want 1", cancelCalls)
	}
}

func TestInterruptedTerminalTimestamp_PrefersSessionLifecycle(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	interruptedAt := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)

	got := interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{InterruptedAt: &interruptedAt}},
		SessionReadResult{Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt}},
	)
	if got == nil || !got.Equal(interruptedAt) {
		t.Fatalf("timestamp = %v, want interruptedAt", got)
	}

	got = interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{FinishedAt: &finishedAt}},
		SessionReadResult{},
	)
	if got == nil || !got.Equal(finishedAt) {
		t.Fatalf("timestamp = %v, want finishedAt", got)
	}

	got = interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{UpdatedAt: &updatedAt}},
		SessionReadResult{},
	)
	if got == nil || !got.Equal(updatedAt) {
		t.Fatalf("timestamp = %v, want updatedAt", got)
	}

	got = interruptedTerminalTimestamp(SessionReadResult{}, SessionReadResult{
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
	})
	if got == nil || !got.Equal(startedAt) {
		t.Fatalf("timestamp = %v, want prior startedAt", got)
	}
}

func TestLegacyInterruptedSnapshot_RemainsLosslessOnSuccessfulSave(t *testing.T) {
	const sessionID = "dur-sess-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	store := &petriCompactionStore{}
	state := runtimeSessionState{session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusInterrupted, OrchestratorKind: interfaces.OrchestratorKindPetri}, petriMutations: legacyTerminalTokenMutations(8)}
	if err := (&JavaScriptRuntimeService{persistence: store}).persistSessionSnapshot(state); err != nil {
		t.Fatalf("persist interrupted legacy snapshot: %v", err)
	}
	var persisted PersistedRuntimeSessionState
	if err := json.Unmarshal(store.payload, &persisted); err != nil {
		t.Fatalf("decode interrupted snapshot: %v", err)
	}
	var mutationCount int
	for _, record := range persisted.Records {
		if record.Kind == DurableRecordKindPetriTokenMutation {
			mutationCount++
		}
	}
	if mutationCount != len(state.petriMutations) {
		t.Fatalf("interrupted persisted mutation count = %d, want %d", mutationCount, len(state.petriMutations))
	}
}

func encodedWarningStateBytes(t *testing.T, state runtimeSessionState) int {
	t.Helper()
	encoded, err := json.MarshalIndent(persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, 0), "", "  ")
	if err != nil {
		t.Fatalf("marshal warning state: %v", err)
	}
	return len(encoded)
}
