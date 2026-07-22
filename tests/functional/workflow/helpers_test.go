package workflow

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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

func dispatchesForWorkstation(
	history []interfaces.CompletedDispatch,
	workstationName string,
) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		if dispatch.WorkstationName == workstationName {
			dispatches = append(dispatches, dispatch)
		}
	}
	return dispatches
}

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

func firstInputToken(rawTokens any) workerexecution.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		tok, ok := tokens[0].(workerexecution.Token)
		if !ok {
			return workerexecution.Token{}
		}
		return tok
	case []workerexecution.Token:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		return tokens[0]
	default:
		return workerexecution.Token{}
	}
}

type capturingExecutor struct {
	result       workerexecution.WorkResult
	lastDispatch work.WorkDispatch
	callCount    int
}

func (e *capturingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.lastDispatch = dispatch
	e.callCount++
	result := e.result
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, nil
}
