package factorysessions

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// EnabledTransitionsForSnapshot evaluates Petri enablement for one engine snapshot.
func EnabledTransitionsForSnapshot(
	ctx context.Context,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	runtimeCfg interfaces.RuntimeDefinitionLookup,
) []interfaces.EnabledTransition {
	if snapshot == nil || snapshot.Topology == nil {
		return nil
	}
	evaluator := scheduler.NewEnablementEvaluator(
		nil,
		scheduler.WithEnablementRuntimeConfig(runtimeCfg),
	)
	return evaluator.FindEnabledTransitions(ctx, snapshot.Topology, &snapshot.Marking)
}
