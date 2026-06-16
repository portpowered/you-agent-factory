package fixtures_test

import (
	"context"
	"sync"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_AgentRunLiveChild_TimeoutInterruptsProviderInfer(t *testing.T) {
	provider := newBlockingFixtureProvider()
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child-timeout",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		RequestedPolicy: map[string]any{
			"maxRunDurationMs": 50,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != fse.LifecycleStatusTimedOut {
		t.Fatalf("session status = %q, want TIMED_OUT", read.Status)
	}
	if read.Failure == nil || read.Failure.Reason != "WORKFLOW_RUNTIME_TIMEOUT" {
		t.Fatalf("session failure = %#v, want WORKFLOW_RUNTIME_TIMEOUT", read.Failure)
	}
	if provider.inferContextsHonored() == 0 {
		t.Fatal("provider Infer did not observe timed-out workflow context")
	}
}

type blockingFixtureProvider struct {
	mu              sync.Mutex
	inferStarted    chan struct{}
	inferStartedSet bool
	contextCanceled int
}

func newBlockingFixtureProvider() *blockingFixtureProvider {
	return &blockingFixtureProvider{
		inferStarted: make(chan struct{}),
	}
}

func (m *blockingFixtureProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.mu.Lock()
	if !m.inferStartedSet {
		m.inferStartedSet = true
		close(m.inferStarted)
	}
	m.mu.Unlock()

	<-ctx.Done()
	m.mu.Lock()
	m.contextCanceled++
	m.mu.Unlock()
	return interfaces.InferenceResponse{}, ctx.Err()
}

func (m *blockingFixtureProvider) inferContextsHonored() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.contextCanceled
}
