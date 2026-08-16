package internal

import (
	"context"
	"fmt"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/prompting"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

// ExecuteCapability is the request-scoped Execute owner composed into the
// Workers root. Legacy runtime and pool capabilities remain available during
// the stateless execution migration, but they do not own this operation.
type ExecuteCapability interface {
	Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

type promptRenderer interface {
	RenderPrompt(string, []workers.Token, *workers.Context) (string, error)
}

// Root is the inert Workers root composed from parent-private runtime assembly
// and workstation owners. It starts no lifecycle, runner execution, or
// workstation pool admission.
type Root struct {
	runtimeAssembly runtimeassembly.Service
	workstations    workstations.Service
	execute         ExecuteCapability
}

var _ workers.Service = (*Root)(nil)

var _ workers.PromptTemplates = (*Root)(nil)

// NewRoot constructs the inert Workers root from parent-private runtime
// assembly and workstation owners.
func NewRoot(
	runtimeAssembly runtimeassembly.Service,
	workstationsOwner workstations.Service,
	execute ...ExecuteCapability,
) (workers.Service, error) {
	if runtimeAssembly == nil {
		return nil, fmt.Errorf("construct Workers: runtime assembly owner is required")
	}
	if workstationsOwner == nil {
		return nil, fmt.Errorf("construct Workers: workstations owner is required")
	}
	var executeOwner ExecuteCapability
	if len(execute) > 0 {
		executeOwner = execute[0]
	}
	return &Root{
		runtimeAssembly: runtimeAssembly,
		workstations:    workstationsOwner,
		execute:         executeOwner,
	}, nil
}

// RootFrom composes a Workers root value from parent-private owners for
// owner-local runtime construction and testing.
func RootFrom(
	runtimeAssembly runtimeassembly.Service,
	workstationsOwner workstations.Service,
) Root {
	return Root{
		runtimeAssembly: runtimeAssembly,
		workstations:    workstationsOwner,
	}
}

// ReplaceRuntimeAssembly returns a copy with an updated runtime assembly owner.
func (r Root) ReplaceRuntimeAssembly(runtimeAssembly runtimeassembly.Service) Root {
	r.runtimeAssembly = runtimeAssembly
	return r
}

// ReplaceWorkstations returns a copy with an updated workstation owner.
func (r Root) ReplaceWorkstations(workstationsOwner workstations.Service) Root {
	r.workstations = workstationsOwner
	return r
}

// ReplaceExecute returns a copy with an updated request-scoped Execute owner.
func (r Root) ReplaceExecute(execute ExecuteCapability) Root {
	r.execute = execute
	return r
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

// BuildRuntime delegates the singular Workers root operation to its
// parent-private Runtime Assembly capability.
func (r *Root) BuildRuntime(
	ctx context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	if r == nil || r.runtimeAssembly == nil {
		return workers.RuntimeBuildResult{}, fmt.Errorf(
			"%w: Workers Runtime Assembly is required",
			workers.ErrIncompleteRuntimeAssembly,
		)
	}
	return r.runtimeAssembly.Build(ctx, request)
}

// StartWorkstationPool delegates lifecycle activation to the parent-private
// workstation capability.
func (r *Root) StartWorkstationPool(
	ctx context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationPoolStartResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Start(ctx, request)
}

// StopWorkstationPool delegates terminal shutdown to the parent-private
// workstation capability.
func (r *Root) StopWorkstationPool(
	ctx context.Context,
) (workers.WorkstationPoolStopResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationPoolStopResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Stop(ctx)
}

// WorkstationRoute reports availability through the private lifecycle owner.
func (r *Root) WorkstationRoute(
	ctx context.Context,
	request workers.WorkstationRouteRequest,
) (workers.WorkstationRouteResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationRouteResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Route(ctx, request)
}

// DispatchWorkstation delegates execution to the private workstation owner.
func (r *Root) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Dispatch(ctx, request)
}

// DispatchWorkstationWithAdmission delegates execution and exposes the exact
// admission point owned by the private workstation capability.
func (r *Root) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.DispatchWithAdmission(ctx, request, admitted)
}

// CancelWorkstationDispatch delegates explicit cancellation to the private
// workstation owner.
func (r *Root) CancelWorkstationDispatch(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if r == nil || r.workstations == nil {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return r.workstations.Cancel(ctx, request)
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
