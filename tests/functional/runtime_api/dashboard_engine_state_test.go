package runtime_api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRuntime_EngineStateSnapshot_EndToEnd(t *testing.T) {
	support.SkipLongFunctional(t, "slow dashboard engine-state sweep")
	dir := scaffoldDashboardWorldViewFunctionalDir(t)
	provider := newFunctionalWorldViewProvider()
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithServiceMode()),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	submitDashboardWorldViewFunctionalWork(t, h, "world-view-success", "trace-world-view-success")
	provider.nextDispatch(t)
	provider.respond(interfaces.InferenceResponse{
		Content: "COMPLETE",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-world-view-success",
		},
	}, nil)
	waitForHarnessWorkInPlace(t, h, "task:complete", "world-view-success", time.Second)

	submitDashboardWorldViewFunctionalWork(t, h, "world-view-failed", "trace-world-view-failed")
	provider.nextDispatch(t)
	provider.respond(interfaces.InferenceResponse{}, workers.NewProviderErrorWithSession(
		interfaces.WorkFailureTypePermanentBadRequest,
		"provider rejected dashboard world-view work",
		errors.New("provider rejected"),
		&interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-world-view-failed"},
	))
	waitForHarnessWorkInPlace(t, h, "task:failed", "world-view-failed", time.Second)

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}
}

func scaffoldDashboardWorldViewFunctionalDir(t *testing.T) string {
	t.Helper()
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
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

func submitDashboardWorldViewFunctionalWork(t *testing.T, h *testutil.ServiceTestHarness, workID string, traceID string) {
	t.Helper()
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkID: workID, WorkTypeID: "task", TraceID: traceID, Payload: []byte(`{"item":"dashboard-world-view-functional"}`),
	}})
}

func waitForHarnessWorkInPlace(t *testing.T, h *testutil.ServiceTestHarness, placeID, workID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if support.HasWorkTokenInPlace(snap.Marking, placeID, workID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work %q in %s", workID, placeID)
}

type functionalWorldViewProvider struct {
	requests  chan interfaces.ProviderInferenceRequest
	responses chan functionalWorldViewProviderResponse
}

type functionalWorldViewProviderResponse struct {
	response interfaces.InferenceResponse
	err      error
}

func newFunctionalWorldViewProvider() *functionalWorldViewProvider {
	return &functionalWorldViewProvider{
		requests:  make(chan interfaces.ProviderInferenceRequest, 2),
		responses: make(chan functionalWorldViewProviderResponse, 2),
	}
}

func (p *functionalWorldViewProvider) Infer(ctx context.Context, request interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	select {
	case p.requests <- request:
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	}
	select {
	case response := <-p.responses:
		return response.response, response.err
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	}
}

func (p *functionalWorldViewProvider) nextDispatch(t *testing.T) interfaces.ProviderInferenceRequest {
	t.Helper()
	select {
	case request := <-p.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider dispatch")
		return interfaces.ProviderInferenceRequest{}
	}
}

func (p *functionalWorldViewProvider) respond(response interfaces.InferenceResponse, err error) {
	p.responses <- functionalWorldViewProviderResponse{response: response, err: err}
}

var _ workers.Provider = (*functionalWorldViewProvider)(nil)
