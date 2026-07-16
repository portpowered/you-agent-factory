// Package workers defines worker dispatcher contracts and compatibility helpers
// for script and model-based workers.
package workers

import (
	"context"
	"encoding/json"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/work"
)

// Dispatcher manages worker execution. It supports two execution modes:
//   - Synchronous (via Tick): all queued dispatches are executed inline; used
//     by test harnesses to control execution step-by-step.
//   - Asynchronous (via Run): dispatches are executed in goroutines; used in
//     production.
type Dispatcher interface {
	// Dispatch executes a work dispatch synchronously, blocking until the
	// result is available.
	Dispatch(ctx context.Context, dispatch *work.WorkDispatch) (workerexecution.WorkResult, error)
	// WorkerState returns a point-in-time snapshot of the dispatcher state.
	WorkerState() workerexecution.WorkerState
	// Tick processes all currently queued dispatches synchronously, blocking
	// until each submitted element completes.
	Tick()
	// Run starts the goroutine-based async dispatch loop (existing behaviour).
	Run()
}

func cloneInputTokens(rawTokens []any) []factorytoken.Token {
	if len(rawTokens) == 0 {
		return nil
	}

	out := make([]factorytoken.Token, 0, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := decodeToken(raw)
		if !ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func clonePetriInputTokens(inputTokens []factorytoken.Token) []any {
	if len(inputTokens) == 0 {
		return nil
	}

	out := make([]any, 0, len(inputTokens))
	for _, token := range inputTokens {
		out = append(out, token)
	}
	return out
}

func decodeToken(raw any) (factorytoken.Token, bool) {
	if token, ok := raw.(factorytoken.Token); ok {
		return token, true
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return factorytoken.Token{}, false
	}
	var token factorytoken.Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return factorytoken.Token{}, false
	}
	return token, true
}

// InputTokens converts typed petri tokens into the shared dispatch representation.
func InputTokens(tokens ...factorytoken.Token) []any {
	return clonePetriInputTokens(tokens)
}

// WorkDispatchInputTokens returns the token payload as typed petri tokens.
func WorkDispatchInputTokens(dispatch work.WorkDispatch) []factorytoken.Token {
	return cloneInputTokens(dispatch.InputTokens)
}
