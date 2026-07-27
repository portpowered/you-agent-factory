package factory

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

// OrchestrationKind is the orchestration-neutral strategy selector published by
// Runtime for activated Factory definitions.
type OrchestrationKind string

const (
	// OrchestrationKindPetri selects the private Petri orchestration variant.
	OrchestrationKindPetri OrchestrationKind = "PETRI"
	// OrchestrationKindJavaScript selects the private JavaScript orchestration variant.
	OrchestrationKindJavaScript OrchestrationKind = "JAVASCRIPT"
)

// OrchestrationCompileRequest carries activated Factory definition inputs for
// inert orchestration kind selection and compilation.
type OrchestrationCompileRequest struct {
	Config       *factorydefinitions.FactoryConfig
	FactoryDir   string
	SourceReader WorkflowSourceReader
}

// OrchestrationCompileResult is the inert compile success shape. It does not
// start a runtime loop, publish Workers requests, or capture/restore checkpoints.
type OrchestrationCompileResult struct {
	Kind OrchestrationKind
}

// OrchestrationCompilation is the Runtime-owned orchestration kind selection and
// activated-definition compilation port. Focused orchestration call sites must
// depend on this port instead of constructing Petri nets or JavaScript runtimes
// directly.
type OrchestrationCompilation interface {
	Compile(context.Context, OrchestrationCompileRequest) (OrchestrationCompileResult, error)
	CompilePetriNet(context.Context, OrchestrationCompileRequest) (*state.Net, error)
}

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
