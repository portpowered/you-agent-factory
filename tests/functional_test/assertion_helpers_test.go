package functional_test

import (
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func assertDispatchHistoryContainsWorkstation(
	t *testing.T,
	history []interfaces.CompletedDispatch,
	workstationName string,
	terminalPlace string,
	workID string,
) {
	t.Helper()

	for _, dispatch := range history {
		if dispatch.WorkstationName != workstationName {
			continue
		}
		for _, mutation := range dispatch.OutputMutations {
			if mutation.ToPlace != terminalPlace || mutation.Token == nil {
				continue
			}
			if mutation.Token.Color.WorkID == workID {
				return
			}
		}
	}

	t.Fatalf(
		"dispatch history missing %q route to %q for work %q: %#v",
		workstationName,
		terminalPlace,
		workID,
		history,
	)
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
