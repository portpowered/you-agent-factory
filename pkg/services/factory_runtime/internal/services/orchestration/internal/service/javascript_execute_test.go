package service_test

import (
	"context"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/internal/service"
)

const progressPrimitivesWorkflowSource = `
phase("setup");
log("starting workflow", { step: 1 });
workflow.log("workflow step");
phase("execute");
const artifactRef = workflow.artifact({
  kind: "log",
  label: "step-output",
  content: { message: "hello" },
});
workflow.checkpoint({
  label: "after-artifact",
  state: { artifactRef: artifactRef, step: 2 },
});
return { label: meta.name, artifactRef: artifactRef };
`

const deniedModelWorkflowSource = `
agent.run({
  prompt: "summarize workflows",
  label: "denied-model",
  model: "gpt-denied",
});
return { ok: true };
`

func TestRunJavaScriptPreservesPhaseAndCheckpointRecordOrdering(t *testing.T) {
	t.Parallel()

	service := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	policy := workflowpolicy.DefaultEffectivePolicy()
	outcome, err := service.RunJavaScript(context.Background(), factoryruntime.JavaScriptRuntimeRequest{
		Source:    progressPrimitivesWorkflowSource,
		SessionID: "orchestration-phase-session",
		Metadata:  map[string]string{"name": "progress-primitives"},
		Policy:    policy,
	}, factoryruntime.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("RunJavaScript() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("RunJavaScript() outcome = %#v, want success", outcome)
	}
	assertRecordKindsIncludeSequence(t, outcome.Records,
		factoryruntime.JavaScriptRecordKindPhase,
		factoryruntime.JavaScriptRecordKindCheckpoint,
	)
}

func TestRunJavaScriptDeniedPolicyReturnsStableRuntimeFacingDiagnostic(t *testing.T) {
	t.Parallel()

	service := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.AllowedModels = []string{"gpt-allowed"}
	outcome, err := service.RunJavaScript(context.Background(), factoryruntime.JavaScriptRuntimeRequest{
		Source:    deniedModelWorkflowSource,
		SessionID: "orchestration-denied-session",
		Policy:    policy,
	}, factoryruntime.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("RunJavaScript() error = %v", err)
	}
	if outcome.OK {
		t.Fatalf("RunJavaScript() outcome = %#v, want failure", outcome)
	}
	if outcome.Failure.Code != factoryruntime.JavaScriptRuntimeCodeScriptError {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, factoryruntime.JavaScriptRuntimeCodeScriptError)
	}
	if !strings.Contains(outcome.Failure.Message, `policy denied: model "gpt-denied"`) {
		t.Fatalf("failure message = %q, want policy denied diagnostic", outcome.Failure.Message)
	}
	if strings.Contains(outcome.Failure.Message, "vm.") || strings.Contains(outcome.Failure.Message, "goja") {
		t.Fatalf("failure message leaks VM internals: %q", outcome.Failure.Message)
	}
}

func assertRecordKindsIncludeSequence(t *testing.T, records []factoryruntime.JavaScriptRuntimeRecord, wantKinds ...string) {
	t.Helper()
	index := 0
	for _, record := range records {
		if index >= len(wantKinds) {
			return
		}
		if record.Kind == wantKinds[index] {
			index++
		}
	}
	if index != len(wantKinds) {
		t.Fatalf("records = %#v, want ordered kinds %v", records, wantKinds)
	}
}
