package fixtures_test

import (
	"context"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_AgentRunLiveChild_ProjectsRealDispatchInspection(t *testing.T) {
	provider := newFixtureMockProvider(interfaces.InferenceResponse{
		Content: `{"text":"live:agent-run-fake-child:summarize-findings:summarize workflows:workflows"}`,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		},
	})
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child",
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
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	assertLiveChildDispatchInspection(t, service, completed, provider.callCount)
}

type fixtureMockProvider struct {
	response  interfaces.InferenceResponse
	callCount int
}

func newFixtureMockProvider(response interfaces.InferenceResponse) *fixtureMockProvider {
	return &fixtureMockProvider{response: response}
}

func (m *fixtureMockProvider) Infer(_ context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.callCount++
	return m.response, nil
}

func (m *fixtureMockProvider) CallCount() int {
	return m.callCount
}

func TestJavaScriptRuntimeService_AgentRunFakeChild_RemainsDefaultWithoutRuntimeOverride(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-fake-child-default",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), completed.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.JavaScript == nil || dispatchDetail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeFake {
		t.Fatalf("dispatch javascript = %#v, want fake execution mode", dispatchDetail.JavaScript)
	}
}
