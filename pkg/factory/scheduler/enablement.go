package scheduler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// EnablementEvaluator wraps transition enablement logic with structured logging.
// When a logger is provided, each transition evaluation emits log output showing
// the transition ID, whether it was enabled or disabled, and the reason for
// disablement.
type EnablementEvaluator struct {
	logger logging.Logger
	now    func() time.Time
}

// EnablementOption configures an EnablementEvaluator.
type EnablementOption func(*EnablementEvaluator)

// WithEnablementClock sets the runtime clock used by time-dependent guards.
func WithEnablementClock(now func() time.Time) EnablementOption {
	return func(e *EnablementEvaluator) {
		if now != nil {
			e.now = now
		}
	}
}

// NewEnablementEvaluator creates an EnablementEvaluator with the given logger.
// If logger is nil, logging is a no-op.
func NewEnablementEvaluator(logger logging.Logger, opts ...EnablementOption) *EnablementEvaluator {
	evaluator := &EnablementEvaluator{
		logger: logging.EnsureLogger(logger),
		now:    time.Now,
	}
	for _, opt := range opts {
		opt(evaluator)
	}
	return evaluator
}

// FindEnabledTransitions identifies all transitions whose input arcs are satisfied
// in the current marking. Each transition evaluation is logged with its result.
func (e *EnablementEvaluator) FindEnabledTransitions(ctx context.Context, n *state.Net, marking *petri.MarkingSnapshot) []interfaces.EnabledTransition {
	var enabled []interfaces.EnabledTransition

	transitions := sortedTransitions(n.Transitions)
	for _, tr := range transitions {
		if et, ok := e.checkTransitionEnabled(ctx, tr, marking); ok {
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
func (e *EnablementEvaluator) checkTransitionEnabled(_ context.Context, tr *petri.Transition, marking *petri.MarkingSnapshot) (interfaces.EnabledTransition, bool) {
	if len(tr.InputArcs) == 0 {
		e.logger.Debug("enablement: transition disabled",
			"transitionID", tr.ID,
			"transitionName", tr.Name,
			"reason", "no input arcs")
		return interfaces.EnabledTransition{}, false
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

	guardBindings := make(map[string]*interfaces.Token)
	result := make(map[string][]interfaces.Token)
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

	// Phase 2: evaluate guarded arcs with bindings from phase 1.
	for _, idx := range guarded {
		arc := &tr.InputArcs[idx]
		candidates := stableTokens(marking.TokensInPlace(arc.PlaceID))
		guardMatched, ok := e.evaluateGuard(arc.Guard, candidates, guardBindings, marking)
		if !ok {
			e.logger.Debug("enablement: transition disabled",
				"transitionID", tr.ID,
				"transitionName", tr.Name,
				"reason", fmt.Sprintf("guard failed for arc %q (place %s, candidates %d)",
					arcKey(arc), arc.PlaceID, len(candidates)))
			return interfaces.EnabledTransition{}, false
		}
		matched := ApplyCardinality(stableTokens(guardMatched), arc.Cardinality)
		if matched == nil {
			e.logger.Debug("enablement: transition disabled",
				"transitionID", tr.ID,
				"transitionName", tr.Name,
				"reason", fmt.Sprintf("insufficient tokens after guard for arc %q (place %s, cardinality %d, matched %d)",
					arcKey(arc), arc.PlaceID, arc.Cardinality.Mode, len(guardMatched)))
			return interfaces.EnabledTransition{}, false
		}
		key := arcKey(arc)
		result[key] = matched
		arcModes[key] = arc.Mode
		if len(matched) > 0 {
			guardBindings[key] = &matched[0]
		}
	}

	return interfaces.EnabledTransition{
		TransitionID: tr.ID,
		WorkerType:   tr.WorkerType,
		Bindings:     result,
		ArcModes:     arcModes,
	}, true
}

func (e *EnablementEvaluator) evaluateGuard(guard petri.Guard, candidates []interfaces.Token, bindings map[string]*interfaces.Token, marking *petri.MarkingSnapshot) ([]interfaces.Token, bool) {
	if guard == nil {
		return nil, false
	}
	if clocked, ok := guard.(petri.ClockedGuard); ok {
		return clocked.EvaluateAt(e.now(), candidates, bindings, marking)
	}
	return guard.Evaluate(candidates, bindings, marking)
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

	arcTokens := make(map[string][]interfaces.Token)
	candidateCount := 0
	hasWorkInput := false

	for i := range tr.InputArcs {
		arc := &tr.InputArcs[i]
		if !isSingleTokenCardinality(arc.Cardinality) {
			return []interfaces.EnabledTransition{base}
		}

		key := arcKey(arc)
		tokens := stableTokens(marking.TokensInPlace(arc.PlaceID))
		if len(tokens) == 0 {
			return []interfaces.EnabledTransition{base}
		}
		if arc.Guard != nil {
			dependencyGuard, ok := arc.Guard.(*petri.DependencyGuard)
			if !ok {
				return []interfaces.EnabledTransition{base}
			}
			matched, ok := dependencyGuard.Evaluate(tokens, nil, marking)
			if !ok || len(matched) == 0 {
				return []interfaces.EnabledTransition{base}
			}
			tokens = stableTokens(matched)
		}
		arcTokens[key] = tokens

		if arc.Mode == interfaces.ArcModeObserve {
			continue
		}
		if candidateCount == 0 || len(tokens) < candidateCount {
			candidateCount = len(tokens)
		}
		for _, token := range tokens {
			if isWorkCandidateToken(token) {
				hasWorkInput = true
				break
			}
		}
	}

	if !hasWorkInput || candidateCount <= 1 {
		return []interfaces.EnabledTransition{base}
	}

	expanded := make([]interfaces.EnabledTransition, 0, candidateCount)
	for candidateIndex := 0; candidateIndex < candidateCount; candidateIndex++ {
		bindings := make(map[string][]interfaces.Token, len(base.Bindings))
		arcModes := make(map[string]interfaces.ArcMode, len(base.ArcModes))
		for i := range tr.InputArcs {
			arc := &tr.InputArcs[i]
			key := arcKey(arc)
			arcModes[key] = arc.Mode
			if arc.Mode == interfaces.ArcModeObserve {
				bindings[key] = append([]interfaces.Token(nil), base.Bindings[key]...)
				continue
			}
			bindings[key] = []interfaces.Token{arcTokens[key][candidateIndex]}
		}
		expanded = append(expanded, interfaces.EnabledTransition{
			TransitionID: base.TransitionID,
			WorkerType:   base.WorkerType,
			Bindings:     bindings,
			ArcModes:     arcModes,
		})
	}

	return expanded
}

func isSingleTokenCardinality(cardinality petri.ArcCardinality) bool {
	if cardinality.Mode == petri.CardinalityOne {
		return true
	}
	return cardinality.Mode == petri.CardinalityN && cardinality.Count == 1
}

func isWorkCandidateToken(token interfaces.Token) bool {
	if token.Color.DataType == interfaces.DataTypeResource {
		return false
	}
	return token.Color.DataType == interfaces.DataTypeWork ||
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

func stableTokens(tokens []interfaces.Token) []interfaces.Token {
	if len(tokens) < 2 {
		return tokens
	}
	ordered := append([]interfaces.Token(nil), tokens...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ID == ordered[j].ID {
			return ordered[i].PlaceID < ordered[j].PlaceID
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

// FindEnabledTransitions identifies all transitions whose input arcs are satisfied
// in the current marking. Guards are evaluated with bindings from other named arcs.
//
// Unguarded arcs are evaluated first to build the initial binding set, then guarded
// arcs are evaluated with those bindings available.
//
// This is a convenience function that uses a no-op logger. For structured logging,
// use EnablementEvaluator directly.
func FindEnabledTransitions(n *state.Net, marking *petri.MarkingSnapshot) []interfaces.EnabledTransition {
	eval := NewEnablementEvaluator(nil)
	return eval.FindEnabledTransitions(context.Background(), n, marking)
}

// ApplyCardinality selects the appropriate number of tokens from matched candidates
// based on the arc's cardinality mode. Returns nil if the cardinality cannot be satisfied.
func ApplyCardinality(tokens []interfaces.Token, cardinality petri.ArcCardinality) []interfaces.Token {
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
			return []interfaces.Token{}
		}
		return tokens

	default:
		return nil
	}
}
