package factory

import "context"

// OrchestrationJavaScriptExecution is the Runtime-owned JavaScript orchestration
// execute/resume surface. Focused orchestration call sites must depend on this
// port instead of injecting JavaScriptWorkflowRuntime directly.
type OrchestrationJavaScriptExecution interface {
	RunJavaScript(
		context.Context,
		JavaScriptRuntimeRequest,
		JavaScriptRuntimeHooks,
	) (JavaScriptRuntimeOutcome, error)
	ResumeJavaScript(
		JavaScriptCompletedCheckpointSummary,
		[]JavaScriptRuntimeRecord,
	) JavaScriptResumeContext
}
