package subsystems

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/workstationconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// CircuitBreakerSubsystem enforces system limits and evaluates exhaustion
// transitions before the Scheduler runs. It runs at TickGroup -1.
//
// Phase 1: checks non-terminal tokens against GlobalLimits.MaxTokenAge,
// GlobalLimits.MaxTotalVisits, and runtime-config-owned workstation retry limits
// (ConsecutiveFailures).
// On breach, tokens are moved to their work-type's FAILED place.
//
// Phase 2: scans EXHAUSTION transitions and evaluates their guards. If a guard
// matches, the token is moved to the exhaustion transition's output place.
type CircuitBreakerSubsystem struct {
	state         *state.Net
	runtimeConfig interfaces.RuntimeWorkstationLookup
	logger        logging.Logger
	now           func() time.Time // injectable clock for testing
}

// NewCircuitBreaker creates a new CircuitBreakerSubsystem.
func NewCircuitBreaker(n *state.Net, logger logging.Logger, opts ...CircuitBreakerOption) *CircuitBreakerSubsystem {
	return NewCircuitBreakerWithClock(n, time.Now, logger, opts...)
}

// NewCircuitBreakerWithClock creates a CircuitBreakerSubsystem with an injectable clock.
func NewCircuitBreakerWithClock(n *state.Net, now func() time.Time, logger logging.Logger, opts ...CircuitBreakerOption) *CircuitBreakerSubsystem {
	cb := &CircuitBreakerSubsystem{
		state:  n,
		logger: logging.EnsureLogger(logger),
		now:    now,
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

var _ Subsystem = (*CircuitBreakerSubsystem)(nil)

// CircuitBreakerOption configures a CircuitBreakerSubsystem.
type CircuitBreakerOption func(*CircuitBreakerSubsystem)

// WithCircuitBreakerRuntimeConfig injects the authoritative runtime workstation
// config used to derive config-owned retry limits.
func WithCircuitBreakerRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) CircuitBreakerOption {
	return func(cb *CircuitBreakerSubsystem) {
		if runtimeConfig != nil {
			cb.runtimeConfig = runtimeConfig
		}
	}
}

// TickGroup returns CircuitBreaker (-1), ensuring it runs before the Scheduler.
func (cb *CircuitBreakerSubsystem) TickGroup() TickGroup {
	return CircuitBreaker
}

// Execute runs both phases of the circuit breaker.
func (cb *CircuitBreakerSubsystem) Execute(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	var mutations []interfaces.MarkingMutation

	// Track tokens already handled by Phase 1 so Phase 2 doesn't double-move them.
	handled := make(map[string]bool)

	// Phase 1: system limits enforcement.
	for _, token := range snapshot.Marking.Tokens {
		if cb.isTerminalOrFailed(token) {
			continue
		}

		if reason := cb.checkLimits(token); reason != "" {
			failedPlace := cb.failedPlaceForToken(token)
			if failedPlace == "" {
				continue
			}
			cb.logger.Info("circuit-breaker: limit breached",
				"token", token.ID, "place", token.PlaceID, "reason", reason)
			mutations = append(mutations, interfaces.MarkingMutation{
				Type:      interfaces.MutationMove,
				TokenID:   token.ID,
				FromPlace: token.PlaceID,
				ToPlace:   failedPlace,
				Reason:    reason,
			})
			handled[token.ID] = true
		}
	}

	// Phase 2: exhaustion transitions.
	for _, tr := range cb.state.Transitions {
		if tr.Type != petri.TransitionExhaustion {
			continue
		}
		if len(tr.InputArcs) == 0 {
			continue
		}
		if len(tr.OutputArcs) == 0 && !isSystemConsumeTransition(tr) {
			continue
		}

		exhaustionMuts := cb.evaluateExhaustion(tr, &snapshot.Marking, handled)
		mutations = append(mutations, exhaustionMuts...)
	}

	if len(mutations) == 0 {

		return nil, nil
	}
	// otherwise there is a circuit breaker event.
	for _, m := range mutations {
		cb.logger.Info("circuit-breaker: breaking token ",
			"tokenID", m.TokenID,
			"from", m.FromPlace,
			"to", m.ToPlace,
			"reason", m.Reason)
	}

	return &interfaces.TickResult{Mutations: mutations}, nil
}

// checkLimits returns a non-empty reason string if the token breaches any limit.
func (cb *CircuitBreakerSubsystem) checkLimits(token *interfaces.Token) string {
	now := cb.now()

	// MaxTokenAge: how long the token has existed.
	if cb.state.Limits.MaxTokenAge > 0 {
		age := now.Sub(token.CreatedAt)
		if age >= cb.state.Limits.MaxTokenAge {
			return fmt.Sprintf("token age %s exceeds max %s", age, cb.state.Limits.MaxTokenAge)
		}
	}

	// MaxTotalVisits: sum of all transition visits.
	if cb.state.Limits.MaxTotalVisits > 0 {
		totalVisits := 0
		for _, v := range token.History.TotalVisits {
			totalVisits += v
		}
		if totalVisits >= cb.state.Limits.MaxTotalVisits {
			return fmt.Sprintf("total visits %d exceeds max %d", totalVisits, cb.state.Limits.MaxTotalVisits)
		}
	}

	// MaxRetries: per-workstation consecutive failures derived from runtime config.
	for _, tr := range cb.state.Transitions {
		maxRetries := workstationconfig.MaxRetries(tr, cb.runtimeConfig)
		if maxRetries <= 0 {
			continue
		}
		failures := token.History.ConsecutiveFailures[tr.ID]
		if failures >= maxRetries {
			return fmt.Sprintf("consecutive failures %d for transition %s exceeds max %d", failures, tr.ID, maxRetries)
		}
	}

	return ""
}

// evaluateExhaustion checks if an EXHAUSTION transition's guards are satisfied
// and returns MOVE mutations for matched tokens.
func (cb *CircuitBreakerSubsystem) evaluateExhaustion(
	tr *petri.Transition,
	snapshot *petri.MarkingSnapshot,
	handled map[string]bool,
) []interfaces.MarkingMutation {
	// Evaluate input arcs using the same guard logic as scheduler enablement.
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
	matchedTokens := make(map[string][]interfaces.Token) // arc name → matched tokens

	// Phase 1: unguarded arcs.
	for _, idx := range unguarded {
		arc := &tr.InputArcs[idx]
		candidates := cb.filterHandled(snapshot.TokensInPlace(arc.PlaceID), handled)
		if len(candidates) == 0 {
			return nil
		}
		matchedTokens[arc.Name] = candidates
		guardBindings[arc.Name] = &candidates[0]
	}

	// Phase 2: guarded arcs.
	for _, idx := range guarded {
		arc := &tr.InputArcs[idx]
		candidates := cb.filterHandled(snapshot.TokensInPlace(arc.PlaceID), handled)
		matched, ok := cb.evaluateGuard(arc.Guard, candidates, guardBindings, snapshot)
		if !ok {
			return nil
		}
		matchedTokens[arc.Name] = matched
		if len(matched) > 0 {
			guardBindings[arc.Name] = &matched[0]
		}
	}

	// Generate MOVE mutations for all matched tokens from input arcs.
	var mutations []interfaces.MarkingMutation
	seen := make(map[string]bool)
	outputPlace := ""
	if len(tr.OutputArcs) > 0 {
		outputPlace = tr.OutputArcs[0].PlaceID
	}
	for _, tokens := range matchedTokens {
		for i := range tokens {
			tok := &tokens[i]
			if seen[tok.ID] || handled[tok.ID] {
				continue
			}
			seen[tok.ID] = true
			mutations = append(mutations, cb.exhaustionMutation(tr, tok, outputPlace))
		}
	}

	return mutations
}

func (cb *CircuitBreakerSubsystem) evaluateGuard(guard petri.Guard, candidates []interfaces.Token, bindings map[string]*interfaces.Token, snapshot *petri.MarkingSnapshot) ([]interfaces.Token, bool) {
	if guard == nil {
		return nil, false
	}
	if clocked, ok := guard.(petri.ClockedGuard); ok {
		return clocked.EvaluateAt(cb.now(), candidates, bindings, snapshot)
	}
	return guard.Evaluate(candidates, bindings, snapshot)
}

func (cb *CircuitBreakerSubsystem) exhaustionMutation(tr *petri.Transition, tok *interfaces.Token, outputPlace string) interfaces.MarkingMutation {
	mutationType := interfaces.MutationMove
	reason := fmt.Sprintf("exhaustion transition %s", tr.ID)
	if outputPlace == "" && isSystemConsumeTransition(tr) {
		mutationType = interfaces.MutationConsume
		reason = fmt.Sprintf("consumed by system transition %s", tr.ID)
	}
	return interfaces.MarkingMutation{
		Type:      mutationType,
		TokenID:   tok.ID,
		FromPlace: tok.PlaceID,
		ToPlace:   outputPlace,
		Reason:    reason,
	}
}

func isSystemConsumeTransition(tr *petri.Transition) bool {
	return tr != nil && tr.ID == interfaces.SystemTimeExpiryTransitionID
}

// isTerminalOrFailed returns true if the token is already in a TERMINAL or FAILED place.
func (cb *CircuitBreakerSubsystem) isTerminalOrFailed(token *interfaces.Token) bool {
	place, ok := cb.state.Places[token.PlaceID]
	if !ok {
		return false
	}
	// Look up the work type to get the state category.
	wt, ok := cb.state.WorkTypes[place.TypeID]
	if !ok {
		return false
	}
	for _, s := range wt.States {
		if s.Value == place.State {
			return s.Category == state.StateCategoryTerminal || s.Category == state.StateCategoryFailed
		}
	}
	return false
}

// failedPlaceForToken returns the FAILED place ID for the token's work type.
func (cb *CircuitBreakerSubsystem) failedPlaceForToken(token *interfaces.Token) string {
	place, ok := cb.state.Places[token.PlaceID]
	if !ok {
		return ""
	}
	wt, ok := cb.state.WorkTypes[place.TypeID]
	if !ok {
		return ""
	}
	for _, s := range wt.States {
		if s.Category == state.StateCategoryFailed {
			return state.PlaceID(wt.ID, s.Value)
		}
	}
	return ""
}

// filterHandled removes tokens that have already been handled by Phase 1.
func (cb *CircuitBreakerSubsystem) filterHandled(tokens []interfaces.Token, handled map[string]bool) []interfaces.Token {
	if len(handled) == 0 {
		return tokens
	}
	var filtered []interfaces.Token
	for _, t := range tokens {
		if !handled[t.ID] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
