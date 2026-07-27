// Package orchestration defines the parent-private Factory Runtime capability
// that selects orchestration kind, compiles activated definitions, and later
// drives execute/resume for private Petri and JavaScript variants.
package orchestration

import (
	"context"
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ErrUnsupportedKind reports an authored orchestration kind outside the
// supported Runtime vocabulary.
var ErrUnsupportedKind = errors.New("unsupported orchestration kind")

// ErrDefinitionUnavailable reports a missing activated Factory definition.
var ErrDefinitionUnavailable = errors.New("orchestration definition unavailable")

// ErrInvalidDefinition reports an activated definition that cannot compile into
// a runnable orchestration binding.
var ErrInvalidDefinition = errors.New("invalid orchestration definition")

// Kind is the orchestration-neutral strategy selector published by Runtime.
type Kind string

const (
	// KindPetri selects the private Petri orchestration variant.
	KindPetri Kind = "PETRI"
	// KindJavaScript selects the private JavaScript orchestration variant.
	KindJavaScript Kind = "JAVASCRIPT"
)

// Diagnostic is a typed Runtime-facing compile failure without exposing Petri
// net/marking shapes or JavaScript VM internals.
type Diagnostic struct {
	Code    string
	Message string
	Path    string
}

// CompileError carries one or more diagnostics for a failed compile attempt.
type CompileError struct {
	Err          error
	Diagnostics  []Diagnostic
	Orchestrator Kind
}

func (e *CompileError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Diagnostics) == 0 {
		return e.Err.Error()
	}
	return e.Diagnostics[0].Message
}

func (e *CompileError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Binding is the opaque compiled orchestration binding for one activated
// Factory definition. Only the orchestration package may unwrap variant state.
type Binding interface {
	OrchestrationKind() Kind
	isOrchestrationBinding()
}

// CompileRequest carries the activated Factory definition inputs needed for
// inert kind selection and compilation.
type CompileRequest struct {
	Config         *factorydefinitions.FactoryConfig
	FactoryDir     string
	SourceReader   factoryruntime.WorkflowSourceReader
}

// CompileResult is the inert compile success shape. It does not start a runtime
// loop, publish Workers requests, or capture/restore checkpoints.
type CompileResult struct {
	Kind    Kind
	Binding Binding
}

// Service owns orchestration kind selection, activated-definition compilation,
// and private-variant execute/resume for Petri and JavaScript orchestration.
type Service interface {
	Compile(context.Context, CompileRequest) (CompileResult, error)
	RunJavaScript(
		context.Context,
		factoryruntime.JavaScriptRuntimeRequest,
		factoryruntime.JavaScriptRuntimeHooks,
	) (factoryruntime.JavaScriptRuntimeOutcome, error)
	ResumeJavaScript(
		factoryruntime.JavaScriptCompletedCheckpointSummary,
		[]factoryruntime.JavaScriptRuntimeRecord,
	) factoryruntime.JavaScriptResumeContext
}
