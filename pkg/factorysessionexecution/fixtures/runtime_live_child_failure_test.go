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

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", read.Status)
	}
	if read.Progress == nil || read.Progress.TotalDispatches != 3 || read.Progress.CompletedDispatches != 2 || read.Progress.FailedDispatches != 1 {
		t.Fatalf("progress = %#v, want two completed and one failed dispatch", read.Progress)
	}

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatches = %#v, want three dispatches", dispatches.Dispatches)
	}

	dispatchByID := map[string]fse.DispatchSummary{}
	for _, dispatch := range dispatches.Dispatches {
		dispatchByID[dispatch.ID] = dispatch
	}
	for _, dispatchID := range []string{"dispatch-1", "dispatch-2", "dispatch-3"} {
		if _, ok := dispatchByID[dispatchID]; !ok {
			t.Fatalf("missing dispatch %q in %#v", dispatchID, dispatches.Dispatches)
		}
	}

	successOne := dispatchByID["dispatch-1"]
	successThree := dispatchByID["dispatch-3"]
	failed := dispatchByID["dispatch-2"]
	if successOne.Status != fse.DispatchStatusCompleted || successThree.Status != fse.DispatchStatusCompleted {
		t.Fatalf("sibling dispatches = %#v/%#v, want COMPLETED", successOne, successThree)
	}
	if len(successOne.OutputArtifactIDs) == 0 || len(successThree.OutputArtifactIDs) == 0 {
		t.Fatalf("sibling output artifacts = %#v/%#v, want one artifact each", successOne.OutputArtifactIDs, successThree.OutputArtifactIDs)
	}
	if len(successOne.ProviderSessionRefs) != 1 || len(successThree.ProviderSessionRefs) != 1 {
		t.Fatalf("sibling provider refs = %#v/%#v", successOne.ProviderSessionRefs, successThree.ProviderSessionRefs)
	}

	if failed.Status != fse.DispatchStatusFailed {
		t.Fatalf("failed dispatch status = %q, want FAILED", failed.Status)
	}
	if failed.FailureDetail == nil {
		t.Fatal("failed dispatch missing failureDetail")
	}
	if failed.FailureDetail.Reason != workflowruntime.ChildExecutionFailureReason {
		t.Fatalf("failure reason = %q, want %q", failed.FailureDetail.Reason, workflowruntime.ChildExecutionFailureReason)
	}
	if !strings.Contains(failed.FailureDetail.Message, "simulated child error") {
		t.Fatalf("failure message = %q, want simulated child error detail", failed.FailureDetail.Message)
	}
	if len(failed.OutputArtifactIDs) != 0 {
		t.Fatalf("failed outputArtifactIds = %#v, want none", failed.OutputArtifactIDs)
	}

	failedDetail, err := service.GetDispatch(context.Background(), completed.SessionID, failed.ID)
	if err != nil {
		t.Fatalf("GetDispatch failed child: %v", err)
	}
	if failedDetail.JavaScript == nil || failedDetail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("failed javascript projection = %#v, want live-provider", failedDetail.JavaScript)
	}
	if len(failedDetail.StatusTransitions) != 3 {
		t.Fatalf("failed statusTransitions = %#v, want queued/running/failed", failedDetail.StatusTransitions)
	}
	wantTransitions := []fse.DispatchStatus{
		fse.DispatchStatusQueued,
		fse.DispatchStatusRunning,
		fse.DispatchStatusFailed,
	}
	for index, got := range failedDetail.StatusTransitions {
		if got != wantTransitions[index] {
			t.Fatalf("failed statusTransitions[%d] = %q, want %q", index, got, wantTransitions[index])
		}
	}
	if failedDetail.FailureDetail == nil || failedDetail.FailureDetail.Reason != workflowruntime.ChildExecutionFailureReason {
		t.Fatalf("failed dispatch detail failure = %#v", failedDetail.FailureDetail)
	}

	successDetail, err := service.GetDispatch(context.Background(), completed.SessionID, successOne.ID)
	if err != nil {
		t.Fatalf("GetDispatch success child: %v", err)
	}
	if successDetail.FailureDetail != nil {
		t.Fatalf("success dispatch failureDetail = %#v, want nil", successDetail.FailureDetail)
	}
	if len(successDetail.ArtifactIDs) != 1 {
		t.Fatalf("success dispatch artifactIds = %#v, want one artifact", successDetail.ArtifactIDs)
	}

	result, err := service.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal || result.SessionStatus != fse.LifecycleStatusSucceeded {
		t.Fatalf("result = status %q session %q, want FINAL/SUCCEEDED", result.ResultStatus, result.SessionStatus)
	}
	primary := decodePrimaryResultMap(t, result.PrimaryResult)
	results, ok := primary["results"].([]any)
	if !ok || len(results) != 3 {
		t.Fatalf("primary results = %#v, want three child entries", primary["results"])
	}
	failedChild, ok := results[1].(map[string]any)
	if !ok || failedChild["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("primary failed child = %#v", results[1])
	}

	artifacts, err := service.ListArtifacts(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.DispatchID == failed.ID {
			t.Fatalf("failed dispatch produced artifact = %#v", artifact)
		}
	}
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
