package factory

import (
	"context"
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
