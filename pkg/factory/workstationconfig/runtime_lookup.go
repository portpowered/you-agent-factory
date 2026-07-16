package workstationconfig

import (
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

const DefaultMaxRetries = 3

// Workstation returns the runtime workstation definition for a transition name.
func Workstation(transition *petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) (*interfaces.FactoryWorkstationConfig, bool) {
	if transition == nil || runtimeConfig == nil {
		return nil, false
	}

	if transition.Name == "" {
		return nil, false
	}

	workstation, ok := runtimeConfig.Workstation(transition.Name)
	if ok && workstation != nil {
		return workstation, true
	}
	return nil, false
}

// Kind derives the transition workstation kind from runtime config.
func Kind(transition *petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) interfaces.WorkstationKind {
	workstation, ok := Workstation(transition, runtimeConfig)
	if !ok || workstation == nil {
		return ""
	}
	return workstation.Kind
}

// MaxRetries derives the transition retry limit from runtime config.
func MaxRetries(transition *petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) int {
	workstation, ok := Workstation(transition, runtimeConfig)
	if !ok || workstation == nil {
		return 0
	}
	if workstation.Limits.MaxRetries > 0 {
		return workstation.Limits.MaxRetries
	}
	return DefaultMaxRetries
}
