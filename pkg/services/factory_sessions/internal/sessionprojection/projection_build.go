package sessionprojection

import (
	"fmt"
	"sort"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

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
		Session: projectLiveSession(input.Session), FactorySessionID: livesession.CanonicalID(input.Session),
		FactoryCfg: factoryCfg, Snapshot: input.Snapshot, Observation: input.Observation,
		BackendScopeID: input.BackendScopeID, LogicalSessionKeyID: input.LogicalSessionKey,
		NormalizedTarget: input.NormalizedTarget, RuntimeStartedAt: input.RuntimeStartedAt,
		Now: input.Now,
	}
	if input.Observation.Health.LifecycleControlStatus != "" {
		result.LifecycleControlStatus = input.Observation.Health.LifecycleControlStatus
	} else if input.Snapshot != nil {
		result.LifecycleControlStatus = input.Snapshot.LifecycleControlStatus
	}
	if interfaces.IsJavaScriptOrchestratorFactory(factoryCfg) && input.CheckpointStore != nil {
		result.JavaScriptCheckpoints = input.CheckpointStore.List()
	}
	if len(input.Events) > 0 {
		if input.WorldStateProjector == nil {
			return ProjectionContext{}, fmt.Errorf("Recordings world-state projector is required")
		}
		worldState, err := input.WorldStateProjector(input.Events, input.Observation.Progress.TickCount)
		if err != nil {
			return ProjectionContext{}, err
		}
		result.JavaScript = worldState.JavaScriptRuntime
		result.JavaScriptSession = worldState.SessionBracket
		result.PendingHumanApprovals = pendingHumanApprovals(worldState.PendingHumanApprovalsByID)
	}
	result.JavaScript = JavaScriptRuntimeStateFromCheckpoints(input.CheckpointStore, result.JavaScript)
	if input.Snapshot != nil {
		result.Enabled = append([]interfaces.EnabledTransition(nil), input.Snapshot.EnabledTransitions...)
	}
	return result, nil
}

func pendingHumanApprovals(values map[string]interfaces.FactoryWorldHumanApproval) []interfaces.FactoryWorldHumanApproval {
	if len(values) == 0 {
		return nil
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]interfaces.FactoryWorldHumanApproval, 0, len(ids))
	for _, id := range ids {
		approval := values[id]
		approval.WorkItemIDs = append([]string(nil), approval.WorkItemIDs...)
		approval.TraceIDs = append([]string(nil), approval.TraceIDs...)
		approval.Decisions = append([]interfaces.HumanApprovalDecision(nil), approval.Decisions...)
		if approval.WorkstationDescription != nil {
			description := *approval.WorkstationDescription
			description.Locales = append([]string(nil), approval.WorkstationDescription.Locales...)
			if approval.WorkstationDescription.Values != nil {
				description.Values = make(map[string]string, len(approval.WorkstationDescription.Values))
				for locale, text := range approval.WorkstationDescription.Values {
					description.Values[locale] = text
				}
			}
			approval.WorkstationDescription = &description
		}
		result = append(result, approval)
	}
	return result
}

func projectLiveSession(session *livesession.LiveSession) *factorysessions.ScopedLiveSessionSummary {
	if session == nil {
		return nil
	}
	return &factorysessions.ScopedLiveSessionSummary{
		ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath, Project: session.Project,
		IsDefault: session.IsDefault, Target: session.Target,
	}
}
