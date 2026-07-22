package factory

import (
	"context"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type CompletionRecorder func(interfaces.FactoryCompletionRecord)

type PetriMutationRecorder func(sessionID string, mutations []interfaces.TokenMutationRecord) error

type FactoryEventRecorder func(interfaces.FactoryEvent)

// RuntimeLedgerFactory constructs the canonical event ledger for one runtime.
// Recordings supplies the implementation through Wire.
type RuntimeLedgerFactory func(
	recordings.InitialStructureSource,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger

type SubmissionHook interface {
	Name() string
	Priority() int
	OnTick(
		ctx context.Context,
		input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]],
	) (interfaces.SubmissionHookResult, error)
}

type DispatchResultHook interface {
	SubmitDispatch(ctx context.Context, dispatch work.WorkDispatch) error
	OnTick(
		ctx context.Context,
		snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	) ([]workerexecution.WorkResult, error)
	WaitCh() <-chan struct{}
	HasPendingResults() bool
}

type DispatchResultHookWakeSignaler interface {
	HasBufferedResults() bool
	SignalBufferedResults()
}

type CompletionDeliveryPlanner = recordings.CompletionDeliveryPlanner
