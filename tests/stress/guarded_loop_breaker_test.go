package stress_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func guardedLoopBreakerWorkstation(name, watchedWorkstation string, maxVisits int, source, target interfaces.IOConfig) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:    name,
		Type:    interfaces.WorkstationTypeLogical,
		Inputs:  []interfaces.IOConfig{source},
		Outputs: []interfaces.IOConfig{target},
		Guards: []interfaces.GuardConfig{{
			Type:        interfaces.GuardTypeVisitCount,
			Workstation: watchedWorkstation,
			MaxVisits:   maxVisits,
		}},
	}
}

func assertDispatchHistoryContainsWorkstationRoute(
	t *testing.T,
	history []interfaces.CompletedDispatch,
	workstationName string,
	terminalPlace string,
) {
	t.Helper()

	for _, dispatch := range history {
		if dispatch.WorkstationName != workstationName {
			continue
		}
		for _, mutation := range dispatch.OutputMutations {
			if mutation.ToPlace == terminalPlace {
				return
			}
		}
	}

	t.Fatalf(
		"dispatch history missing %q route to %q: %#v",
		workstationName,
		terminalPlace,
		history,
	)
}
