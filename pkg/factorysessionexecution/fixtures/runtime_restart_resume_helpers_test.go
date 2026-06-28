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

type interruptedResumableHarness struct {
	projectRoot string
	provider    *sequentialBlockingProvider
	initial     fse.Service
	sessionID   string
	interrupted fse.SessionReadResult
}

func startInterruptedResumableSession(t *testing.T, requestID string) interruptedResumableHarness {
	t.Helper()

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
		RequestID: requestID,
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
	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-2", fse.DispatchStatusRunning, 3*time.Second)

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

	return interruptedResumableHarness{
		projectRoot: projectRoot,
		provider:    provider,
		initial:     initial,
		sessionID:   started.SessionID,
		interrupted: interrupted,
	}
}

func newResumedRuntimeService(harness interruptedResumableHarness) *fse.JavaScriptRuntimeService {
	return fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       harness.projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          harness.provider,
		PersistSessions:   true,
	})
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
