package scheduler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// EnablementEvaluator wraps transition enablement logic with structured logging.
// When a logger is provided, each transition evaluation emits log output showing
// the transition ID, whether it was enabled or disabled, and the reason for
// disablement.
type EnablementEvaluator struct {
	logger        logging.Logger
	now           func() time.Time
	runtimeConfig interfaces.RuntimeDefinitionLookup
}

// NewEnablementEvaluator creates an EnablementEvaluator with the given logger.
// If logger is nil, logging is a no-op.
func NewEnablementEvaluator(
	logger logging.Logger,
	now func() time.Time,
	runtimeConfig interfaces.RuntimeDefinitionLookup,
) *EnablementEvaluator {
	if now == nil {
		panic("Factory Runtime scheduler clock is required")
	}
	return &EnablementEvaluator{
		logger:        logging.EnsureLogger(logger),
		now:           now,
		runtimeConfig: runtimeConfig,
	}
}

// FindEnabledTransitions identifies all transitions whose input arcs are satisfied
// in the current marking. Each transition evaluation is logged with its result.
func (e *EnablementEvaluator) FindEnabledTransitions(ctx context.Context, n *state.Net, marking *petri.MarkingSnapshot) []interfaces.EnabledTransition {
	return e.FindEnabledTransitionsWithSnapshot(ctx, n, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking:  *marking,
		Topology: n,
	})
}

// FindEnabledTransitionsWithSnapshot evaluates transitions with access to the
// broader runtime snapshot for guards that depend on dispatch history or time.
func (e *EnablementEvaluator) FindEnabledTransitionsWithSnapshot(ctx context.Context, n *state.Net, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.EnabledTransition {
	var enabled []interfaces.EnabledTransition
	if snapshot == nil {
		return enabled
	}

	transitions := sortedTransitions(n.Transitions)
	for _, tr := range transitions {
		if et, ok := e.checkTransitionEnabled(ctx, tr, snapshot); ok {
			e.logger.Info("enablement: transition enabled",
				"transitionID", tr.ID,
				"transitionName", tr.Name,
				"workerType", tr.WorkerType,
				"bindingCount", len(et.Bindings))
			enabled = append(enabled, et)
		}
	}

	e.logger.Debug("enablement: evaluation complete",
		"totalTransitions", len(n.Transitions),
		"enabledCount", len(enabled))

	return enabled
}

// checkTransitionEnabled evaluates a single transition and logs the reason if disabled.
// portos:func-length-exception owner=agent-factory reason=legacy-enable-evaluation-loop review=2026-07-18 removal=split-binding-phases-before-next-scheduler-expansion
func (e *EnablementEvaluator) checkTransitionEnabled(_ context.Context, tr *petri.Transition, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (interfaces.EnabledTransition, bool) {
	marking := &snapshot.Marking
	if len(tr.InputArcs) == 0 {
		e.logger.Debug("enablement: transition disabled",
			"transitionID", tr.ID,
			"transitionName", tr.Name,
			"reason", "no input arcs")
		return interfaces.EnabledTransition{}, false
	}

	if et, enabled, handled := e.checkSingleTokenGuardedTransition(tr, snapshot); handled {
		return et, enabled
	}

	// Separate unguarded and guarded arcs.
	var unguarded, guarded []int
	for i := range tr.InputArcs {
		if tr.InputArcs[i].Guard == nil {
			unguarded = append(unguarded, i)
		} else {
			guarded = append(guarded, i)
		}
	}

	guardBindings := make(map[string]*factorytoken.Token)
	result := make(map[string][]factorytoken.Token)
	arcModes := make(map[string]interfaces.ArcMode)

	// Phase 1: evaluate unguarded arcs to build bindings.
	for _, idx := range unguarded {
		arc := &tr.InputArcs[idx]
		candidates := stableTokens(marking.TokensInPlace(arc.PlaceID))
		matched := ApplyCardinality(candidates, arc.Cardinality)
		if matched == nil {
			e.logger.Debug("enablement: transition disabled",
				"transitionID", tr.ID,
				"transitionName", tr.Name,
				"reason", fmt.Sprintf("insufficient tokens for unguarded arc %q (place %s, cardinality %d, candidates %d)",
					arcKey(arc), arc.PlaceID, arc.Cardinality.Mode, len(candidates)))
			return interfaces.EnabledTransition{}, false
		}
		key := arcKey(arc)
		result[key] = matched
		arcModes[key] = arc.Mode
		if len(matched) > 0 {
			guardBindings[key] = &matched[0]
		}
	}

	// Phase 2: evaluate guarded arcs that do not require peer bindings first.
	var guardedPeerBinding []int
	for _, idx := range guarded {
		arc := &tr.InputArcs[idx]
		if guardRequiresPeerBinding(arc.Guard) {
			guardedPeerBinding = append(guardedPeerBinding, idx)
			continue
		}
		if !e.evaluateGuardedArc(tr, snapshot, arc, marking, guardBindings, result, arcModes) {
			return interfaces.EnabledTransition{}, false
		}
	}

	// Phase 3: evaluate peer-binding guards after their match arcs are bound.
	for _, idx := range guardedPeerBinding {
		arc := &tr.InputArcs[idx]
		if !e.evaluateGuardedArc(tr, snapshot, arc, marking, guardBindings, result, arcModes) {
			return interfaces.EnabledTransition{}, false
		}
	}
	if !e.bindingDependenciesMet(tr, snapshot, result) {
		e.logger.Debug("enablement: transition disabled",
			"transitionID", tr.ID,
			"transitionName", tr.Name,
			"reason", "dependency guard failed for selected binding")
		return interfaces.EnabledTransition{}, false
	}

	return interfaces.EnabledTransition{
		TransitionID: tr.ID,
		WorkerType:   tr.WorkerType,
		Bindings:     workerBindings(result),
		ArcModes:     arcModes,
	}, true
}

func (e *EnablementEvaluator) checkSingleTokenGuardedTransition(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) (interfaces.EnabledTransition, bool, bool) {
	if !singleTokenGuardedTransition(tr) {
		return interfaces.EnabledTransition{}, false, false
	}
	if et, ok := e.findSingleTokenBindingTransition(tr, snapshot); ok {
		return et, true, true
	}
	if !shouldFailClosedSameNameJoin(tr, snapshot) {
		return interfaces.EnabledTransition{}, false, false
	}
	// The backtracking evaluator is the only binding path that can honor peer
	// guards whose authored guard lives on a different input arc. The legacy
	// phased fallback would bind an arbitrary unguarded peer first, allowing a
	// historical same-name child when the canonical current child is in another
	// state. Fail closed only when the canonical registration projection proves
	// that the current registered child is elsewhere.
	e.logger.Debug("enablement: transition disabled",
		"transitionID", tr.ID,
		"transitionName", tr.Name,
		"reason", "guard failed for registered single-token binding")
	return interfaces.EnabledTransition{}, false, true
}

func (e *EnablementEvaluator) evaluateGuardedArc(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	arc *petri.Arc,
	marking *petri.MarkingSnapshot,
	guardBindings map[string]*factorytoken.Token,
	result map[string][]factorytoken.Token,
	arcModes map[string]interfaces.ArcMode,
) bool {
	candidates := stableTokens(marking.TokensInPlace(arc.PlaceID))
	guardMatched, ok := e.evaluateGuard(arc.Guard, petri.RuntimeGuardContext{
		Now:                      e.now(),
		CurrentTransitionID:      tr.ID,
		DispatchHistory:          snapshot.DispatchHistory,
		ActiveDispatches:         snapshot.Dispatches,
		RuntimeConfig:            e.runtimeConfig,
		TransitionWorkers:        transitionWorkerTypes(snapshot.Topology, tr),
		StateCategoryForPlace:    stateCategoryForPlace(snapshot.Topology),
		ParentChildRegistrations: marking.ParentChildRegistrations,
	}, candidates, guardBindings, marking)
	if !ok {
		e.logger.Debug("enablement: transition disabled",
			"transitionID", tr.ID,
			"transitionName", tr.Name,
			"reason", fmt.Sprintf("guard failed for arc %q (place %s, candidates %d)",
				arcKey(arc), arc.PlaceID, len(candidates)))
		return false
	}
	matched := ApplyCardinality(stableTokens(guardMatched), arc.Cardinality)
	if matched == nil {
		e.logger.Debug("enablement: transition disabled",
			"transitionID", tr.ID,
			"transitionName", tr.Name,
			"reason", fmt.Sprintf("insufficient tokens after guard for arc %q (place %s, cardinality %d, matched %d)",
				arcKey(arc), arc.PlaceID, arc.Cardinality.Mode, len(guardMatched)))
		return false
	}
	key := arcKey(arc)
	result[key] = matched
	arcModes[key] = arc.Mode
	if len(matched) > 0 {
		guardBindings[key] = &matched[0]
	}
	return true
}

func guardRequiresPeerBinding(guard petri.Guard) bool {
	if guard == nil {
		return false
	}
	switch typed := guard.(type) {
	case *petri.SameNameGuard, *petri.SameTraceIDGuard:
		return true
	case *petri.MatchesFieldsGuard:
		return typed.MatchBinding != ""
	case *petri.AllGuard:
		for _, nested := range typed.Guards {
			if guardRequiresPeerBinding(nested) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (e *EnablementEvaluator) findSingleTokenBindingTransition(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) (interfaces.EnabledTransition, bool) {
	if tr == nil || snapshot == nil || len(tr.InputArcs) == 0 {
		return interfaces.EnabledTransition{}, false
	}
	if !singleTokenGuardedTransition(tr) {
		return interfaces.EnabledTransition{}, false
	}
	search := singleTokenBindingSearch{
		evaluator:           e,
		transition:          tr,
		snapshot:            snapshot,
		runtime:             singleTokenRuntimeContext(e, tr, snapshot),
		order:               singleTokenBindingOrder(tr),
		bindings:            make(map[string]*factorytoken.Token, len(tr.InputArcs)),
		result:              make(map[string][]factorytoken.Token, len(tr.InputArcs)),
		arcModes:            make(map[string]interfaces.ArcMode, len(tr.InputArcs)),
		usedConsumeTokenIDs: make(map[string]bool),
	}
	if !search.search(0) {
		return interfaces.EnabledTransition{}, false
	}
	return interfaces.EnabledTransition{
		TransitionID: tr.ID,
		WorkerType:   tr.WorkerType,
		Bindings:     workerBindings(search.result),
		ArcModes:     search.arcModes,
	}, true
}

func singleTokenGuardedTransition(tr *petri.Transition) bool {
	hasGuard := false
	for i := range tr.InputArcs {
		if !isSingleTokenCardinality(tr.InputArcs[i].Cardinality) {
			return false
		}
		if tr.InputArcs[i].Guard != nil {
			hasGuard = true
		}
	}
	return hasGuard
}

func singleTokenBindingOrder(tr *petri.Transition) []int {
	order := make([]int, 0, len(tr.InputArcs))
	for i := range tr.InputArcs {
		if tr.InputArcs[i].Guard == nil {
			order = append(order, i)
		}
	}
	for i := range tr.InputArcs {
		if tr.InputArcs[i].Guard != nil && !guardRequiresPeerBinding(tr.InputArcs[i].Guard) {
			order = append(order, i)
		}
	}
	for i := range tr.InputArcs {
		if guardRequiresPeerBinding(tr.InputArcs[i].Guard) {
			order = append(order, i)
		}
	}
	return order
}

func singleTokenRuntimeContext(e *EnablementEvaluator, tr *petri.Transition, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) petri.RuntimeGuardContext {
	return petri.RuntimeGuardContext{
		Now:                      e.now(),
		CurrentTransitionID:      tr.ID,
		DispatchHistory:          snapshot.DispatchHistory,
		ActiveDispatches:         snapshot.Dispatches,
		RuntimeConfig:            e.runtimeConfig,
		TransitionWorkers:        transitionWorkerTypes(snapshot.Topology, tr),
		StateCategoryForPlace:    stateCategoryForPlace(snapshot.Topology),
		ParentChildRegistrations: snapshot.Marking.ParentChildRegistrations,
	}
}

func stateCategoryForPlace(topology *state.Net) func(string) string {
	if topology == nil || len(topology.WorkTypes) == 0 {
		return nil
	}
	return func(placeID string) string {
		return string(topology.StateCategoryForPlace(placeID))
	}
}

type singleTokenBindingSearch struct {
	evaluator           *EnablementEvaluator
	transition          *petri.Transition
	snapshot            *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	runtime             petri.RuntimeGuardContext
	order               []int
	bindings            map[string]*factorytoken.Token
	result              map[string][]factorytoken.Token
	arcModes            map[string]interfaces.ArcMode
	usedConsumeTokenIDs map[string]bool
}

func (s *singleTokenBindingSearch) search(position int) bool {
	if position >= len(s.order) {
		return s.evaluator.bindingDependenciesMet(s.transition, s.snapshot, s.result)
	}

	arc := &s.transition.InputArcs[s.order[position]]
	key := arcKey(arc)
	candidates := stableTokens(s.snapshot.Marking.TokensInPlace(arc.PlaceID))
	if len(candidates) == 0 {
		return false
	}

	for _, candidate := range s.matchedCandidates(arc, candidates) {
		if !s.tryCandidate(position, arc, key, candidate) {
			continue
		}
		return true
	}
	return false
}

func (e *EnablementEvaluator) bindingDependenciesMet(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	bindings map[string][]factorytoken.Token,
) bool {
	if tr == nil || !transitionUsesDependencyGuard(tr) {
		return true
	}
	if snapshot == nil {
		return false
	}
	return (&petri.DependencyGuard{}).AllDependenciesMet(bindings, &snapshot.Marking)
}

func transitionUsesDependencyGuard(tr *petri.Transition) bool {
	if tr == nil {
		return false
	}
	for i := range tr.InputArcs {
		if guardUsesDependencyGuard(tr.InputArcs[i].Guard) {
			return true
		}
	}
	return false
}

func guardUsesDependencyGuard(guard petri.Guard) bool {
	switch typed := guard.(type) {
	case *petri.DependencyGuard:
		return true
	case *petri.AllGuard:
		for _, nested := range typed.Guards {
			if guardUsesDependencyGuard(nested) {
				return true
			}
		}
	}
	return false
}

func transitionUsesSameNameGuard(tr *petri.Transition) bool {
	if tr == nil {
		return false
	}
	for i := range tr.InputArcs {
		if guardUsesSameNameGuard(tr.InputArcs[i].Guard) {
			return true
		}
	}
	return false
}

func shouldFailClosedSameNameJoin(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) bool {
	return transitionUsesSameNameGuard(tr) && sameNameJoinHasMissingCurrentChild(tr, snapshot)
}

func guardUsesSameNameGuard(guard petri.Guard) bool {
	switch typed := guard.(type) {
	case *petri.SameNameGuard:
		return typed != nil
	case *petri.AllGuard:
		if typed == nil {
			return false
		}
		for _, nested := range typed.Guards {
			if guardUsesSameNameGuard(nested) {
				return true
			}
		}
	}
	return false
}

func sameNameJoinHasMissingCurrentChild(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) bool {
	if tr == nil || snapshot == nil {
		return false
	}
	for i := range tr.InputArcs {
		matchBinding, ok := sameNameMatchBinding(tr.InputArcs[i].Guard)
		if !ok {
			continue
		}
		var peerArc *petri.Arc
		for peerIndex := range tr.InputArcs {
			candidate := &tr.InputArcs[peerIndex]
			if arcKey(candidate) == matchBinding {
				peerArc = candidate
				break
			}
		}
		if peerArc == nil {
			continue
		}
		if sameNameParentHasMissingCurrentChild(
			snapshot,
			stableTokens(snapshot.Marking.TokensInPlace(tr.InputArcs[i].PlaceID)),
			stableTokens(snapshot.Marking.TokensInPlace(peerArc.PlaceID)),
		) || sameNameParentHasMissingCurrentChild(
			snapshot,
			stableTokens(snapshot.Marking.TokensInPlace(peerArc.PlaceID)),
			stableTokens(snapshot.Marking.TokensInPlace(tr.InputArcs[i].PlaceID)),
		) {
			return true
		}
	}
	return false
}

func sameNameParentHasMissingCurrentChild(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	parentCandidates, childCandidates []factorytoken.Token,
) bool {
	for _, parentCandidate := range parentCandidates {
		parentWorkID := parentCandidate.Color.WorkID
		if parentWorkID == "" || parentCandidate.Color.Name == "" || !sameNameParentChildCandidate(
			parentCandidate,
			childCandidates,
		) {
			continue
		}
		registration, registered := snapshot.Marking.ParentChildRegistrations[parentWorkID]
		if !registered {
			// A missing registration does not prove that this is a parent-child
			// population. Preserve the legacy same-name fallback for fresh
			// flows whose relation projection has not been populated yet.
			continue
		}
		if !registration.Complete || len(registration.Children) == 0 ||
			!sameNameRegistrationIsConsistent(registration.Children, parentWorkID) {
			// Once the candidate pair proves the authored parent-child join, an
			// incomplete or contradictory canonical projection cannot authorize
			// an arbitrary historical token through the legacy fallback.
			return true
		}

		foundRegisteredSameNameChild := false
		for index := len(registration.Children) - 1; index >= 0; index-- {
			child := registration.Children[index]
			if child.Color.Name != parentCandidate.Color.Name {
				continue
			}
			foundRegisteredSameNameChild = true
			for _, candidate := range childCandidates {
				if sameNameTokenIdentity(candidate, child) {
					return false
				}
			}
			return true
		}
		if !foundRegisteredSameNameChild {
			// The visible candidate proves the relation, but the canonical
			// projection omits the same-name child. Do not fall back to
			// token ordering when the two authorities disagree.
			return true
		}
	}
	return false
}

func sameNameParentChildCandidate(
	parentCandidate factorytoken.Token,
	childCandidates []factorytoken.Token,
) bool {
	for _, childCandidate := range childCandidates {
		if childCandidate.Color.ParentID == parentCandidate.Color.WorkID &&
			childCandidate.Color.Name == parentCandidate.Color.Name {
			return true
		}
	}
	return false
}

func sameNameRegistrationIsConsistent(children []factorytoken.Token, parentWorkID string) bool {
	seen := make(map[string]struct{}, len(children))
	for _, child := range children {
		if child.Color.ParentID != parentWorkID {
			return false
		}
		identity := sameNameRegistrationIdentity(child)
		if identity == "" {
			return false
		}
		if _, duplicate := seen[identity]; duplicate {
			return false
		}
		seen[identity] = struct{}{}
	}
	return true
}

func sameNameRegistrationIdentity(token factorytoken.Token) string {
	if token.Color.WorkID != "" {
		return "work:" + token.Color.WorkID
	}
	if token.ID != "" {
		return "token:" + token.ID
	}
	return ""
}

func sameNameTokenIdentity(left, right factorytoken.Token) bool {
	if left.Color.WorkID != "" && right.Color.WorkID != "" {
		return left.Color.WorkID == right.Color.WorkID
	}
	return left.ID != "" && left.ID == right.ID
}

func (s *singleTokenBindingSearch) matchedCandidates(arc *petri.Arc, candidates []factorytoken.Token) []factorytoken.Token {
	if arc.Guard == nil {
		if matched, handled := s.sameNamePeerCandidates(arc, candidates); handled {
			return matched
		}
		return candidates
	}
	guardMatched, ok := s.evaluator.evaluateGuard(arc.Guard, s.runtime, candidates, s.bindings, &s.snapshot.Marking)
	if !ok || len(guardMatched) == 0 {
		return nil
	}
	return stableTokens(guardMatched)
}

// sameNamePeerCandidates handles the authored Factory shape where SAME_NAME
// is declared on the parent input and names the child input as its peer. The
// child arc is otherwise unguarded, so its stable token ordering would choose
// a historical child before the parent guard is evaluated. When a peer guard
// references this arc, select the child from the ordered parent registration
// projection before the parent candidate is tried.
func (s *singleTokenBindingSearch) sameNamePeerCandidates(arc *petri.Arc, candidates []factorytoken.Token) ([]factorytoken.Token, bool) {
	if s == nil || s.transition == nil || s.snapshot == nil || arc == nil {
		return nil, false
	}
	childBinding := arcKey(arc)
	for index := range s.transition.InputArcs {
		peerArc := &s.transition.InputArcs[index]
		if peerArc == arc {
			continue
		}
		matchBinding, ok := sameNameMatchBinding(peerArc.Guard)
		if !ok || matchBinding != childBinding {
			continue
		}

		parentCandidates := stableTokens(s.snapshot.Marking.TokensInPlace(peerArc.PlaceID))
		if len(parentCandidates) == 0 {
			return nil, true
		}
		matched := make([]factorytoken.Token, 0, len(candidates))
		seen := make(map[string]bool, len(candidates))
		for _, parentCandidate := range parentCandidates {
			parent := parentCandidate
			selected, selectedOK := (&petri.SameNameGuard{MatchBinding: arcKey(peerArc)}).EvaluateRuntime(
				s.runtime,
				candidates,
				map[string]*factorytoken.Token{arcKey(peerArc): &parent},
				&s.snapshot.Marking,
			)
			if !selectedOK {
				continue
			}
			for _, candidate := range selected {
				if seen[candidate.ID] {
					continue
				}
				seen[candidate.ID] = true
				matched = append(matched, candidate)
			}
		}
		return stableTokens(matched), true
	}
	return nil, false
}

func sameNameMatchBinding(guard petri.Guard) (string, bool) {
	switch typed := guard.(type) {
	case *petri.SameNameGuard:
		if typed == nil || typed.MatchBinding == "" {
			return "", false
		}
		return typed.MatchBinding, true
	case *petri.AllGuard:
		if typed == nil {
			return "", false
		}
		for _, nested := range typed.Guards {
			if matchBinding, ok := sameNameMatchBinding(nested); ok {
				return matchBinding, true
			}
		}
	}
	return "", false
}

func (s *singleTokenBindingSearch) tryCandidate(position int, arc *petri.Arc, key string, candidate factorytoken.Token) bool {
	if arc.Mode != interfaces.ArcModeObserve && s.usedConsumeTokenIDs[candidate.ID] {
		return false
	}

	candidateCopy := candidate
	s.bindings[key] = &candidateCopy
	s.result[key] = []factorytoken.Token{candidateCopy}
	s.arcModes[key] = arc.Mode
	if arc.Mode != interfaces.ArcModeObserve {
		s.usedConsumeTokenIDs[candidate.ID] = true
	}

	if s.search(position + 1) {
		return true
	}

	delete(s.bindings, key)
	delete(s.result, key)
	delete(s.arcModes, key)
	if arc.Mode != interfaces.ArcModeObserve {
		delete(s.usedConsumeTokenIDs, candidate.ID)
	}
	return false
}

func (e *EnablementEvaluator) evaluateGuard(guard petri.Guard, runtime petri.RuntimeGuardContext, candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *petri.MarkingSnapshot) ([]factorytoken.Token, bool) {
	if guard == nil {
		return nil, false
	}
	if runtimeGuard, ok := guard.(petri.RuntimeGuard); ok {
		return runtimeGuard.EvaluateRuntime(runtime, candidates, bindings, marking)
	}
	if clocked, ok := guard.(petri.ClockedGuard); ok {
		return clocked.EvaluateAt(e.now(), candidates, bindings, marking)
	}
	return guard.Evaluate(candidates, bindings, marking)
}

func transitionWorkerTypes(topology *state.Net, current *petri.Transition) map[string]string {
	if topology == nil || len(topology.Transitions) == 0 {
		if current == nil || current.ID == "" || current.WorkerType == "" {
			return nil
		}
		return map[string]string{current.ID: current.WorkerType}
	}
	workersByTransition := make(map[string]string, len(topology.Transitions))
	for transitionID, transition := range topology.Transitions {
		if transition == nil || transition.WorkerType == "" {
			continue
		}
		workersByTransition[transitionID] = transition.WorkerType
	}
	return workersByTransition
}

// ExpandRepeatedBindings converts single-token work transitions into one enabled
// candidate per disjoint token binding. It is intended for schedulers that can
// batch multiple firings of the same transition in a tick.
func ExpandRepeatedBindings(n *state.Net, marking *petri.MarkingSnapshot, enabled []interfaces.EnabledTransition) []interfaces.EnabledTransition {
	if n == nil || marking == nil || len(enabled) == 0 {
		return enabled
	}

	expanded := make([]interfaces.EnabledTransition, 0, len(enabled))
	for _, et := range enabled {
		tr, ok := n.Transitions[et.TransitionID]
		if !ok {
			expanded = append(expanded, et)
			continue
		}
		expanded = append(expanded, expandRepeatedCardinalityOneBindings(tr, marking, et)...)
	}
	return expanded
}

func expandRepeatedCardinalityOneBindings(tr *petri.Transition, marking *petri.MarkingSnapshot, base interfaces.EnabledTransition) []interfaces.EnabledTransition {
	if tr == nil || marking == nil || len(tr.InputArcs) == 0 {
		return []interfaces.EnabledTransition{base}
	}

	arcTokens, candidateCount, hasWorkInput, ok := repeatedBindingArcTokens(tr, marking)
	if !ok || !hasWorkInput || candidateCount <= 1 {
		return []interfaces.EnabledTransition{base}
	}
	return expandRepeatedBindingCandidates(tr, base, arcTokens, candidateCount)
}

func repeatedBindingArcTokens(
	tr *petri.Transition,
	marking *petri.MarkingSnapshot,
) (map[string][]factorytoken.Token, int, bool, bool) {
	arcTokens := make(map[string][]factorytoken.Token)
	candidateCount := 0
	hasWorkInput := false
	for i := range tr.InputArcs {
		key, tokens, count, hasWorkToken, ok := repeatedBindingTokensForInput(&tr.InputArcs[i], marking, candidateCount)
		if !ok {
			return nil, 0, false, false
		}
		arcTokens[key] = tokens
		candidateCount = count
		hasWorkInput = hasWorkInput || hasWorkToken
	}
	return arcTokens, candidateCount, hasWorkInput, true
}

func repeatedBindingTokensForInput(
	arc *petri.Arc,
	marking *petri.MarkingSnapshot,
	currentCandidateCount int,
) (string, []factorytoken.Token, int, bool, bool) {
	if !isSingleTokenCardinality(arc.Cardinality) {
		return "", nil, 0, false, false
	}
	key := arcKey(arc)
	tokens, ok := repeatedBindingTokensForArc(arc, marking)
	if !ok {
		return "", nil, 0, false, false
	}
	candidateCount := currentCandidateCount
	hasWorkInput := false
	if arc.Mode != interfaces.ArcModeObserve {
		candidateCount = minPositiveCandidateCount(candidateCount, len(tokens))
		hasWorkInput = containsWorkCandidateToken(tokens)
	}
	return key, tokens, candidateCount, hasWorkInput, true
}

func repeatedBindingTokensForArc(arc *petri.Arc, marking *petri.MarkingSnapshot) ([]factorytoken.Token, bool) {
	tokens := stableTokens(marking.TokensInPlace(arc.PlaceID))
	if len(tokens) == 0 {
		return nil, false
	}
	if arc.Guard == nil {
		return tokens, true
	}
	dependencyGuard, ok := arc.Guard.(*petri.DependencyGuard)
	if !ok {
		return nil, false
	}
	matched, ok := dependencyGuard.Evaluate(tokens, nil, marking)
	if !ok || len(matched) == 0 {
		return nil, false
	}
	return stableTokens(matched), true
}

func minPositiveCandidateCount(current int, candidate int) int {
	if current == 0 || candidate < current {
		return candidate
	}
	return current
}

func containsWorkCandidateToken(tokens []factorytoken.Token) bool {
	for _, token := range tokens {
		if isWorkCandidateToken(token) {
			return true
		}
	}
	return false
}

func expandRepeatedBindingCandidates(
	tr *petri.Transition,
	base interfaces.EnabledTransition,
	arcTokens map[string][]factorytoken.Token,
	candidateCount int,
) []interfaces.EnabledTransition {
	expanded := make([]interfaces.EnabledTransition, 0, candidateCount)
	for candidateIndex := 0; candidateIndex < candidateCount; candidateIndex++ {
		bindings := make(map[string][]factorytoken.Token, len(base.Bindings))
		arcModes := make(map[string]interfaces.ArcMode, len(base.ArcModes))
		for i := range tr.InputArcs {
			arc := &tr.InputArcs[i]
			key := arcKey(arc)
			arcModes[key] = arc.Mode
			if arc.Mode == interfaces.ArcModeObserve {
				bindings[key] = runtimeTokens(base.Bindings[key])
				continue
			}
			bindings[key] = []factorytoken.Token{arcTokens[key][candidateIndex]}
		}
		expanded = append(expanded, interfaces.EnabledTransition{
			TransitionID: base.TransitionID,
			WorkerType:   base.WorkerType,
			Bindings:     workerBindings(bindings),
			ArcModes:     arcModes,
		})
	}
	return expanded
}

func workerBindings(bindings map[string][]factorytoken.Token) map[string][]workerexecution.Token {
	if len(bindings) == 0 {
		return nil
	}
	projected := make(map[string][]workerexecution.Token, len(bindings))
	for name, tokens := range bindings {
		projected[name] = make([]workerexecution.Token, len(tokens))
		for index, value := range tokens {
			projected[name][index] = factorytoken.ToWorker(value)
		}
	}
	return projected
}

func runtimeTokens(tokens []workerexecution.Token) []factorytoken.Token {
	if len(tokens) == 0 {
		return nil
	}
	projected := make([]factorytoken.Token, len(tokens))
	for index, value := range tokens {
		projected[index] = factorytoken.FromWorker(value)
	}
	return projected
}

func isSingleTokenCardinality(cardinality petri.ArcCardinality) bool {
	if cardinality.Mode == petri.CardinalityOne {
		return true
	}
	return cardinality.Mode == petri.CardinalityN && cardinality.Count == 1
}

func isWorkCandidateToken(token factorytoken.Token) bool {
	if token.Color.DataType == factorytoken.DataTypeResource {
		return false
	}
	return token.Color.DataType == factorytoken.DataTypeWork ||
		token.Color.WorkID != "" ||
		token.Color.WorkTypeID != "" ||
		token.Color.TraceID != ""
}

// arcKey returns the binding key for an arc: its Name if set, otherwise its ID.
func arcKey(arc *petri.Arc) string {
	if arc.Name != "" {
		return arc.Name
	}
	return arc.ID
}

func sortedTransitions(transitions map[string]*petri.Transition) []*petri.Transition {
	ordered := make([]*petri.Transition, 0, len(transitions))
	for _, tr := range transitions {
		ordered = append(ordered, tr)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left := transitionSortID(ordered[i])
		right := transitionSortID(ordered[j])
		if left == right {
			return transitionSortName(ordered[i]) < transitionSortName(ordered[j])
		}
		return left < right
	})
	return ordered
}

func transitionSortID(tr *petri.Transition) string {
	if tr == nil {
		return ""
	}
	return tr.ID
}

func transitionSortName(tr *petri.Transition) string {
	if tr == nil {
		return ""
	}
	return tr.Name
}

func stableTokens(tokens []factorytoken.Token) []factorytoken.Token {
	if len(tokens) < 2 {
		return tokens
	}
	ordered := append([]factorytoken.Token(nil), tokens...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ID == ordered[j].ID {
			return ordered[i].PlaceID < ordered[j].PlaceID
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

// ApplyCardinality selects the appropriate number of tokens from matched candidates
// based on the arc's cardinality mode. Returns nil if the cardinality cannot be satisfied.
func ApplyCardinality(tokens []factorytoken.Token, cardinality petri.ArcCardinality) []factorytoken.Token {
	switch cardinality.Mode {
	case petri.CardinalityOne:
		if len(tokens) < 1 {
			return nil
		}
		return tokens[:1]

	case petri.CardinalityAll:
		if len(tokens) == 0 {
			return nil
		}
		return tokens

	case petri.CardinalityN:
		if len(tokens) < cardinality.Count {
			return nil
		}
		return tokens[:cardinality.Count]

	case petri.CardinalityAllTerminal:
		if len(tokens) == 0 {
			return nil
		}
		return tokens

	case petri.CardinalityZeroOrMore:
		if tokens == nil {
			return []factorytoken.Token{}
		}
		return tokens

	default:
		return nil
	}
}
