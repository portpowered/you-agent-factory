package functional_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// TestDashboard_EngineStateSnapshot_EndToEnd validates the full path from
// factory run to service-layer state aggregation and event-first world-view
// projection.
func TestDashboard_EngineStateSnapshot_EndToEnd(t *testing.T) {
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
	assertFunctionalWorldViewActive(t, buildFunctionalWorldView(t, h), "world-view-success")
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
		interfaces.ProviderErrorTypePermanentBadRequest,
		"provider rejected dashboard world-view work",
		errors.New("provider rejected"),
		&interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-world-view-failed",
		},
	))
	waitForHarnessWorkInPlace(t, h, "task:failed", "world-view-failed", time.Second)

	assertFunctionalWorldViewTerminalSession(t, buildFunctionalWorldView(t, h))

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}
}

func scaffoldDashboardWorldViewFunctionalDir(t *testing.T) string {
	t.Helper()
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure:      &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
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

func submitDashboardWorldViewFunctionalWork(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	workID string,
	traceID string,
) {
	t.Helper()
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"item":"dashboard-world-view-functional"}`),
	}})
}

func buildFunctionalWorldView(t *testing.T, h *testutil.ServiceTestHarness) interfaces.FactoryWorldView {
	t.Helper()
	es, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if es.FactoryState == "" {
		t.Fatal("expected non-empty FactoryState in engine state snapshot")
	}

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	worldState, err := projections.ReconstructFactoryWorldState(events, es.TickCount)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	return projections.BuildFactoryWorldViewWithActiveThrottlePauses(worldState, es.ActiveThrottlePauses)
}

func assertFunctionalWorldViewActive(t *testing.T, view interfaces.FactoryWorldView, workID string) {
	t.Helper()
	if view.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("InFlightDispatchCount = %d, want 1", view.Runtime.InFlightDispatchCount)
	}
	if view.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("DispatchedCount = %d, want 1", view.Runtime.Session.DispatchedCount)
	}
	for _, execution := range view.Runtime.ActiveExecutionsByDispatchID {
		for _, item := range execution.WorkItems {
			if item.WorkID == workID {
				return
			}
		}
	}
	t.Fatalf("active executions did not include work %q: %#v", workID, view.Runtime.ActiveExecutionsByDispatchID)
}

func assertFunctionalWorldViewTerminalSession(t *testing.T, view interfaces.FactoryWorldView) {
	t.Helper()
	session := view.Runtime.Session
	if session.DispatchedCount != 2 || session.CompletedCount != 1 || session.FailedCount != 1 {
		t.Fatalf("session counts = %#v, want dispatched=2 completed=1 failed=1", session)
	}
	if len(session.DispatchHistory) != 2 {
		t.Fatalf("dispatch history length = %d, want 2", len(session.DispatchHistory))
	}
	assertFunctionalWorldViewProviderSessions(t, session.ProviderSessions)
	if !functionalWorldViewContainsWorkInPlace(view, "task:failed", "world-view-failed") {
		t.Fatalf("failed occupancy missing world-view-failed: %#v", view.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:failed"])
	}
}

func assertFunctionalWorldViewProviderSessions(
	t *testing.T,
	sessions []interfaces.FactoryWorldProviderSessionRecord,
) {
	t.Helper()
	seen := map[string]bool{}
	for _, session := range sessions {
		seen[session.ProviderSession.ID] = true
	}
	for _, want := range []string{"sess-world-view-success", "sess-world-view-failed"} {
		if !seen[want] {
			t.Fatalf("provider sessions = %#v, missing %q", sessions, want)
		}
	}
}

func functionalWorldViewContainsWorkInPlace(
	view interfaces.FactoryWorldView,
	placeID string,
	workID string,
) bool {
	for _, item := range view.Runtime.PlaceOccupancyWorkItemsByPlaceID[placeID] {
		if item.WorkID == workID {
			return true
		}
	}
	return false
}

func waitForHarnessWorkInPlace(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	placeID string,
	workID string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if hasWorkTokenInPlace(snap.Marking, placeID, workID) {
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

func (p *functionalWorldViewProvider) Infer(
	ctx context.Context,
	request interfaces.ProviderInferenceRequest,
) (interfaces.InferenceResponse, error) {
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
