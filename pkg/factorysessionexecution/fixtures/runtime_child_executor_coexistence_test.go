package fixtures_test

import (
	"context"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_ChildExecutorModes_CoexistOnSameWorkflowSource(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := newFixtureMockProvider(interfaces.InferenceResponse{
		Content: `{"text":"live child output"}`,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		},
	})

	fakeService := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	liveService := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	fakeCompleted := startAgentRunFakeChild(t, fakeService, "req-runtime-child-mode-fake")
	liveCompleted := startAgentRunFakeChild(t, liveService, "req-runtime-child-mode-live")

	fakeDetail := dispatchExecutionMode(t, fakeService, fakeCompleted.SessionID, "dispatch-1")
	liveDetail := dispatchExecutionMode(t, liveService, liveCompleted.SessionID, "dispatch-1")

	if fakeDetail != workflowruntime.ChildExecutionModeFake {
		t.Fatalf("fake dispatch executionMode = %q, want %q", fakeDetail, workflowruntime.ChildExecutionModeFake)
	}
	if liveDetail != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("live dispatch executionMode = %q, want %q", liveDetail, workflowruntime.ChildExecutionModeLive)
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1 (live path only)", provider.callCount)
	}
}

func TestJavaScriptRuntimeService_ExplicitFakeMode_OverridesLiveServiceConfig(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := newFixtureMockProvider(interfaces.InferenceResponse{
		Content: `{"text":"unused live output"}`,
	})
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-child-mode-explicit-fake",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeFake,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	mode := dispatchExecutionMode(t, service, completed.SessionID, "dispatch-1")
	if mode != workflowruntime.ChildExecutionModeFake {
		t.Fatalf("dispatch executionMode = %q, want %q", mode, workflowruntime.ChildExecutionModeFake)
	}
	if provider.callCount != 0 {
		t.Fatalf("provider call count = %d, want 0 for explicit fake override", provider.callCount)
	}
}

func TestJavaScriptRuntimeService_ParallelFakeChildren_RemainsDeterministicWithoutProvider(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "parallel-fake-children.workflow.js", "parallel-fake-children")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-parallel-fake-children",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "parallel-fake-children",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		RequestedPolicy: map[string]any{
			"maxAgents":   8,
			"concurrency": 2,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 4 {
		t.Fatalf("dispatches = %#v, want four fake children", dispatches.Dispatches)
	}
	for _, dispatch := range dispatches.Dispatches {
		detail, err := service.GetDispatch(context.Background(), completed.SessionID, dispatch.ID)
		if err != nil {
			t.Fatalf("GetDispatch(%s): %v", dispatch.ID, err)
		}
		if detail.JavaScript == nil || detail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeFake {
			t.Fatalf("dispatch %s javascript = %#v, want fake execution mode", dispatch.ID, detail.JavaScript)
		}
	}

	result, err := service.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	primary := decodePrimaryResultMap(t, result.PrimaryResult)
	results, ok := primary["results"].([]any)
	if !ok || len(results) != 4 {
		t.Fatalf("primary results = %#v, want four child results", primary["results"])
	}
	for index, entry := range results {
		child, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object", index, entry)
		}
		if child["executionMode"] != workflowruntime.ChildExecutionModeFake {
			t.Fatalf("results[%d].executionMode = %#v, want fake", index, child["executionMode"])
		}
	}
}

func TestJavaScriptRuntimeService_PipelineFakeChildren_RemainsDeterministicWithoutProvider(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "pipeline-staged-fake-children.workflow.js", "pipeline-staged-fake-children")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-pipeline-fake-children",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "pipeline-staged-fake-children",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 6 {
		t.Fatalf("dispatches = %#v, want six staged fake children", dispatches.Dispatches)
	}
	for _, dispatch := range dispatches.Dispatches {
		detail, err := service.GetDispatch(context.Background(), completed.SessionID, dispatch.ID)
		if err != nil {
			t.Fatalf("GetDispatch(%s): %v", dispatch.ID, err)
		}
		if detail.JavaScript == nil || detail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeFake {
			t.Fatalf("dispatch %s javascript = %#v, want fake execution mode", dispatch.ID, detail.JavaScript)
		}
	}
}

func startAgentRunFakeChild(t *testing.T, service fse.Service, requestID string) fse.SyncStartResult {
	t.Helper()
	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: requestID,
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync(%s): %v", requestID, err)
	}
	return completed
}

func dispatchExecutionMode(t *testing.T, service fse.Service, sessionID, dispatchID string) string {
	t.Helper()
	detail, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if detail.JavaScript == nil {
		t.Fatal("dispatch javascript projection is nil")
	}
	return detail.JavaScript.ExecutionMode
}
