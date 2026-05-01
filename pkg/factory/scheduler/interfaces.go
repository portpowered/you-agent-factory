// Package scheduler provides transition scheduling strategies for the CPN engine.
package scheduler

import (
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// Scheduler selects which enabled transitions to fire in a given tick. It ensures
// no token is double-consumed across concurrent firings.
type Scheduler interface {
	Select(enabled []interfaces.EnabledTransition, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision
}

type repeatedBindingScheduler interface {
	SupportsRepeatedTransitionBindings() bool
}

type runtimeConfigAwareScheduler interface {
	SetRuntimeConfig(interfaces.RuntimeWorkstationLookup)
}

// SupportsRepeatedTransitionBindings reports whether a scheduler can safely
// select multiple distinct bindings for the same transition in one tick.
func SupportsRepeatedTransitionBindings(s Scheduler) bool {
	if s == nil {
		return false
	}
	optIn, ok := s.(repeatedBindingScheduler)
	return ok && optIn.SupportsRepeatedTransitionBindings()
}

// ApplyRuntimeConfig injects authoritative workstation runtime metadata into
// schedulers that opt into runtime-config-aware priority derivation.
func ApplyRuntimeConfig(s Scheduler, runtimeConfig interfaces.RuntimeWorkstationLookup) {
	if s == nil || runtimeConfig == nil {
		return
	}
	aware, ok := s.(runtimeConfigAwareScheduler)
	if !ok {
		return
	}
	aware.SetRuntimeConfig(runtimeConfig)
}
