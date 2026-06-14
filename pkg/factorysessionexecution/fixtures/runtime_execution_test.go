package fixtures_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const simpleFinalWorkflowSource = `// Simple final-only workflow fixture for runtime boundary tests.
return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

func TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult(t *testing.T) {
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{ProjectRoot: projectRoot},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}

	started, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-sync-simple-final-001",
		Source: fse.Source{
			Kind: workflowsource.KindInlineWorkflow,
			InlineWorkflow: &fse.InlineWorkflowSource{
				InlineSource: simpleFinalWorkflowSource,
				Dialect:      "you-workflow-v1",
				Metadata: map[string]string{
					"name":        "simple-final",
					"description": "returns a structured final value",
				},
			},
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-runtime-sync-001",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", started.SyncOutcome)
	}
	if started.Status != string(fse.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
	if started.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", started.OrchestratorKind)
	}
	if started.SessionID == "" {
		t.Fatal("expected durable session id")
	}
	if started.ResolvedSource.SourceRef == "" || started.SourceHash == "" {
		t.Fatalf("resolved source = %#v", started.ResolvedSource)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", session.Status)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
	if err := fse.ValidateResultMatchesSessionRead(session, fse.ResultReadResult{
		SessionID:     session.SessionID,
		ResultStatus:  fse.ResultStatusFinal,
		SessionStatus: session.Status,
	}); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	want := map[string]any{
		"label":       "simple-final",
		"description": "returns a structured final value",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
	}
	for key, wantValue := range want {
		if projected[key] != wantValue {
			t.Fatalf("primaryResult[%q] = %#v, want %#v", key, projected[key], wantValue)
		}
	}

	hash, err := fixtures.ProjectedResultReadHash(result)
	if err != nil {
		t.Fatalf("fixtures.ProjectedResultReadHash: %v", err)
	}
	if hash == "" {
		t.Fatal("expected stable projected result hash")
	}
}

func TestNewExecutionService_SelectsFakeAndJavaScriptRuntimeProviders(t *testing.T) {
	fakeService, err := fse.NewExecutionService(
		fse.ExecutionProviderFake,
		fse.ServiceConfig{},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	if _, ok := fakeService.(*fse.FakeService); !ok {
		t.Fatalf("fake provider type = %T, want *fse.FakeService", fakeService)
	}

	projectRoot := t.TempDir()
	runtimeService, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{ProjectRoot: projectRoot},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(runtime): %v", err)
	}
	if _, ok := runtimeService.(*fse.JavaScriptRuntimeService); !ok {
		t.Fatalf("runtime provider type = %T, want *fse.JavaScriptRuntimeService", runtimeService)
	}
}

func writeSimpleFinalWorkflowProject(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	workflowPath := filepath.Join(workflowDir, "simple-final.workflow.js")
	if err := os.WriteFile(workflowPath, []byte(simpleFinalWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func decodePrimaryResultMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var projected map[string]any
	if err := json.Unmarshal(raw, &projected); err != nil {
		t.Fatalf("unmarshal primary result: %v", err)
	}
	return projected
}
