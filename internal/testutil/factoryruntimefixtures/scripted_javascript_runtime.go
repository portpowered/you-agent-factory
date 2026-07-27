package factoryruntimefixtures

import (
	"context"
	"encoding/json"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ScriptedJavaScriptWorkflows is a programmable Factory Runtime root fake.
// Cross-owner tests provide only the source, runtime, and child-value outcomes
// they observe; JavaScript parsing and execution policy remain owner-local.
type ScriptedJavaScriptWorkflows struct {
	PreviewWorkflowFunc      func(context.Context, factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error)
	BuildPreviewFunc         func(factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview
	DefaultSourceContextFunc func(string) (factoryruntime.WorkflowSourceContext, error)
	ResolveSourceFunc        func(factoryruntime.WorkflowSourceRequest, factoryruntime.WorkflowSourceContext) factoryruntime.WorkflowSourceResolution
	LoadSourceFunc           func(factoryruntime.WorkflowValidationLoadRequest) (factoryruntime.WorkflowValidationLoadedSource, []factoryruntime.WorkflowValidationIssue)
	ValidateArgsFunc         func([]byte, map[string]any) error
	ValidateLoadedFunc       func(factoryruntime.WorkflowValidationLoadedSource, factoryruntime.WorkflowValidationRequest) factoryruntime.WorkflowValidationResult
	ValidateFunc             func(factoryruntime.WorkflowValidationRequest) factoryruntime.WorkflowValidationResult
	RunFunc                  func(context.Context, factoryruntime.JavaScriptRuntimeRequest, factoryruntime.JavaScriptRuntimeHooks) (factoryruntime.JavaScriptRuntimeOutcome, error)
	ResumeContextFunc        func(factoryruntime.JavaScriptCompletedCheckpointSummary, []factoryruntime.JavaScriptRuntimeRecord) factoryruntime.JavaScriptResumeContext
	TextDigestFunc           func(string) string
	SchemaDigestFunc         func(map[string]any) string
	CloneOutputMapFunc       func(map[string]any) map[string]any
}

var (
	_ factoryruntime.JavaScriptWorkflows             = ScriptedJavaScriptWorkflows{}
	_ factoryruntime.OrchestrationJavaScriptExecution = ScriptedJavaScriptWorkflows{}
)

func (s ScriptedJavaScriptWorkflows) PreviewWorkflow(ctx context.Context, input factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error) {
	if s.PreviewWorkflowFunc == nil {
		return factoryruntime.WorkflowPreview{}, fmt.Errorf("unexpected PreviewWorkflow call")
	}
	return s.PreviewWorkflowFunc(ctx, input)
}

func (s ScriptedJavaScriptWorkflows) BuildPreview(request factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview {
	if s.BuildPreviewFunc == nil {
		return factoryruntime.WorkflowPreview{}
	}
	return s.BuildPreviewFunc(request)
}

func (s ScriptedJavaScriptWorkflows) DefaultSourceContext(root string) (factoryruntime.WorkflowSourceContext, error) {
	if s.DefaultSourceContextFunc == nil {
		return factoryruntime.WorkflowSourceContext{ProjectRoot: root}, nil
	}
	return s.DefaultSourceContextFunc(root)
}

func (s ScriptedJavaScriptWorkflows) ResolveSource(request factoryruntime.WorkflowSourceRequest, sourceContext factoryruntime.WorkflowSourceContext) factoryruntime.WorkflowSourceResolution {
	if s.ResolveSourceFunc == nil {
		content := request.InlineSource
		if content == "" {
			content = request.Value
		}
		sourceRef := request.Value
		if request.Kind == factoryruntime.WorkflowSourceKindInlineWorkflow {
			sourceRef = "scripted-inline.workflow.js"
		}
		return factoryruntime.WorkflowSourceResolution{
			RequestKind:  request.Kind,
			RequestValue: request.Value,
			ResolvedKind: request.Kind,
			SourceRef:    sourceRef,
			SourceHash:   "sha256:scripted",
			Dialect:      "you-workflow-v1",
			Content:      content,
			ArtifactRoot: factoryruntime.WorkflowSourceArtifactRootDecision{Allowed: true},
			Found:        true,
		}
	}
	return s.ResolveSourceFunc(request, sourceContext)
}

func (s ScriptedJavaScriptWorkflows) LoadSource(request factoryruntime.WorkflowValidationLoadRequest) (factoryruntime.WorkflowValidationLoadedSource, []factoryruntime.WorkflowValidationIssue) {
	if s.LoadSourceFunc == nil {
		return factoryruntime.WorkflowValidationLoadedSource{
			SourceRef:        request.SourceRef,
			SourceHash:       "sha256:scripted",
			Format:           factoryruntime.WorkflowValidationFormatJavaScript,
			AuthoredSource:   request.Content,
			ExecutableSource: request.Content,
		}, nil
	}
	return s.LoadSourceFunc(request)
}

func (s ScriptedJavaScriptWorkflows) ValidateArgs(schema []byte, args map[string]any) error {
	if s.ValidateArgsFunc == nil {
		return nil
	}
	return s.ValidateArgsFunc(schema, args)
}

func (s ScriptedJavaScriptWorkflows) ValidateLoaded(loaded factoryruntime.WorkflowValidationLoadedSource, request factoryruntime.WorkflowValidationRequest) factoryruntime.WorkflowValidationResult {
	if s.ValidateLoadedFunc == nil {
		return factoryruntime.WorkflowValidationResult{}
	}
	return s.ValidateLoadedFunc(loaded, request)
}

func (s ScriptedJavaScriptWorkflows) Validate(request factoryruntime.WorkflowValidationRequest) factoryruntime.WorkflowValidationResult {
	if s.ValidateFunc == nil {
		return factoryruntime.WorkflowValidationResult{}
	}
	return s.ValidateFunc(request)
}

func (s ScriptedJavaScriptWorkflows) RunJavaScript(
	ctx context.Context,
	request factoryruntime.JavaScriptRuntimeRequest,
	hooks factoryruntime.JavaScriptRuntimeHooks,
) (factoryruntime.JavaScriptRuntimeOutcome, error) {
	return s.Run(ctx, request, hooks)
}

func (s ScriptedJavaScriptWorkflows) ResumeJavaScript(
	summary factoryruntime.JavaScriptCompletedCheckpointSummary,
	records []factoryruntime.JavaScriptRuntimeRecord,
) factoryruntime.JavaScriptResumeContext {
	return s.ResumeContext(summary, records)
}

func (s ScriptedJavaScriptWorkflows) Run(ctx context.Context, request factoryruntime.JavaScriptRuntimeRequest, hooks factoryruntime.JavaScriptRuntimeHooks) (factoryruntime.JavaScriptRuntimeOutcome, error) {
	if s.RunFunc == nil {
		value, err := json.Marshal(map[string]any{"status": "scripted"})
		if err != nil {
			return factoryruntime.JavaScriptRuntimeOutcome{}, err
		}
		return factoryruntime.JavaScriptRuntimeOutcome{
			OK:    true,
			Value: factoryruntime.TypedValue{JSON: value},
		}, nil
	}
	return s.RunFunc(ctx, request, hooks)
}

func (s ScriptedJavaScriptWorkflows) ResumeContext(summary factoryruntime.JavaScriptCompletedCheckpointSummary, records []factoryruntime.JavaScriptRuntimeRecord) factoryruntime.JavaScriptResumeContext {
	if s.ResumeContextFunc == nil {
		return factoryruntime.JavaScriptResumeContext{}
	}
	return s.ResumeContextFunc(summary, records)
}

func (s ScriptedJavaScriptWorkflows) TextDigest(text string) string {
	if s.TextDigestFunc == nil {
		return fmt.Sprintf("scripted-text:%d", len(text))
	}
	return s.TextDigestFunc(text)
}

func (s ScriptedJavaScriptWorkflows) SchemaDigest(schema map[string]any) string {
	if s.SchemaDigestFunc == nil {
		return fmt.Sprintf("scripted-schema:%d", len(schema))
	}
	return s.SchemaDigestFunc(schema)
}

func (s ScriptedJavaScriptWorkflows) CloneOutputMap(output map[string]any) map[string]any {
	if s.CloneOutputMapFunc != nil {
		return s.CloneOutputMapFunc(output)
	}
	cloned := make(map[string]any, len(output))
	for key, value := range output {
		cloned[key] = value
	}
	return cloned
}
