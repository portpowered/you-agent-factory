package internal

import (
	"context"
	"fmt"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/prompting"
)

// ExecuteCapability is the request-scoped Execute owner composed into the
// Workers root.
type ExecuteCapability interface {
	Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

type promptRenderer interface {
	RenderPrompt(string, []workers.Token, *workers.Context) (string, error)
}

// Root is the inert Workers root composed from one request-scoped execution
// owner. It starts no lifecycle and retains no runtime or dispatch state.
type Root struct {
	execute ExecuteCapability
}

var _ workers.Service = (*Root)(nil)

var _ workers.PromptTemplates = (*Root)(nil)

// NewRoot constructs the inert Workers root from the one directly injected
// request-scoped execution owner.
func NewRoot(execute ExecuteCapability) (workers.Service, error) {
	if execute == nil {
		return nil, fmt.Errorf("construct Workers: execution owner is required")
	}
	return &Root{execute: execute}, nil
}

// Execute delegates one isolated attempt to the request-scoped Execute owner.
func (r *Root) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	if r == nil || r.execute == nil {
		return workers.ExecuteResult{}, workers.ErrExecuteUnavailable
	}
	return r.execute.Execute(ctx, request)
}

// InvokeModel is unavailable on the inert Workers root; direct model invocation
// requires a constructed runtime service.
func (r *Root) InvokeModel(
	context.Context,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, fmt.Errorf("factory service runtime is not available")
}

// BuildPromptTemplateContract keeps editor-facing prompt validation on the
// process-scoped Workers root. Prompt inspection is pure and does not require
// a Factory Session or a live Runtime.
func (*Root) BuildPromptTemplateContract(inputCount int, docPaths []string) workers.PromptTemplateContract {
	return workerprompting.BuildPromptTemplateContract(inputCount, docPaths)
}

// ValidatePromptTemplate keeps editor-facing prompt validation on the
// process-scoped Workers root. The request is evaluated from explicit prompt
// data and has no session lookup side effect.
func (*Root) ValidatePromptTemplate(
	template string,
	inputCount int,
	docPaths []string,
) workers.PromptTemplateValidationResult {
	return workerprompting.ValidatePromptTemplate(template, inputCount, docPaths)
}

// RenderPrompt forwards the optional request-scoped renderer owned by the
// concrete Workers service. Keeping this capability on the root preserves the
// process composition boundary without exposing the implementation owner to
// Factory Runtime.
func (r *Root) RenderPrompt(
	template string,
	tokens []workers.Token,
	workflowContext *workers.Context,
) (string, error) {
	if r == nil || r.execute == nil {
		return "", fmt.Errorf("render Worker prompt: execution service is unavailable")
	}
	renderer, ok := r.execute.(promptRenderer)
	if !ok {
		return "", fmt.Errorf("render Worker prompt: renderer is unavailable")
	}
	return renderer.RenderPrompt(template, tokens, workflowContext)
}

// ResolveTemplateFields forwards request-scoped workstation field rendering
// to the canonical Workers prompting implementation. Factory Runtime uses
// this optional capability when it constructs a detached execution target.
func (r *Root) ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	workflowContext *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	if r == nil || r.execute == nil {
		return nil, fmt.Errorf("resolve Worker template fields: execution service is unavailable")
	}
	return workerprompting.ResolveTemplateFields(
		workingDirectory,
		environment,
		tokens,
		workflowContext,
		worktree,
	)
}

// RuntimeOwnsModelEventRecording marks the process-scoped detached root. The
// Factory Runtime records model boundary events against its own ledger after a
// request completes; Workers itself remains independent of Factory Session
// recording state.
func (*Root) RuntimeOwnsModelEventRecording() bool { return true }
