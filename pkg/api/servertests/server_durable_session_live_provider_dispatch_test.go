package apiserver_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsQueuedRunningCompletedPath(t *testing.T) {
	service := newAPILiveProviderRuntimeService(t)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-live-provider-dispatch-sync-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &factorysessionexecution.RuntimeOptions{
			ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	sessionRead := getDurableFactorySession(t, server.URL, completed.SessionID)
	if sessionRead.Progress == nil ||
		sessionRead.Progress.TotalDispatches == nil || *sessionRead.Progress.TotalDispatches != 1 ||
		sessionRead.Progress.CompletedDispatches == nil || *sessionRead.Progress.CompletedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one completed dispatch", sessionRead.Progress)
	}

	dispatchList := getDurableDispatchList(t, server.URL, completed.SessionID)
	if len(dispatchList.Dispatches) != 1 {
		t.Fatalf("dispatch list = %#v, want one dispatch", dispatchList.Dispatches)
	}
	dispatchSummary := dispatchList.Dispatches[0]
	if dispatchSummary.Id != "dispatch-1" {
		t.Fatalf("dispatch id = %q, want dispatch-1", dispatchSummary.Id)
	}
	if dispatchSummary.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatchSummary.Status)
	}
	if dispatchSummary.Provider == nil || *dispatchSummary.Provider != "mock" {
		t.Fatalf("dispatch provider = %#v, want mock", dispatchSummary.Provider)
	}
	if dispatchSummary.Javascript == nil || dispatchSummary.Javascript.ExecutionMode == nil ||
		*dispatchSummary.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch executionMode = %#v, want live-provider", dispatchSummary.Javascript)
	}
	if dispatchSummary.ProviderSessionRefs == nil || len(*dispatchSummary.ProviderSessionRefs) != 1 ||
		(*dispatchSummary.ProviderSessionRefs)[0].Id != "live-provider-session-1" {
		t.Fatalf("providerSessionRefs = %#v", dispatchSummary.ProviderSessionRefs)
	}
	if dispatchSummary.OutputArtifactIds == nil || len(*dispatchSummary.OutputArtifactIds) != 1 ||
		(*dispatchSummary.OutputArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("outputArtifactIds = %#v, want [child-artifact-1]", dispatchSummary.OutputArtifactIds)
	}

	dispatchDetail := getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
	if dispatchDetail.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch detail status = %q, want COMPLETED", dispatchDetail.Status)
	}
	if dispatchDetail.Javascript == nil || dispatchDetail.Javascript.ExecutionMode == nil ||
		*dispatchDetail.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch detail executionMode = %#v, want live-provider", dispatchDetail.Javascript)
	}
	assertAPIDispatchStatusTransitions(t, dispatchDetail.StatusTransitions, []factoryapi.FactoryDispatchStatus{
		factoryapi.FactoryDispatchStatusQUEUED,
		factoryapi.FactoryDispatchStatusRUNNING,
		factoryapi.FactoryDispatchStatusCOMPLETED,
	})
	if dispatchDetail.ArtifactIds == nil || len(*dispatchDetail.ArtifactIds) != 1 ||
		(*dispatchDetail.ArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("dispatch artifactIds = %#v, want [child-artifact-1]", dispatchDetail.ArtifactIds)
	}
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsRunningDispatchBeforeCompletion(t *testing.T) {
	service, provider := newAPILiveProviderBlockingRuntimeService(t)
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-live-provider-dispatch-async-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &factorysessionexecution.RuntimeOptions{
			ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	provider.waitForInferStart(t)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	runningDispatch := waitForAPIDispatchStatus(
		t,
		server.URL,
		started.SessionID,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusRUNNING,
		2*time.Second,
	)
	if runningDispatch.Javascript == nil || runningDispatch.Javascript.ExecutionMode == nil ||
		*runningDispatch.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("running dispatch executionMode = %#v, want live-provider", runningDispatch.Javascript)
	}

	sessionRead := getDurableFactorySession(t, server.URL, started.SessionID)
	if sessionRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session status while child running = %q, want RUNNING", sessionRead.Status)
	}
	if sessionRead.Progress == nil || sessionRead.Progress.TotalDispatches == nil ||
		*sessionRead.Progress.TotalDispatches != 1 {
		t.Fatalf("session progress while child running = %#v, want one total dispatch", sessionRead.Progress)
	}

	provider.releaseInfer()
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	completedDetail := getDurableDispatchDetail(t, server.URL, started.SessionID, "dispatch-1")
	if completedDetail.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch detail after completion = %q, want COMPLETED", completedDetail.Status)
	}
	assertAPIDispatchStatusTransitions(t, completedDetail.StatusTransitions, []factoryapi.FactoryDispatchStatus{
		factoryapi.FactoryDispatchStatusQUEUED,
		factoryapi.FactoryDispatchStatusRUNNING,
		factoryapi.FactoryDispatchStatusCOMPLETED,
	})
}

func newAPILiveProviderRuntimeService(t *testing.T) factorysessionexecution.Service {
	t.Helper()
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          factorysessionexecution.SmokeLiveChildProvider(),
	})
}

func newAPILiveProviderBlockingRuntimeService(t *testing.T) (
	*factorysessionexecution.JavaScriptRuntimeService,
	*apiLiveProviderBlockingFixtureProvider,
) {
	t.Helper()
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := &apiLiveProviderBlockingFixtureProvider{}
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          provider,
	})
	return service, provider
}

type apiLiveProviderBlockingFixtureProvider struct {
	mu           sync.Mutex
	inferStarted chan struct{}
	release      chan struct{}
}

func (p *apiLiveProviderBlockingFixtureProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	if p.inferStarted == nil {
		p.inferStarted = make(chan struct{})
	}
	if p.release == nil {
		p.release = make(chan struct{})
	}
	started := p.inferStarted
	release := p.release
	p.mu.Unlock()

	close(started)
	select {
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	case <-release:
		return interfaces.InferenceResponse{
			Content: `{"text":"live:agent-run-fake-child:summarize-findings:summarize workflows:workflows"}`,
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}
}

func (p *apiLiveProviderBlockingFixtureProvider) waitForInferStart(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	if p.inferStarted == nil {
		p.inferStarted = make(chan struct{})
	}
	started := p.inferStarted
	p.mu.Unlock()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider Infer did not start before timeout")
	}
}

func (p *apiLiveProviderBlockingFixtureProvider) releaseInfer() {
	p.mu.Lock()
	if p.release == nil {
		p.release = make(chan struct{})
	}
	release := p.release
	p.mu.Unlock()
	close(release)
}

func waitForAPIDispatchStatus(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dispatchList := getDurableDispatchList(t, serverURL, sessionID)
		for _, dispatch := range dispatchList.Dispatches {
			if dispatch.Id == dispatchID && dispatch.Status == want {
				return dispatch
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("dispatch %q did not reach status %q before timeout", dispatchID, want)
	return factoryapi.FactorySessionDispatchSummary{}
}

func assertAPIDispatchStatusTransitions(
	t *testing.T,
	got *[]factoryapi.FactoryDispatchStatus,
	want []factoryapi.FactoryDispatchStatus,
) {
	t.Helper()
	if got == nil {
		t.Fatalf("statusTransitions = nil, want %#v", want)
	}
	if len(*got) != len(want) {
		t.Fatalf("statusTransitions = %#v, want %#v", *got, want)
	}
	for index, status := range *got {
		if status != want[index] {
			t.Fatalf("statusTransitions[%d] = %q, want %q", index, status, want[index])
		}
	}
}
