package fixtures_test

import (
	"context"
	"sync"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_ResumeInterruptedSession_ReconstructsFromCheckpointSummary(t *testing.T) {
	provider := newSequentialBlockingProvider()
	projectRoot := setupRuntimeWorkflowFixture(
		t,
		"resumable-two-step-fake-children.workflow.js",
		"resumable-two-step-fake-children",
	)
	initial := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		PersistSessions:   true,
	})

	started, err := initial.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-resume-interrupted-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-1", fse.DispatchStatusCompleted, 3*time.Second)
	dispatchTwo := waitForDispatchStatus(t, initial, started.SessionID, "dispatch-2", fse.DispatchStatusRunning, 3*time.Second)
	if dispatchTwo.Status != fse.DispatchStatusRunning {
		t.Fatalf("dispatch-2 = %#v, want RUNNING", dispatchTwo)
	}
	if provider.CallCount() < 2 {
		t.Fatalf("provider infer calls = %d, want at least 2 before interrupt", provider.CallCount())
	}

	interruptResult, err := initial.InterruptDispatch(context.Background(), started.SessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "process restart simulation"},
		DispatchID:     "dispatch-2",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != fse.LifecycleControlOutcomeAccepted {
		t.Fatalf("interrupt outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}

	provider.waitForCanceledInfer(t, 3*time.Second)
	interrupted := waitUntilSessionStatus(t, initial, started.SessionID, fse.LifecycleStatusInterrupted, 5*time.Second)
	if interrupted.Status != fse.LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED", interrupted.Status)
	}

	firstDispatchBeforeResume, err := initial.GetDispatch(context.Background(), started.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch dispatch-1 before resume: %v", err)
	}
	if firstDispatchBeforeResume.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch-1 before resume = %#v, want COMPLETED", firstDispatchBeforeResume)
	}

	resumedService := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		PersistSessions:   true,
	})
	resumed, err := resumedService.ResumeInterruptedSession(context.Background(), started.SessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-interrupted-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.SessionID != started.SessionID {
		t.Fatalf("resumed sessionId = %q, want %q", resumed.SessionID, started.SessionID)
	}
	if resumed.Status != string(fse.LifecycleStatusResuming) {
		t.Fatalf("resumed start status = %q, want RESUMING", resumed.Status)
	}

	success := waitUntilSessionStatus(t, resumedService, started.SessionID, fse.LifecycleStatusSucceeded, 5*time.Second)
	if success.ResultSummary == nil || success.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resumed resultSummary = %#v, want FINAL", success.ResultSummary)
	}

	if provider.CallCount() != 3 {
		t.Fatalf("provider infer calls = %d, want 3 (step-one, blocked step-two, resumed step-two without rerunning step-one)", provider.CallCount())
	}

	firstDispatchAfterResume, err := resumedService.GetDispatch(context.Background(), started.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch dispatch-1 after resume: %v", err)
	}
	if firstDispatchAfterResume.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch-1 after resume = %#v, want COMPLETED", firstDispatchAfterResume)
	}
	if firstDispatchAfterResume.ID != firstDispatchBeforeResume.ID {
		t.Fatalf("dispatch-1 id changed across resume: %q -> %q", firstDispatchBeforeResume.ID, firstDispatchAfterResume.ID)
	}

	secondDispatchAfterResume, err := resumedService.GetDispatch(context.Background(), started.SessionID, "dispatch-2")
	if err != nil {
		t.Fatalf("GetDispatch dispatch-2 after resume: %v", err)
	}
	if secondDispatchAfterResume.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch-2 after resume = %#v, want COMPLETED", secondDispatchAfterResume)
	}

	result, err := resumedService.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", result.ResultStatus)
	}
}

type sequentialBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
}

func newSequentialBlockingProvider() *sequentialBlockingProvider {
	return &sequentialBlockingProvider{}
}

func (p *sequentialBlockingProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		return interfaces.InferenceResponse{
			Content: `{"text":"live:resumable-two-step-fake-children:step-one:step-one:workflows","label":"step-one"}`,
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return interfaces.InferenceResponse{}, ctx.Err()
	}

	return interfaces.InferenceResponse{
		Content: `{"text":"live:resumable-two-step-fake-children:step-two:step-two:workflows","label":"step-two"}`,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}, nil
}

func (p *sequentialBlockingProvider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *sequentialBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled
		p.mu.Unlock()
		if canceled > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe canceled workflow context")
}

func waitForDispatchStatus(
	t *testing.T,
	service fse.Service,
	sessionID string,
	dispatchID string,
	want fse.DispatchStatus,
	timeout time.Duration,
) fse.DispatchSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed, err := service.ListDispatches(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListDispatches: %v", err)
		}
		for _, dispatch := range listed.Dispatches {
			if dispatch.ID != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return dispatch
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dispatch %q did not reach status %q within %s", dispatchID, want, timeout)
	return fse.DispatchSummary{}
}
