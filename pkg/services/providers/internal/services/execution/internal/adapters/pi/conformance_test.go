package pi_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	pi "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/pi"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestPiAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		Provider:   providers.IDPi,
		NewAdapter: newPiConformanceAdapter,
		NewRoot:    newPiConformanceRoot,
	})
}

func newPiConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalog, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalog,
		execution.Registration{
			Provider: providers.IDPi,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalog, executionService)
}

type piConformanceState struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	observation executiontest.Observation
	started     chan struct{}
	startOnce   sync.Once
}

func newPiConformanceAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &piConformanceState{
		plan:    plan,
		started: make(chan struct{}),
	}
	effect := pi.EffectFunc(state.execute)
	return executiontest.Adapter{
		Attempt: pi.NewRegistration(effect).Attempt,
		Observe: state.observe,
		Started: state.started,
	}
}

func (state *piConformanceState) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (pi.EffectResult, error) {
	state.mu.Lock()
	state.observation.Calls++
	state.observation.Requests = append(
		state.observation.Requests,
		request.Clone(),
	)
	state.mu.Unlock()
	state.startOnce.Do(func() { close(state.started) })
	defer func() {
		state.mu.Lock()
		state.observation.Cleanups++
		state.mu.Unlock()
	}()
	if state.plan.MutateRequest {
		request.ResumeSession.ID = "pi-effect-mutated"
	}
	if state.plan.WaitForContext {
		<-ctx.Done()
		return pi.EffectResult{}, ctx.Err()
	}
	if state.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
	}
	if state.plan.Failure != nil {
		return pi.EffectResult{}, state.plan.Failure
	}
	if err := emitPiConformanceStream(observe, state.plan.Result); err != nil {
		return pi.EffectResult{}, err
	}
	effectResult := pi.EffectResult{
		Command: pi.CommandFacts{
			Stdout: append([]byte(nil), piConformanceStdout(state.plan.Result)...),
		},
	}
	if state.plan.Result.Diagnostics != nil {
		effectResult.DurationMillis = state.plan.Result.Diagnostics.DurationMillis
		effectResult.Metadata = state.plan.Result.Diagnostics.Metadata
	}
	return effectResult, nil
}

func emitPiConformanceStream(
	observe func([]byte) error,
	result providers.ExecuteResult,
) error {
	sessionID := ""
	if result.SessionRef != nil {
		sessionID = result.SessionRef.ID
		record, _ := json.Marshal(map[string]any{"type": "session", "id": sessionID})
		if err := observe(append(record, '\n')); err != nil {
			return err
		}
	}
	record, _ := json.Marshal(map[string]any{
		"type": "message_end",
		"message": map[string]any{
			"id":      "conformance-message",
			"role":    "assistant",
			"content": []map[string]any{{"type": "text", "text": result.Content}},
			"stopReason": "stop",
		},
	})
	return observe(append(record, '\n'))
}

func piConformanceStdout(result providers.ExecuteResult) []byte {
	sessionID := ""
	if result.SessionRef != nil {
		sessionID = result.SessionRef.ID
	}
	lines := make([]string, 0, 2)
	if sessionID != "" {
		lines = append(lines, `{"type":"session","id":"`+sessionID+`"}`)
	}
	lines = append(lines, `{"type":"message_end","message":{"id":"conformance-message","role":"assistant","content":[{"type":"text","text":"`+result.Content+`"}],"stopReason":"stop"}}`)
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (state *piConformanceState) observe() executiontest.Observation {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observation.Clone()
}
