package fixtures_test

import (
	"context"
	"strings"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_ParallelLiveChildFailure_ProjectsTypedFailureAndPreservesSiblings(t *testing.T) {
	provider := newParallelLiveChildMockProvider()
	projectRoot := setupRuntimeWorkflowFixture(t, "parallel-child-failure.workflow.js", "parallel-child-failure")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-parallel-live-child-failure",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "parallel-child-failure",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	assertParallelLiveChildFailureInspection(t, service, completed)
}

func TestJavaScriptRuntimeService_AgentRunLiveChildFailure_ProjectsFailedDispatchOnWorkflowFailure(t *testing.T) {
	provider := newFixtureMockProvider(interfaces.InferenceResponse{Content: `{"text":"unused"}`})
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-live-child-failure.workflow.js", "agent-run-live-child-failure")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child-failure",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-live-child-failure",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != fse.LifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", read.Status)
	}
	if read.Failure == nil || read.Failure.Reason == "" {
		t.Fatalf("session failure = %#v, want typed workflow failure", read.Failure)
	}
	if read.Progress == nil || read.Progress.TotalDispatches != 1 || read.Progress.FailedDispatches != 1 {
		t.Fatalf("progress = %#v, want one failed dispatch", read.Progress)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), completed.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.Status != fse.DispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", dispatchDetail.Status)
	}
	if dispatchDetail.FailureDetail == nil || dispatchDetail.FailureDetail.Reason != workflowruntime.ChildExecutionFailureReason {
		t.Fatalf("dispatch failureDetail = %#v", dispatchDetail.FailureDetail)
	}
	if !strings.Contains(dispatchDetail.FailureDetail.Message, "simulated live child error") {
		t.Fatalf("dispatch failure message = %q", dispatchDetail.FailureDetail.Message)
	}
}

type parallelLiveChildMockProvider struct {
	callCount int
}

func newParallelLiveChildMockProvider() *parallelLiveChildMockProvider {
	return &parallelLiveChildMockProvider{}
}

func (m *parallelLiveChildMockProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.callCount++
	return interfaces.InferenceResponse{
		Content: `{"text":"live:` + req.Dispatch.DispatchID + `:` + req.UserMessage + `"}`,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-" + req.Dispatch.DispatchID,
		},
	}, nil
}
