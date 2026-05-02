package subsystems

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// TerminationCheckSubsystem detects when the runtime snapshot has no active
// work left: no dispatches are in flight, all resource tokens have been
// returned, and every visible token is already terminal or failed. It is
// intentionally snapshot-driven: it does not query transition enablement and
// it does not retain its own lifecycle state.
type TerminationCheckSubsystem struct {
	state       *state.Net
	logger      logging.Logger
	runtimeMode interfaces.RuntimeMode
}

// NewTerminationCheck creates a new TerminationCheckSubsystem.
func NewTerminationCheck(n *state.Net, logger logging.Logger, mode interfaces.RuntimeMode) *TerminationCheckSubsystem {
	if mode == "" {
		mode = interfaces.RuntimeModeBatch
	}
	return &TerminationCheckSubsystem{
		state:       n,
		logger:      logging.EnsureLogger(logger),
		runtimeMode: mode,
	}
}

var _ Subsystem = (*TerminationCheckSubsystem)(nil)

// TickGroup returns TerminationCheck (40).
func (tc *TerminationCheckSubsystem) TickGroup() TickGroup {
	return TerminationCheck
}

// Execute checks if the snapshot shows a fully terminated workflow.
func (tc *TerminationCheckSubsystem) Execute(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	if tc.runtimeMode != interfaces.RuntimeModeBatch {
		return nil, nil
	}
	if !tc.shouldTerminate(snapshot) {
		return nil, nil
	}

	tc.logger.Info("termination-check: no active work remains in the snapshot",
		"tokens", len(snapshot.Marking.Tokens),
		"in_flight", snapshot.InFlightCount)

	return &interfaces.TickResult{ShouldTerminate: true}, nil
}

func (tc *TerminationCheckSubsystem) shouldTerminate(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.InFlightCount > 0 {
		return false
	}
	if !tc.allResourcesReturned(&snapshot.Marking) {
		return false
	}

	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		if !tc.isTerminalOrFailed(token) {
			return false
		}
	}

	return true
}

// isTerminalOrFailed returns true if the token is in a TERMINAL or FAILED place.
func (tc *TerminationCheckSubsystem) isTerminalOrFailed(token *interfaces.Token) bool {
	place, ok := tc.state.Places[token.PlaceID]
	if !ok {
		return false
	}
	wt, ok := tc.state.WorkTypes[place.TypeID]
	if !ok {
		_, isResource := tc.state.Resources[place.TypeID]
		return isResource
	}
	for _, s := range wt.States {
		if s.Value == place.State {
			return s.Category == state.StateCategoryTerminal || s.Category == state.StateCategoryFailed
		}
	}
	return false
}

// allResourcesReturned checks that each resource place has at least its initial
// capacity of tokens (i.e., consumed resources have been returned).
func (tc *TerminationCheckSubsystem) allResourcesReturned(snapshot *petri.MarkingSnapshot) bool {
	for _, res := range tc.state.Resources {
		placeID := state.PlaceID(res.ID, interfaces.ResourceStateAvailable)
		tokensInPlace := snapshot.TokensInPlace(placeID)
		if len(tokensInPlace) < res.Capacity {
			return false
		}
	}
	return true
}
