package factory

import (
	"context"
	"errors"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type CompletionRecorder func(interfaces.FactoryCompletionRecord)
type PetriMutationRecorder func(sessionID string, mutations []interfaces.TokenMutationRecord) error
type FactoryEventRecorder func(interfaces.FactoryEvent)
// RuntimeLedgerFactory remains a compatibility seam until the Factory
// Sessions opening cutover removes the legacy factories in the next stack
// slice.
type RuntimeLedgerFactory func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger

type SubmissionHook interface {
	Name() string
	Priority() int
	OnTick(context.Context, interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error)
}

type DispatchResultHook interface {
	SubmitDispatch(context.Context, work.WorkDispatch) error
	OnTick(context.Context, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]workerexecution.WorkResult, error)
	WaitCh() <-chan struct{}
	HasPendingResults() bool
}

type DispatchResultHookWakeSignaler interface {
	HasBufferedResults() bool
	SignalBufferedResults()
}

type CompletionDeliveryPlanner = recordings.CompletionDeliveryPlanner
type ReplayWorkerSessionIDResolver = recordings.ReplayWorkerSessionIDResolver

// ExecutionCatalogSelection is the Runtime-owned pair selected from a
// Definitions catalog. It contains detached values only; mapping it does not
// construct or query Workers execution machinery.
type ExecutionCatalogSelection struct {
	Worker      interfaces.ResolvedWorkerDefinition
	Workstation interfaces.ResolvedWorkstationDefinition
}

// ExecutionCatalogMappingError reports a malformed detached selection before
// it reaches Workers.
type ExecutionCatalogMappingError struct {
	Path    string
	Message string
}

func (e *ExecutionCatalogMappingError) Error() string {
	if e == nil {
		return "invalid execution catalog selection"
	}
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func (e *ExecutionCatalogMappingError) Unwrap() error {
	return ErrInvalidExecutionCatalogSelection
}

// ErrInvalidExecutionCatalogSelection identifies a malformed detached
// Definition-to-Workers mapping request.
var ErrInvalidExecutionCatalogSelection = errors.New("invalid execution catalog selection")

// ExecutionCatalogMapper is the stateless Runtime boundary that converts a
// detached Definitions selection into a Workers-owned policy value.
type ExecutionCatalogMapper struct{}

// MapExecutionCatalogSelection maps one detached Definitions pair to the
// Workers-owned policy value boundary. It is intentionally pure and returns a
// fresh value on every call.
func (ExecutionCatalogMapper) MapExecutionCatalogSelection(
	selection ExecutionCatalogSelection,
) (workerexecution.ResolvedExecutionPolicy, error) {
	if err := validateExecutionCatalogSelection(selection); err != nil {
		return workerexecution.ResolvedExecutionPolicy{}, err
	}
	return mapExecutionCatalogSelection(selection), nil
}

func validateExecutionCatalogSelection(selection ExecutionCatalogSelection) error {
	worker := selection.Worker
	workstation := selection.Workstation
	switch {
	case strings.TrimSpace(workstation.Name) == "":
		return &ExecutionCatalogMappingError{
			Path:    "workstation.name",
			Message: "workstation name is required",
		}
	case strings.TrimSpace(workstation.Runner) == "":
		return &ExecutionCatalogMappingError{
			Path:    "workstation.runner",
			Message: "runner selection is required",
		}
	case workstation.WorkerName != "" && worker.Name != workstation.WorkerName:
		return &ExecutionCatalogMappingError{
			Path:    "workstation.worker",
			Message: "workstation worker does not match selected worker",
		}
	case workstation.WorkerName != "" && strings.TrimSpace(worker.Name) == "":
		return &ExecutionCatalogMappingError{
			Path:    "worker.name",
			Message: "selected worker name is required",
		}
	case workstation.Timeout < 0 || worker.Timeout < 0:
		return &ExecutionCatalogMappingError{
			Path:    "timeout",
			Message: "timeout cannot be negative",
		}
	default:
		return nil
	}
}

func mapExecutionCatalogSelection(
	selection ExecutionCatalogSelection,
) workerexecution.ResolvedExecutionPolicy {
	worker := selection.Worker
	workstation := selection.Workstation
	policy := workerexecution.ResolvedExecutionPolicy{
		WorkerName:                  worker.Name,
		WorkerType:                  worker.Type,
		WorkstationName:             workstation.Name,
		WorkstationType:             workstation.Type,
		RunnerID:                    workstation.Runner,
		RunnerSelectionSource:       workerexecution.RunnerSelectionSource(workstation.RunnerSelectionSource),
		Provider:                    worker.Provider,
		Model:                       worker.Model,
		ModelProvider:               worker.ModelProvider,
		ModelLocality:               worker.ModelLocality,
		ReasoningEffort:             worker.ReasoningEffort,
		ExecutorProvider:            worker.ExecutorProvider,
		Command:                     worker.Command,
		Args:                        append([]string(nil), worker.Args...),
		StopToken:                   worker.StopToken,
		AgentToolPolicy:             worker.AgentToolPolicy,
		SkipPermissions:             worker.SkipPermissions,
		PromptFile:                  workstation.PromptFile,
		Prompt:                      workstation.Body,
		PromptTemplate:              workstation.PromptTemplate,
		OutputSchema:                workstation.OutputSchema,
		OutputContract:              workstation.OutputContract,
		OutputFormat:                workstation.OutputFormat,
		DecisionEnvelope:            workstation.DecisionEnvelope,
		GoalRoutingDecisionEnvelope: workstation.GoalRoutingDecisionEnvelope,
		FormatInvocationSummary:     workstation.FormatInvocationSummary,
		FormatInvocationResponse:    workstation.FormatInvocationResponse,
		FormatTTSMetadata:           workstation.FormatTTSMetadata,
		Environment:                 cloneExecutionEnvironment(workstation.Environment),
		WorkingDirectory:            workstation.WorkingDirectory,
		Worktree:                    workstation.Worktree,
		Timeout:                     workstation.Timeout,
		WorkPropagation:             string(workstation.WorkPropagation),
		Operation:                   workstation.Operation,
		OperationBindings:           mapExecutionOperationBindings(workstation.OperationBindings),
		StopWords:                   append([]string(nil), workstation.StopWords...),
		RuntimeStopWords:            append([]string(nil), workstation.RuntimeStopWords...),
	}
	if policy.Prompt == "" {
		policy.Prompt = worker.Body
	}
	if policy.PromptFile == "" {
		policy.PromptFile = worker.PromptSourcePath
	}
	if policy.Timeout == 0 {
		policy.Timeout = worker.Timeout
	}
	return policy
}

// MapExecutionCatalogEntry is the compact two-value form used by Runtime
// call-sites that have already selected the worker and workstation entries.
func (ExecutionCatalogMapper) MapExecutionCatalogEntry(
	worker interfaces.ResolvedWorkerDefinition,
	workstation interfaces.ResolvedWorkstationDefinition,
) (workerexecution.ResolvedExecutionPolicy, error) {
	return ExecutionCatalogMapper{}.MapExecutionCatalogSelection(ExecutionCatalogSelection{
		Worker:      worker,
		Workstation: workstation,
	})
}

// MapResolvedExecutionCatalogEntry is an explicit alias for callers that want
// the resolved-value terminology at the Runtime boundary.
func (ExecutionCatalogMapper) MapResolvedExecutionCatalogEntry(
	worker interfaces.ResolvedWorkerDefinition,
	workstation interfaces.ResolvedWorkstationDefinition,
) (workerexecution.ResolvedExecutionPolicy, error) {
	return ExecutionCatalogMapper{}.MapExecutionCatalogEntry(worker, workstation)
}

func cloneExecutionEnvironment(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func mapExecutionOperationBindings(
	values []interfaces.ResolvedModelOperationBinding,
) []workerexecution.ResolvedExecutionPolicyBinding {
	if len(values) == 0 {
		return nil
	}
	mapped := make([]workerexecution.ResolvedExecutionPolicyBinding, len(values))
	for index, value := range values {
		mapped[index] = workerexecution.ResolvedExecutionPolicyBinding{
			Slot:           value.Slot,
			Config:         work.CloneWorkContentParts(value.Config),
			DefaultContent: work.CloneWorkContentParts(value.DefaultContent),
		}
		if value.Selector != nil {
			mapped[index].SelectorSlot = value.Selector.Slot
			mapped[index].SelectorLabel = value.Selector.Label
			mapped[index].SelectorType = value.Selector.Type
			mapped[index].SelectorRole = value.Selector.Role
		}
	}
	return mapped
}

// DispatchPlanOutcome is the plain success vocabulary for Factory Runtime root
// dispatch-plan operations. Peers branch on these values without Petri
// transition objects or Workers construction types. DUPLICATE_IDEMPOTENT is the
// published idempotent-duplicate handling vocabulary for accepted re-plans and
// re-accepts.
type DispatchPlanOutcome string

const (
	// DispatchPlanOutcomeAccepted indicates a new dispatch intent was planned
	// or a correlated result was accepted for the first time.
	DispatchPlanOutcomeAccepted DispatchPlanOutcome = "ACCEPTED"
	// DispatchPlanOutcomeRetired indicates a correlated worker result was
	// accepted and the dispatch intent was retired from the outbox.
	DispatchPlanOutcomeRetired DispatchPlanOutcome = "RETIRED"
	// DispatchPlanOutcomeDuplicateIdempotent indicates the same intent or
	// correlated result was already applied and the re-delivery was handled
	// idempotently without changing outbox state.
	DispatchPlanOutcomeDuplicateIdempotent DispatchPlanOutcome = "DUPLICATE_IDEMPOTENT"
)

// DispatchResultOutcome is the plain correlated worker-result vocabulary peers
// submit when accepting or retiring a planned dispatch. It must not carry Petri
// token payloads or Workers implementation types.
type DispatchResultOutcome string

const (
	// DispatchResultOutcomeSuccess indicates the worker completed successfully.
	DispatchResultOutcomeSuccess DispatchResultOutcome = "SUCCESS"
	// DispatchResultOutcomeFailure indicates the worker completed with failure.
	DispatchResultOutcomeFailure DispatchResultOutcome = "FAILURE"
	// DispatchResultOutcomeCancelled indicates the worker result was cancelled.
	DispatchResultOutcomeCancelled DispatchResultOutcome = "CANCELLED"
)

// PlanDispatchRequest is the plain publish/plan dispatch-intent input published
// at the Runtime root. Runtime owns planning/outbox vocabulary; Workers remains
// the execution owner.
type PlanDispatchRequest struct {
	DispatchID      string
	CorrelationID   string
	WorkIDs         []string
	WorkstationName string
	WorkerType      string
	ReplayKey       string
}

// PlanDispatchResult is the plain publish/plan success shape published at the
// Runtime root.
type PlanDispatchResult struct {
	Outcome       DispatchPlanOutcome
	DispatchID    string
	CorrelationID string
}

// AcceptDispatchResultRequest is the plain accept/retire correlated worker
// result input published at the Runtime root.
type AcceptDispatchResultRequest struct {
	DispatchID    string
	CorrelationID string
	WorkID        string
	ResultOutcome DispatchResultOutcome
}

// AcceptDispatchResultResult is the plain accept/retire success shape published
// at the Runtime root.
type AcceptDispatchResultResult struct {
	Outcome       DispatchPlanOutcome
	DispatchID    string
	CorrelationID string
}
