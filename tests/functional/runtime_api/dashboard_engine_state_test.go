package runtime_api

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDashboard_EngineStateSnapshot_EndToEnd(t *testing.T) {
	support.SkipLongFunctional(t, "slow dashboard engine-state sweep")
	dir := scaffoldDashboardWorldViewFunctionalDir(t)
	provider := newFunctionalWorldViewProvider()
	server := startSharedFunctionalServer(t, dir, runtimeAPIScenario{
		provider: provider,
		models:   []string{"gpt-5-codex"},
	})

	submitDashboardWorldViewFunctionalWork(t, server, "world-view-success", "trace-world-view-success")
	provider.nextDispatch(t)
	if got := server.Session(t).Runtime.Progress.InFlightCount; got != 1 {
		t.Fatalf("in-flight dispatch count = %d, want 1", got)
	}
	provider.respond(providers.ExecuteResult{
		Content: "COMPLETE",
		SessionRef: &providers.SessionRef{
			Provider: "codex",
			Kind:     providers.SessionIDKind,
			ID:       "sess-world-view-success",
		},
	}, nil)
	waitForPublicWorkInPlace(t, server, "task:complete", "world-view-success", time.Second)

	submitDashboardWorldViewFunctionalWork(t, server, "world-view-failed", "trace-world-view-failed")
	provider.nextDispatch(t)
	provider.respond(providers.ExecuteResult{}, providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindInvalidRequest,
		Message: "provider rejected dashboard world-view work",
		SessionRef: &providers.SessionRef{
			Provider: "codex",
			Kind:     providers.SessionIDKind,
			ID:       "sess-world-view-failed",
		},
	})
	waitForPublicWorkInPlace(t, server, "task:failed", "world-view-failed", time.Second)

	listed := server.ListWork(t)
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("task:complete token count = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("task:failed token count = %d, want 1", got)
	}
	assertFunctionalProviderSessionsInEvents(t, server.GetFactoryEvents(t))
}

func scaffoldDashboardWorldViewFunctionalDir(t *testing.T) string {
	t.Helper()
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: "worker-a",
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	})
	writeDashboardWorldViewAgents(t, filepath.Join(dir, "workers", "worker-a"), "MODEL_WORKER")
	writeDashboardWorldViewAgents(t, filepath.Join(dir, "workstations", "process"), "MODEL_WORKSTATION")
	return dir
}

func writeDashboardWorldViewAgents(t *testing.T, dir string, agentType string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	body := "---\ntype: " + agentType + "\n"
	if agentType == "MODEL_WORKER" {
		body += "model: gpt-5-codex\nmodelProvider: codex\nstopToken: COMPLETE\n"
	}
	body += "---\nProcess the dashboard world-view work.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write AGENTS.md in %s: %v", dir, err)
	}
}

func submitDashboardWorldViewFunctionalWork(t *testing.T, server *functionalAPIServer, workID string, traceID string) {
	t.Helper()
	workType := "task"
	works := []factoryapi.Work{{
		Name:         workID,
		WorkId:       &workID,
		WorkTypeName: &workType,
		TraceId:      &traceID,
		Payload:      map[string]any{"item": "dashboard-world-view-functional"},
	}}
	putGeneratedWorkRequestAt(t, server.workURL("/work-requests/"+url.PathEscape("request-"+workID)), factoryapi.WorkRequest{
		RequestId: "request-" + workID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &works,
	})
}

func assertFunctionalProviderSessionsInEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil || payload.ProviderSession == nil {
			continue
		}
		seen[support.StringPointerValue(payload.ProviderSession.Id)] = true
	}
	for _, want := range []string{"sess-world-view-success", "sess-world-view-failed"} {
		if !seen[want] {
			t.Fatalf("provider sessions in events = %#v, missing %q", seen, want)
		}
	}
}

func waitForPublicWorkInPlace(t *testing.T, server *functionalAPIServer, placeID, workID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := server.ListWork(t)
		if support.HasWorkAtCustomerState(listed, workID, placeID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work %q at public location %q", workID, placeID)
}

// C06-SHARED EXCEPTION CASE-dashboard-engine-state: this scenario asserts the
// provider-session IDs emitted in canonical Factory Events for both a success
// and a failure. The ProviderCommandRunner edge exposes command request/output
// data but has no contract for injecting provider SessionRef metadata, so a
// narrowly scoped Providers service remains necessary for this public event
// witness. All other shareable provider controls in this lane use command
// runners.
type functionalWorldViewProvider struct {
	testutil.NativeProvider
	requests  chan providers.ExecuteRequest
	responses chan functionalWorldViewProviderResponse
}

type functionalWorldViewProviderResponse struct {
	response providers.ExecuteResult
	err      error
}

func newFunctionalWorldViewProvider() *functionalWorldViewProvider {
	provider := &functionalWorldViewProvider{
		requests:  make(chan providers.ExecuteRequest, 2),
		responses: make(chan functionalWorldViewProviderResponse, 2),
	}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (p *functionalWorldViewProvider) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	select {
	case p.requests <- request:
	case <-ctx.Done():
		return providers.ExecuteResult{}, ctx.Err()
	}
	select {
	case response := <-p.responses:
		return response.response, response.err
	case <-ctx.Done():
		return providers.ExecuteResult{}, ctx.Err()
	}
}

func (p *functionalWorldViewProvider) nextDispatch(t *testing.T) providers.ExecuteRequest {
	t.Helper()
	select {
	case request := <-p.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider dispatch")
		return providers.ExecuteRequest{}
	}
}

func (p *functionalWorldViewProvider) respond(response providers.ExecuteResult, err error) {
	if response.Content != "" && response.Diagnostics == nil {
		response.Diagnostics = &providers.ExecuteDiagnostics{Metadata: map[string]string{
			"completion_evidence": "provider_response",
		}}
	}
	p.responses <- functionalWorldViewProviderResponse{response: response, err: err}
}
