package runtime_api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDashboard_EngineStateSnapshot_EndToEnd(t *testing.T) {
	support.SkipLongFunctional(t, "slow dashboard engine-state sweep")
	dir := scaffoldDashboardWorldViewFunctionalDir(t)
	provider := newFunctionalWorldViewProvider()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})

	submitDashboardWorldViewFunctionalWork(t, server.URL(), "world-view-success", "trace-world-view-success")
	provider.nextDispatch(t)
	if got := support.GetDefaultSession(t, server.URL()).Runtime.Progress.InFlightCount; got != 1 {
		t.Fatalf("in-flight dispatch count = %d, want 1", got)
	}
	provider.respond(workerexecution.InferenceResponse{
		Content: "COMPLETE",
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-world-view-success",
		},
	}, nil)
	waitForPublicWorkInPlace(t, server.URL(), "task:complete", "world-view-success", time.Second)

	submitDashboardWorldViewFunctionalWork(t, server.URL(), "world-view-failed", "trace-world-view-failed")
	provider.nextDispatch(t)
	provider.respond(workerexecution.InferenceResponse{}, &workerexecution.ProviderError{
		Family:  workerexecution.WorkFailureFamilyTerminal,
		Type:    workerexecution.WorkFailureTypePermanentBadRequest,
		Message: "provider rejected dashboard world-view work",
		Cause:   errors.New("provider rejected"),
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-world-view-failed",
		},
	})
	waitForPublicWorkInPlace(t, server.URL(), "task:failed", "world-view-failed", time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("task:complete token count = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("task:failed token count = %d, want 1", got)
	}
	assertFunctionalProviderSessionsInEvents(t, server.GetFactoryEvents(t))
	server.Stop(t)
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

func submitDashboardWorldViewFunctionalWork(t *testing.T, baseURL string, workID string, traceID string) {
	t.Helper()
	workType := "task"
	works := []factoryapi.Work{{
		Name:         workID,
		WorkId:       &workID,
		WorkTypeName: &workType,
		TraceId:      &traceID,
		Payload:      map[string]any{"item": "dashboard-world-view-functional"},
	}}
	support.UpsertDefaultSessionWorkRequest(t, baseURL, factoryapi.WorkRequest{
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

func waitForPublicWorkInPlace(t *testing.T, baseURL, placeID, workID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		if support.HasWorkAtCustomerState(listed, workID, placeID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work %q at public location %q", workID, placeID)
}

type functionalWorldViewProvider struct {
	requests  chan workerexecution.ProviderInferenceRequest
	responses chan functionalWorldViewProviderResponse
}

type functionalWorldViewProviderResponse struct {
	response workerexecution.InferenceResponse
	err      error
}

func newFunctionalWorldViewProvider() *functionalWorldViewProvider {
	return &functionalWorldViewProvider{
		requests:  make(chan workerexecution.ProviderInferenceRequest, 2),
		responses: make(chan functionalWorldViewProviderResponse, 2),
	}
}

func (p *functionalWorldViewProvider) Infer(ctx context.Context, request workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	select {
	case p.requests <- request:
	case <-ctx.Done():
		return workerexecution.InferenceResponse{}, ctx.Err()
	}
	select {
	case response := <-p.responses:
		return response.response, response.err
	case <-ctx.Done():
		return workerexecution.InferenceResponse{}, ctx.Err()
	}
}

func (p *functionalWorldViewProvider) nextDispatch(t *testing.T) workerexecution.ProviderInferenceRequest {
	t.Helper()
	select {
	case request := <-p.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider dispatch")
		return workerexecution.ProviderInferenceRequest{}
	}
}

func (p *functionalWorldViewProvider) respond(response workerexecution.InferenceResponse, err error) {
	p.responses <- functionalWorldViewProviderResponse{response: response, err: err}
}

var _ workerprovider.Provider = (*functionalWorldViewProvider)(nil)
