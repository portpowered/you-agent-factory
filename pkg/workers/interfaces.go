// Package workers defines worker executor interfaces and implementations for
// script and model-based workers.
package workers

import (
	"context"
	"encoding/json"

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

// Runner executes one shared runner request and returns the normalized runner
// result used by standard orchestration flows.
type Runner interface {
	Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error)
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
