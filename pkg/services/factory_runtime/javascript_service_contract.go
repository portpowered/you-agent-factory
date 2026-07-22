package factory

import "context"

// JavaScriptWorkflowDefinitions owns JavaScript workflow source resolution,
// loading, validation, and preview behavior. Consumers receive this capability
// from composition instead of calling implementation-forwarding package vars.
type JavaScriptWorkflowDefinitions interface {
	BuildPreview(WorkflowPreviewRequest) WorkflowPreview
	DefaultSourceContext(string) (WorkflowSourceContext, error)
	ResolveSource(WorkflowSourceRequest, WorkflowSourceContext) WorkflowSourceResolution
	LoadSource(WorkflowValidationLoadRequest) (WorkflowValidationLoadedSource, []WorkflowValidationIssue)
	ValidateArgs([]byte, map[string]any) error
	ValidateLoaded(WorkflowValidationLoadedSource, WorkflowValidationRequest) WorkflowValidationResult
	Validate(WorkflowValidationRequest) WorkflowValidationResult
}

// JavaScriptWorkflowRuntime executes and resumes JavaScript workflows.
type JavaScriptWorkflowRuntime interface {
	Run(context.Context, JavaScriptRuntimeRequest, JavaScriptRuntimeHooks) (JavaScriptRuntimeOutcome, error)
	ResumeContext(
		JavaScriptCompletedCheckpointSummary,
		[]JavaScriptRuntimeRecord,
	) JavaScriptResumeContext
}

// JavaScriptChildValues owns canonical child-dispatch digest and cloning
// behavior used at Factory Session execution boundaries.
type JavaScriptChildValues interface {
	TextDigest(string) string
	SchemaDigest(map[string]any) string
	CloneOutputMap(map[string]any) map[string]any
}

// JavaScriptWorkflows is the complete JavaScript orchestrator capability
// selected by Wire. Consumers should request the narrow embedded capability
// they actually use.
type JavaScriptWorkflows interface {
	JavaScriptWorkflowDefinitions
	WorkflowPreviewOperation
	JavaScriptWorkflowRuntime
	JavaScriptChildValues
}
