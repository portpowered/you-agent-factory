package runtime_api_test

import (
	"context"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceModeSmoke_EmptyStartupIdleSubmissionAndPostCompletionIdleStayReachableUntilCanceled(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-mode lifecycle smoke")
	server, dispatchRelease := newServiceModeObservabilityServer(t)

	initial := waitForPublicFactorySession(t, server, 5*time.Second, serviceModeSessionIdle)
	if initial.Runtime.Progress.TotalTokens != 0 {
		t.Fatalf("initial total tokens = %d, want 0", initial.Runtime.Progress.TotalTokens)
	}

	traceID := submitServiceModeSmokeWork(t, server)
	waitForPublicFactorySession(t, server, 10*time.Second, serviceModeSessionActive)
	activeWorkID := requirePublicWorkForTrace(t, server, traceID)
	assertServiceModeHasPendingDispatch(t, server, activeWorkID)

	close(dispatchRelease)
	completed := waitForPublicFactorySession(t, server, 10*time.Second, func(session factoryapi.FactorySession) bool {
		listed := server.ListWork(t)
		return serviceModeSessionIdle(session) &&
			support.HasWorkAtCustomerState(listed, activeWorkID, "task:complete")
	})
	if completed.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf("completed terminal count = %d, want 1", completed.Runtime.Progress.Categories.Terminal)
	}
	assertCompletedServiceModeWork(t, server, traceID, activeWorkID)
	assertServiceModeHasCompletedDispatch(t, server, activeWorkID)
	assertServiceModeServerStillRunning(t, server, "service-mode runtime exited after returning to idle; expected it to stay alive until cancellation")

	server.Stop(t)
	assertServiceModeServerStops(t, server)
}

func TestObservabilitySmoke_PublicStatusSessionWorkAndEventsAlignAcrossRuntimeTransitions(t *testing.T) {
	support.SkipLongFunctional(t, "slow observability smoke")
	server, dispatchRelease := newServiceModeObservabilityServer(t)

	idle := waitForPublicFactorySession(t, server, 5*time.Second, serviceModeSessionIdle)
	assertPublicStatusMatchesSession(t, server, idle)

	traceID := submitServiceModeSmokeWork(t, server)
	active := waitForPublicFactorySession(t, server, 10*time.Second, serviceModeSessionActive)
	activeWorkID := requirePublicWorkForTrace(t, server, traceID)
	assertPublicStatusMatchesSession(t, server, active)
	assertServiceModeHasPendingDispatch(t, server, activeWorkID)

	close(dispatchRelease)
	completed := waitForPublicFactorySession(t, server, 10*time.Second, func(session factoryapi.FactorySession) bool {
		listed := server.ListWork(t)
		return serviceModeSessionIdle(session) &&
			support.HasWorkAtCustomerState(listed, activeWorkID, "task:complete")
	})
	assertPublicStatusMatchesSession(t, server, completed)
	assertCompletedServiceModeWork(t, server, traceID, activeWorkID)
	assertServiceModeHasCompletedDispatch(t, server, activeWorkID)
	assertServiceModeServerStillRunning(t, server, "service-mode runtime exited after returning to idle")

	server.Stop(t)
	assertServiceModeServerStops(t, server)
}

func newServiceModeObservabilityServer(t *testing.T) (*FunctionalServer, chan struct{}) {
	t.Helper()

	dir := support.ScaffoldFactory(t, twoStagePipelineConfig())
	dispatchRelease := make(chan struct{})
	provider := &serviceModeBlockingProvider{release: dispatchRelease}
	return StartFunctionalServer(t, dir, false, withProvider(provider)), dispatchRelease
}

func serviceModeSessionIdle(session factoryapi.FactorySession) bool {
	return session.Runtime.Progress.FactoryState == "RUNNING" &&
		string(session.Runtime.Status) == string(interfaces.RuntimeStatusIdle) &&
		session.Runtime.Progress.InFlightCount == 0
}

func serviceModeSessionActive(session factoryapi.FactorySession) bool {
	return session.Runtime.Progress.FactoryState == "RUNNING" &&
		string(session.Runtime.Status) == string(interfaces.RuntimeStatusActive) &&
		session.Runtime.Progress.InFlightCount > 0
}

func submitServiceModeSmokeWork(t *testing.T, server *FunctionalServer) string {
	t.Helper()

	traceID := server.SubmitWork(t, "task", []byte(`{"title":"service-mode smoke item"}`))
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}
	return traceID
}

func requirePublicWorkForTrace(t *testing.T, server *FunctionalServer, traceID string) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, item := range server.ListWork(t).Results {
			if support.StringPointerValue(item.TraceId) == traceID {
				workID := support.StringPointerValue(item.WorkId)
				if workID == "" {
					t.Fatalf("public Work for trace %q has empty work ID: %#v", traceID, item)
				}
				return workID
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("public Work listing never exposed trace %q", traceID)
	return ""
}

func assertCompletedServiceModeWork(t *testing.T, server *FunctionalServer, traceID, workID string) {
	t.Helper()

	for _, item := range server.ListWork(t).Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		if support.StringPointerValue(item.TraceId) != traceID {
			t.Fatalf("completed Work trace ID = %q, want %q", support.StringPointerValue(item.TraceId), traceID)
		}
		if item.State == nil || item.State.Name != "complete" || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("completed Work state = %#v, want complete/TERMINAL", item.State)
		}
		return
	}
	t.Fatalf("public Work listing missing completed work %q", workID)
}

func assertPublicStatusMatchesSession(t *testing.T, server *FunctionalServer, session factoryapi.FactorySession) {
	t.Helper()

	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.FactoryState != session.Runtime.Progress.FactoryState {
		t.Fatalf("GET /status factoryState = %q, Factory Session = %q", status.FactoryState, session.Runtime.Progress.FactoryState)
	}
	if status.RuntimeStatus != string(session.Runtime.Status) {
		t.Fatalf("GET /status runtimeStatus = %q, Factory Session = %q", status.RuntimeStatus, session.Runtime.Status)
	}
	if status.TotalTokens != session.Runtime.Progress.TotalTokens {
		t.Fatalf("GET /status totalTokens = %d, Factory Session = %d", status.TotalTokens, session.Runtime.Progress.TotalTokens)
	}
	if status.Categories.Terminal != session.Runtime.Progress.Categories.Terminal ||
		status.Categories.Processing != session.Runtime.Progress.Categories.Processing {
		t.Fatalf("GET /status categories = %#v, Factory Session = %#v", status.Categories, session.Runtime.Progress.Categories)
	}
}

func assertServiceModeHasPendingDispatch(t *testing.T, server *FunctionalServer, workID string) {
	t.Helper()

	for _, dispatch := range support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)) {
		if support.DispatchObservationIncludesWork(dispatch, workID) && dispatch.Response == nil {
			return
		}
	}
	t.Fatalf("public Factory Events contain no pending dispatch for work %q", workID)
}

func assertServiceModeHasCompletedDispatch(t *testing.T, server *FunctionalServer, workID string) {
	t.Helper()

	for _, dispatch := range support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)) {
		if support.DispatchObservationIncludesWork(dispatch, workID) && dispatch.Response != nil {
			return
		}
	}
	t.Fatalf("public Factory Events contain no completed dispatch for work %q", workID)
}

func waitForPublicFactorySession(
	t *testing.T,
	server *FunctionalServer,
	timeout time.Duration,
	match func(factoryapi.FactorySession) bool,
) factoryapi.FactorySession {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := support.GetDefaultSession(t, server.URL())
		if match(session) {
			return session
		}
		time.Sleep(50 * time.Millisecond)
	}
	session := support.GetDefaultSession(t, server.URL())
	t.Fatalf("timed out waiting for public Factory Session within %s: %#v", timeout, session.Runtime)
	return session
}

func assertServiceModeServerStillRunning(t *testing.T, server *FunctionalServer, failureMessage string) {
	t.Helper()

	select {
	case <-server.Done():
		t.Fatal(failureMessage)
	case <-time.After(500 * time.Millisecond):
	}
}

func assertServiceModeServerStops(t *testing.T, server *FunctionalServer) {
	t.Helper()

	select {
	case <-server.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("service-mode runtime did not exit after explicit cancellation")
	}
}

type serviceModeBlockingProvider struct {
	release <-chan struct{}
	mu      sync.Mutex
	calls   int
}

func (p *serviceModeBlockingProvider) Execute(
	ctx context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		select {
		case <-p.release:
		case <-ctx.Done():
			return workerexecution.InferenceResponse{}, ctx.Err()
		}
	}
	return workerexecution.InferenceResponse{Content: "completed"}, nil
}
