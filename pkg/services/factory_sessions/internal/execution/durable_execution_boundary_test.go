package factorysessionexecution_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const factoryRuntimeImportRoot = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

var executionLeaseImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/...",
}

// boundaryRootWorkflows implements factory.JavaScriptWorkflows using only
// Runtime root contracts so durable execution construction stays sealed.
type boundaryRootWorkflows struct{}

func (boundaryRootWorkflows) PreviewWorkflow(
	context.Context,
	factory.WorkflowPreviewInput,
) (factory.WorkflowPreview, error) {
	return factory.WorkflowPreview{}, nil
}

func (boundaryRootWorkflows) BuildPreview(factory.WorkflowPreviewRequest) factory.WorkflowPreview {
	return factory.WorkflowPreview{}
}

func (boundaryRootWorkflows) DefaultSourceContext(
	projectRoot string,
) (factory.WorkflowSourceContext, error) {
	return factory.WorkflowSourceContext{ProjectRoot: projectRoot}, nil
}

func (boundaryRootWorkflows) ResolveSource(
	request factory.WorkflowSourceRequest,
	_ factory.WorkflowSourceContext,
) factory.WorkflowSourceResolution {
	return factory.WorkflowSourceResolution{
		RequestKind:  request.Kind,
		RequestValue: request.Value,
		ResolvedKind: request.Kind,
		SourceRef:    "boundary.workflow.js",
		SourceHash:   "sha256:boundary",
		Dialect:      "you-workflow-v1",
		Content:      `return { subject: args.subject };`,
		Found:        true,
	}
}

func (boundaryRootWorkflows) LoadSource(
	factory.WorkflowValidationLoadRequest,
) (factory.WorkflowValidationLoadedSource, []factory.WorkflowValidationIssue) {
	return factory.WorkflowValidationLoadedSource{
		SourceRef:        "boundary.workflow.js",
		SourceHash:       "sha256:boundary",
		Format:           factory.WorkflowValidationFormatJavaScript,
		AuthoredSource:   `return { subject: args.subject };`,
		ExecutableSource: `return { subject: args.subject };`,
	}, nil
}

func (boundaryRootWorkflows) ValidateArgs([]byte, map[string]any) error { return nil }

func (boundaryRootWorkflows) ValidateLoaded(
	factory.WorkflowValidationLoadedSource,
	factory.WorkflowValidationRequest,
) factory.WorkflowValidationResult {
	return factory.WorkflowValidationResult{}
}

func (boundaryRootWorkflows) Validate(
	factory.WorkflowValidationRequest,
) factory.WorkflowValidationResult {
	return factory.WorkflowValidationResult{}
}

func (boundaryRootWorkflows) Run(
	_ context.Context,
	_ factory.JavaScriptRuntimeRequest,
	_ factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	encoded, err := json.Marshal(map[string]any{"subject": "boundary"})
	if err != nil {
		return factory.JavaScriptRuntimeOutcome{}, err
	}
	return factory.JavaScriptRuntimeOutcome{
		OK:    true,
		Value: factory.TypedValue{JSON: encoded},
	}, nil
}

func (boundaryRootWorkflows) ResumeContext(
	factory.JavaScriptCompletedCheckpointSummary,
	[]factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return factory.JavaScriptResumeContext{}
}

func (boundaryRootWorkflows) TextDigest(string) string { return "sha256:boundary-prompt" }

func (boundaryRootWorkflows) SchemaDigest(map[string]any) string { return "sha256:boundary-schema" }

func (boundaryRootWorkflows) CloneOutputMap(output map[string]any) map[string]any {
	cloned := make(map[string]any, len(output))
	for key, value := range output {
		cloned[key] = value
	}
	return cloned
}

type boundaryOrchestrationAdapter struct {
	factory.JavaScriptWorkflowRuntime
}

func (a boundaryOrchestrationAdapter) RunJavaScript(
	ctx context.Context,
	req factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	return a.Run(ctx, req, hooks)
}

func (a boundaryOrchestrationAdapter) ResumeJavaScript(
	summary factory.JavaScriptCompletedCheckpointSummary,
	records []factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return a.ResumeContext(summary, records)
}

func TestDurableExecutionConstructionUsesRootWorkflowContracts(t *testing.T) {
	t.Parallel()

	workflows := boundaryRootWorkflows{}
	orchestration := boundaryOrchestrationAdapter{workflows}
	service, err := factorysessionexecution.NewJavaScriptExecutionService(
		t.TempDir(),
		factorysessionexecution.ChildExecutorModeFake,
		nil,
		factorysessionexecution.DisabledPersistence(),
		fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		harnessSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		workflows,
		orchestration,
		workflows,
		nil,
		factory.JavaScriptWorkerSettings{},
		boundaryRecordingWriter{},
		func() string { return "dur-sess-boundary-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewJavaScriptExecutionService: %v", err)
	}

	started, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-boundary-root-workflow",
		Source: factorysessionexecution.Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &factorysessionexecution.InlineWorkflowSource{
				Dialect:      "you-workflow-v1",
				InlineSource: `return { subject: args.subject };`,
			},
		},
		Args: map[string]any{"subject": "boundary"},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", started.OrchestratorKind)
	}
	if started.ResolvedSource.SourceRef != "boundary.workflow.js" {
		t.Fatalf("resolved source ref = %q, want boundary.workflow.js", started.ResolvedSource.SourceRef)
	}
	if started.SourceHash != "sha256:boundary" {
		t.Fatalf("source hash = %q, want sha256:boundary", started.SourceHash)
	}
	if started.Status != string(factorysessionexecution.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
}

type boundaryRecordingWriter struct{}

func (boundaryRecordingWriter) Write(path string, value recordings.PortableRecording) error {
	return recordings.ValidatePortableRecording(value)
}
