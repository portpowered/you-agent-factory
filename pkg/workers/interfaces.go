// Package workers defines worker executor interfaces and implementations for
// script and model-based workers.
package workers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WorkerExecutor is the side-effect interface — what actually happens when
// a transition fires. This is the only place where external I/O occurs.
// Everything else in the factory is pure CPN state manipulation.
type WorkerExecutor interface {
	Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error)
}

// WorkstationRequestExecutor handles worker-owned execution requests after the
// dispatch-owned contract has been resolved for one workstation invocation.
type WorkstationRequestExecutor interface {
	Execute(ctx context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error)
}

// Dispatcher manages worker execution. It supports two execution modes:
//   - Synchronous (via Tick): all queued dispatches are executed inline; used
//     by test harnesses to control execution step-by-step.
//   - Asynchronous (via Run): dispatches are executed in goroutines; used in
//     production.
type Dispatcher interface {
	// Dispatch executes a work dispatch synchronously, blocking until the
	// result is available.
	Dispatch(ctx context.Context, dispatch *interfaces.WorkDispatch) (interfaces.WorkResult, error)
	// WorkerState returns a point-in-time snapshot of the dispatcher state.
	WorkerState() interfaces.WorkerState
	// Tick processes all currently queued dispatches synchronously, blocking
	// until each submitted element completes.
	Tick()
	// Run starts the goroutine-based async dispatch loop (existing behaviour).
	Run()
}

func cloneInputTokens(rawTokens []any) []interfaces.Token {
	if len(rawTokens) == 0 {
		return nil
	}

	out := make([]interfaces.Token, 0, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := decodeToken(raw)
		if !ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func clonePetriInputTokens(inputTokens []interfaces.Token) []any {
	if len(inputTokens) == 0 {
		return nil
	}

	out := make([]any, 0, len(inputTokens))
	for _, token := range inputTokens {
		out = append(out, token)
	}
	return out
}

func cloneRawInputTokens(inputTokens []any) []any {
	if len(inputTokens) == 0 {
		return nil
	}
	return append([]any(nil), inputTokens...)
}

func decodeToken(raw any) (interfaces.Token, bool) {
	if token, ok := raw.(interfaces.Token); ok {
		return token, true
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return interfaces.Token{}, false
	}
	var token interfaces.Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return interfaces.Token{}, false
	}
	return token, true
}

// InputTokens converts typed petri tokens into the shared dispatch representation.
func InputTokens(tokens ...interfaces.Token) []any {
	return clonePetriInputTokens(tokens)
}

// WorkDispatchInputTokens returns the token payload as typed petri tokens.
func WorkDispatchInputTokens(dispatch interfaces.WorkDispatch) []interfaces.Token {
	return cloneInputTokens(dispatch.InputTokens)
}

// CommandRequestInputTokens returns the subprocess request token payload as typed
// petri tokens.
func CommandRequestInputTokens(request CommandRequest) []interfaces.Token {
	return cloneInputTokens(request.InputTokens)
}

func workDispatchNonResourceTokensForWorkstation(dispatch interfaces.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []interfaces.Token {
	var tokens []interfaces.Token
	for _, token := range orderedWorkDispatchTokensForWorkstation(dispatch, workstationDef) {
		if token.Color.DataType != interfaces.DataTypeResource {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func orderedWorkDispatchTokensForWorkstation(dispatch interfaces.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []interfaces.Token {
	tokens := WorkDispatchInputTokens(dispatch)
	if workstationDef == nil || len(tokens) < 2 {
		return tokens
	}

	byPlace := make(map[string][]int)
	for i, token := range tokens {
		byPlace[token.PlaceID] = append(byPlace[token.PlaceID], i)
	}

	ordered := make([]interfaces.Token, 0, len(tokens))
	used := make([]bool, len(tokens))
	appendPlaceTokens := func(placeID string) {
		for _, index := range byPlace[placeID] {
			used[index] = true
			ordered = append(ordered, tokens[index])
		}
	}

	for _, input := range workstationDef.Inputs {
		appendPlaceTokens(fmt.Sprintf("%s:%s", input.WorkTypeName, input.StateName))
	}
	for _, resource := range workstationDef.Resources {
		appendPlaceTokens(fmt.Sprintf("%s:%s", resource.Name, interfaces.ResourceStateAvailable))
	}
	for i, token := range tokens {
		if used[i] {
			continue
		}
		ordered = append(ordered, token)
	}

	return ordered
}

func cloneEnvVars(envVars map[string]string) map[string]string {
	if len(envVars) == 0 {
		return nil
	}
	clone := make(map[string]string, len(envVars))
	for key, value := range envVars {
		clone[key] = value
	}
	return clone
}
