package factorysessionexecution

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

	completeCalls := 0
	var completed workers.ExecuteResult
	var completeErr error
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

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:           "wait for Antigravity",
		ExecutorProvider: "SCRIPT_WRAP",
		ModelProvider:    "antigravity",
		Model:            "gemini-3.6-flash-medium",
		ResourceID:       "reviewers",
	})
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
	if completeCalls != 1 || completed.Failure == nil || completed.Failure.Type != workers.WorkFailureTypeTimeout || completeErr == nil {
		t.Fatalf("completed attempt = calls:%d result:%#v err:%v, want one typed timeout", completeCalls, completed, completeErr)
	}
	if workerReleases != 1 {
		t.Fatalf("worker dispatch releases = %d, want exactly one", workerReleases)
	}

	select {
	case <-capacity:
		capacity <- struct{}{}
	default:
		t.Fatal("child capacity was not released for a subsequent dispatch")
	}

	progressMu.Lock()
	deferredProgress := append([]workers.ProgressFragment(nil), progress...)
	progressMu.Unlock()
	if len(deferredProgress) != 1 || deferredProgress[0].Kind != workers.FailedFragmentKind ||
		deferredProgress[0].Metadata["work_failure_type"] != string(workers.WorkFailureTypeTimeout) {
		t.Fatalf("terminal progress = %#v, want exactly one typed timeout", deferredProgress)
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
