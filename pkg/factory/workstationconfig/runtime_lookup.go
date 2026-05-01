package workstationconfig

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// lookupKey returns the canonical authored workstation key for transition-owned
// topology metadata, preferring Name and falling back to ID.
func lookupKey(transition *petri.Transition) string {
	if transition == nil {
		return ""
	}
	if transition.Name != "" {
		return transition.Name
	}
	return transition.ID
}

// Workstation returns the runtime workstation definition for a transition,
// falling back from Name to ID in one place.
func Workstation(transition *petri.Transition, runtimeConfig interfaces.RuntimeWorkstationLookup) (*interfaces.FactoryWorkstationConfig, bool) {
	if transition == nil || runtimeConfig == nil {
		return nil, false
	}
	for _, key := range lookupKeys(transition) {
		workstation, ok := runtimeConfig.Workstation(key)
		if ok && workstation != nil {
			return workstation, true
		}
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
	return workstation.Limits.MaxRetries
}

func lookupKeys(transition *petri.Transition) []string {
	if transition == nil {
		return nil
	}
	key := lookupKey(transition)
	if key == "" {
		return nil
	}
	if transition.ID == "" || transition.ID == key {
		return []string{key}
	}
	return []string{key, transition.ID}
}
