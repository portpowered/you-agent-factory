package runtime

import (
	"context"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var _ factory.Service = (*factoryImpl)(nil)

// GetEngineStateSnapshot returns the aggregate observability snapshot for
// service-facing callers.
func (f *factoryImpl) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	snapshotStartedAt := f.clock.Now()
	runtimeSnap := f.engine.GetRuntimeStateSnapshot()
	engineSnapshotDuration := f.clock.Now().Sub(snapshotStartedAt)
	runtimeSnap.StreamGenerationID = f.eventHistory.StreamGenerationID()

	f.mu.RLock()
	currentState := f.state
	startedAt := f.startedAt
	now := f.clock.Now()
	f.mu.RUnlock()

	worldStateStartedAt := f.clock.Now()
	worldState := f.currentWorldState(runtimeSnap.TickCount)
	worldStateDuration := f.clock.Now().Sub(worldStateStartedAt)
	runtimeSnap.RuntimeStatus = f.deriveRuntimeStatus(currentState, runtimeSnap, worldState)
	uptime := time.Duration(0)
	if !startedAt.IsZero() {
		uptime = now.Sub(startedAt)
	}

	snap := state.NewEngineStateSnapshot(runtimeSnap, string(currentState), uptime, f.topology)
	snap.LifecycleControlStatus = lifecycleControlStatusFromWorldState(worldState, string(currentState))
	enablementStartedAt := f.clock.Now()
	snap.EnabledTransitions = scheduler.NewEnablementEvaluator(
		f.logger,
		f.clock.Now,
		f.cfg.runtimeConfig,
	).FindEnabledTransitionsWithSnapshot(ctx, f.topology, &snap)
	f.logger.Debug(
		"factory runtime state snapshot phases",
		"engine_snapshot_duration_ms", engineSnapshotDuration.Milliseconds(),
		"world_state_duration_ms", worldStateDuration.Milliseconds(),
		"enablement_duration_ms", f.clock.Now().Sub(enablementStartedAt).Milliseconds(),
		"total_duration_ms", f.clock.Now().Sub(snapshotStartedAt).Milliseconds(),
		"token_count", len(runtimeSnap.Marking.Tokens),
		"dispatch_count", len(runtimeSnap.Dispatches),
		"in_flight_count", runtimeSnap.InFlightCount,
		"result_count", len(runtimeSnap.Results),
		"dispatch_history_count", len(runtimeSnap.DispatchHistory),
		"active_throttle_pause_count", len(runtimeSnap.ActiveThrottlePauses),
		"enabled_transition_count", len(snap.EnabledTransitions),
	)
	return &snap, nil
}

// CleanInvocationSnapshot projects the engine-owned invocation facts into the
// narrow transport contract. The raw snapshot remains entirely inside the
// Factory Runtime implementation package.
func (f *factoryImpl) CleanInvocationSnapshot(ctx context.Context) (factory.CleanInvocationSnapshot, error) {
	return cleanInvocationSnapshot(ctx, f.GetEngineStateSnapshot)
}

type cleanInvocationSnapshotReader func(
	context.Context,
) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)

func cleanInvocationSnapshot(
	ctx context.Context,
	read cleanInvocationSnapshotReader,
) (factory.CleanInvocationSnapshot, error) {
	snapshot, err := read(ctx)
	if err != nil {
		return factory.CleanInvocationSnapshot{}, err
	}
	return projectCleanInvocationSnapshot(snapshot), nil
}

func projectCleanInvocationSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) factory.CleanInvocationSnapshot {
	if snapshot == nil {
		return factory.CleanInvocationSnapshot{}
	}
	result := factory.CleanInvocationSnapshot{
		Work:            make([]factory.CleanInvocationWork, 0, len(snapshot.Marking.Tokens)),
		DispatchHistory: make([]factory.CleanInvocationDispatch, 0, len(snapshot.DispatchHistory)),
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		result.Work = append(result.Work, cleanInvocationWorkFromToken(snapshot.Topology, token))
	}
	for _, completion := range snapshot.DispatchHistory {
		projected := factory.CleanInvocationDispatch{
			Outcome: completionOutcome(completion.Outcome),
			Reason:  completion.Reason,
		}
		if completion.FailureMetadata != nil {
			projected.FailureType = string(completion.FailureMetadata.Type)
		}
		projected.Consumed = make([]factory.CleanInvocationWork, 0, len(completion.ConsumedTokens))
		for index := range completion.ConsumedTokens {
			projected.Consumed = append(projected.Consumed, cleanInvocationWorkFromToken(snapshot.Topology, &completion.ConsumedTokens[index]))
		}
		projected.Outputs = make([]factory.CleanInvocationWork, 0, len(completion.OutputMutations))
		for _, mutation := range completion.OutputMutations {
			if mutation.Token == nil {
				continue
			}
			projected.Outputs = append(projected.Outputs, cleanInvocationWorkFromToken(snapshot.Topology, mutation.Token))
		}
		result.DispatchHistory = append(result.DispatchHistory, projected)
	}
	return result
}

func cleanInvocationWorkFromToken(topology *state.Net, token *factorytoken.Token) factory.CleanInvocationWork {
	if token == nil {
		return factory.CleanInvocationWork{}
	}
	category := factory.StateCategoryProcessing
	if topology != nil {
		category = topology.StateCategoryForPlace(token.PlaceID)
	}
	return factory.CleanInvocationWork{
		WorkID:        token.Color.WorkID,
		WorkTypeID:    token.Color.WorkTypeID,
		StateCategory: string(category),
		Output:        string(token.Color.Payload),
		TraceID:       token.Color.TraceID,
		DataType:      string(token.Color.DataType),
	}
}

func completionOutcome(outcome workerexecution.WorkOutcome) string {
	return string(outcome)
}
