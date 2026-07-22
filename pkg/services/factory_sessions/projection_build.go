package factorysessions

import (
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ProjectionBuildInput contains the canonical runtime facts needed to build
// one live Factory Session projection context.
type ProjectionBuildInput struct {
	Session             *LiveSession
	RuntimeConfig       interfaces.RuntimeConfigLookup
	Snapshot            *factory.StateSnapshot
	BackendScopeID      string
	LogicalSessionKey   string
	NormalizedTarget    *RuntimeLogicalTarget
	RuntimeStartedAt    time.Time
	CheckpointStore     factory.JavaScriptCheckpointStore
	Events              []interfaces.FactoryEvent
	WorldStateProjector factory.WorldStateProjector
	Now                 time.Time
}

// BuildProjectionContext combines runtime state, event projection, JavaScript
// checkpoints, and enabled transitions in one Session-owned implementation.
func BuildProjectionContext(input ProjectionBuildInput) (ProjectionContext, error) {
	if input.Now.IsZero() {
		return ProjectionContext{}, fmt.Errorf("Factory Session projection time is required")
	}
	factoryCfg := (*interfaces.FactoryConfig)(nil)
	if input.RuntimeConfig != nil {
		factoryCfg = input.RuntimeConfig.FactoryConfig()
	}
	result := ProjectionContext{
		Session: input.Session, FactoryCfg: factoryCfg, Snapshot: input.Snapshot,
		BackendScopeID: input.BackendScopeID, LogicalSessionKeyID: input.LogicalSessionKey,
		NormalizedTarget: input.NormalizedTarget, RuntimeStartedAt: input.RuntimeStartedAt,
		Now: input.Now,
	}
	if input.Snapshot != nil {
		result.LifecycleControlStatus = input.Snapshot.LifecycleControlStatus
	}
	if interfaces.IsJavaScriptOrchestratorFactory(factoryCfg) && input.CheckpointStore != nil {
		result.JavaScriptCheckpoints = input.CheckpointStore.List()
	}
	if input.Snapshot != nil && len(input.Events) > 0 {
		if input.WorldStateProjector == nil {
			return ProjectionContext{}, fmt.Errorf("Recordings world-state projector is required")
		}
		worldState, err := input.WorldStateProjector(input.Events, input.Snapshot.TickCount)
		if err != nil {
			return ProjectionContext{}, err
		}
		result.JavaScript = worldState.JavaScriptRuntime
		result.JavaScriptSession = worldState.SessionBracket
	}
	result.JavaScript = JavaScriptRuntimeStateFromCheckpoints(input.CheckpointStore, result.JavaScript)
	if input.Snapshot != nil {
		result.Enabled = append([]interfaces.EnabledTransition(nil), input.Snapshot.EnabledTransitions...)
	}
	return result, nil
}
