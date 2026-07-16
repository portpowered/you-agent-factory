package subsystems

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// NoOpDispatcherSubsystem is a synchronous dispatcher that auto-accepts all
// dispatches within a single tick. It fires transitions, consumes input tokens,
// immediately produces an ACCEPTED WorkResult with pass-through token colors,
// and enqueues results via a callback — all inline.
//
// This is intended for tests that need fire-and-forget dispatch without
// constructing a full raceSyncDispatcher + delayExecutor setup.
type NoOpDispatcherSubsystem struct {
	state        *state.Net
	sched        scheduler.Scheduler
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult]
}

// NewNoOpDispatcher creates a NoOpDispatcherSubsystem that auto-accepts all
// dispatches. Results are written to the provided typed buffer so the engine or
// test harness can drain them through the normal runtime-state path.
func NewNoOpDispatcher(n *state.Net, sched scheduler.Scheduler, resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult]) *NoOpDispatcherSubsystem {
	return &NoOpDispatcherSubsystem{
		state:        n,
		sched:        sched,
		resultBuffer: resultBuffer,
	}
}

var _ Subsystem = (*NoOpDispatcherSubsystem)(nil)

// TickGroup returns Dispatcher (5).
func (d *NoOpDispatcherSubsystem) TickGroup() TickGroup {
	return Dispatcher
}

// Execute finds enabled transitions, selects firings via the scheduler,
// consumes input tokens, and immediately enqueues an ACCEPTED result via the
// enqueueResult callback with pass-through token colors. No external executor
// is invoked.
func (d *NoOpDispatcherSubsystem) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	evaluator := scheduler.NewEnablementEvaluator(nil)
	enabled := evaluator.FindEnabledTransitions(ctx, d.state, &snapshot.Marking)
	if len(enabled) == 0 {
		return nil, nil
	}

	decisions := d.sched.Select(enabled, snapshot)
	if len(decisions) == 0 {
		return nil, nil
	}

	var mutations []interfaces.MarkingMutation
	var dispatchRecords []interfaces.DispatchRecord

	for _, decision := range decisions {
		// CONSUME input tokens.
		var consumeMutations []interfaces.MarkingMutation
		for _, tokenID := range decision.ConsumeTokens {
			tok, ok := snapshot.Marking.Tokens[tokenID]
			if !ok {
				continue
			}
			consumeMutations = append(consumeMutations, interfaces.MarkingMutation{
				Type:      interfaces.MutationConsume,
				TokenID:   tokenID,
				FromPlace: tok.PlaceID,
				Reason:    fmt.Sprintf("consumed by transition %s", decision.TransitionID),
			})
		}
		mutations = append(mutations, consumeMutations...)

		// Collect input tokens for the result.
		inputTokens := make([]factorytoken.Token, 0, len(decision.ConsumeTokens))
		for _, id := range decision.ConsumeTokens {
			if tok, ok := snapshot.Marking.Tokens[id]; ok {
				inputTokens = append(inputTokens, *tok)
			}
		}

		// Build the dispatch record pairing the dispatch with its consumed mutations.
		execution := executionMetadataForDispatch(decision.TransitionID, snapshot.TickCount, inputTokens)
		dispatch := work.WorkDispatch{
			DispatchID:               uuid.NewString(),
			TransitionID:             decision.TransitionID,
			WorkerType:               decision.WorkerType,
			CurrentChainingTraceID:   factorytoken.CurrentChainingTraceID(inputTokens, interfaces.SystemTimeWorkTypeID),
			PreviousChainingTraceIDs: factorytoken.PreviousChainingTraceIDs(inputTokens),
			Execution:                execution,
			InputTokens:              workers.InputTokens(inputTokens...),
		}
		dispatchRecords = append(dispatchRecords, interfaces.DispatchRecord{
			Dispatch:  dispatch,
			Mutations: consumeMutations,
		})

		// Build the result as if an executor returned ACCEPTED.
		result := workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: decision.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
		}

		if d.resultBuffer != nil {
			d.resultBuffer.Write(ctx, result)
		}
	}

	if len(mutations) == 0 {
		return nil, nil
	}

	return &interfaces.TickResult{Mutations: mutations, Dispatches: dispatchRecords}, nil
}
