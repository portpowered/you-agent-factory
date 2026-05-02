package petri

import (
	"strings"
	"time"

	factorythrottle "github.com/portpowered/infinite-you/pkg/factory/throttle"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type RuntimeGuardContext struct {
	Now               time.Time
	CurrentTransitionID string
	DispatchHistory   []interfaces.CompletedDispatch
	RuntimeConfig     interfaces.RuntimeDefinitionLookup
	TransitionWorkers map[string]string
}

type RuntimeGuard interface {
	Guard
	EvaluateRuntime(ctx RuntimeGuardContext, candidates []interfaces.Token, bindings map[string]*interfaces.Token, marking *MarkingSnapshot) (matched []interfaces.Token, ok bool)
}

type ActivePauseProvider interface {
	ActivePauses(ctx RuntimeGuardContext) []interfaces.ActiveThrottlePause
}

// InferenceThrottleGuard blocks a transition while throttle failures remain
// active for the authored provider/model lane derived from dispatch history.
type InferenceThrottleGuard struct {
	Provider             string
	Model                string
	WorkerName           string
	RefreshWindow        time.Duration
	WatchedTransitionIDs map[string]struct{}
}

var _ RuntimeGuard = (*InferenceThrottleGuard)(nil)
var _ ActivePauseProvider = (*InferenceThrottleGuard)(nil)

func (g *InferenceThrottleGuard) Evaluate(_ []interfaces.Token, _ map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	return nil, false
}

func (g *InferenceThrottleGuard) EvaluateRuntime(ctx RuntimeGuardContext, candidates []interfaces.Token, _ map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	if !g.appliesToCurrentTransition(ctx) {
		return candidates, len(candidates) > 0
	}
	if len(g.ActivePauses(ctx)) > 0 {
		return nil, false
	}
	return candidates, len(candidates) > 0
}

func (g *InferenceThrottleGuard) ActivePauses(ctx RuntimeGuardContext) []interfaces.ActiveThrottlePause {
	if g == nil || g.Provider == "" || g.RefreshWindow <= 0 || ctx.Now.IsZero() {
		return nil
	}
	history := make([]factorythrottle.FailureRecord, 0, len(ctx.DispatchHistory))
	for i := range ctx.DispatchHistory {
		record := ctx.DispatchHistory[i]
		if record.ProviderFailure == nil || record.ProviderFailure.Family != interfaces.ProviderErrorFamilyThrottle {
			continue
		}
		if !g.historyDispatchMatchesLane(ctx, record.TransitionID) {
			continue
		}
		history = append(history, factorythrottle.FailureRecord{
			Provider:        g.Provider,
			Model:           g.Model,
			OccurredAt:      record.EndTime,
			ProviderFailure: record.ProviderFailure,
		})
	}
	return factorythrottle.DeriveActiveThrottlePauses(history, g.RefreshWindow, ctx.Now)
}

func (g *InferenceThrottleGuard) appliesToCurrentTransition(ctx RuntimeGuardContext) bool {
	if g == nil {
		return false
	}
	if ctx.CurrentTransitionID != "" && len(g.WatchedTransitionIDs) > 0 {
		_, ok := g.WatchedTransitionIDs[ctx.CurrentTransitionID]
		return ok
	}
	if g.WorkerName == "" || ctx.RuntimeConfig == nil {
		return true
	}
	worker, ok := ctx.RuntimeConfig.Worker(g.WorkerName)
	if !ok || worker == nil {
		return false
	}
	return strings.EqualFold(worker.ModelProvider, g.Provider) && (g.Model == "" || worker.Model == g.Model)
}

func (g *InferenceThrottleGuard) historyDispatchMatchesLane(ctx RuntimeGuardContext, transitionID string) bool {
	if g == nil {
		return false
	}
	if ctx.RuntimeConfig != nil && len(ctx.TransitionWorkers) > 0 {
		workerName, ok := ctx.TransitionWorkers[transitionID]
		if ok && workerName != "" {
			worker, ok := ctx.RuntimeConfig.Worker(workerName)
			if ok && worker != nil {
				return strings.EqualFold(worker.ModelProvider, g.Provider) && (g.Model == "" || worker.Model == g.Model)
			}
		}
	}
	if len(g.WatchedTransitionIDs) == 0 {
		return false
	}
	_, ok := g.WatchedTransitionIDs[transitionID]
	return ok
}
