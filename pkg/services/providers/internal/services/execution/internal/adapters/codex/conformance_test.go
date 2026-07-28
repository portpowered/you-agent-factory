package codex_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestCodexAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		NewAdapter: newCodexConformanceAdapter,
		NewRoot:    newCodexConformanceRoot,
	})
}

func newCodexConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalog, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalog,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalog, executionService)
}

type codexConformanceState struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	observation executiontest.Observation
	started     chan struct{}
	startOnce   sync.Once
}

func newCodexConformanceAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &codexConformanceState{
		plan:    plan,
		started: make(chan struct{}),
	}
	effect := codex.EffectFunc(state.execute)
	return executiontest.Adapter{
		Attempt: codex.NewRegistration(effect).Attempt,
		Observe: state.observe,
		Started: state.started,
	}
}

func (state *codexConformanceState) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (codex.EffectResult, error) {
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
		request.ResumeSession.ID = "codex-effect-mutated"
	}
	if state.plan.WaitForContext {
		<-ctx.Done()
		return codex.EffectResult{}, ctx.Err()
	}
	if state.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
	}
	if state.plan.Failure != nil {
		return codex.EffectResult{}, state.plan.Failure
	}
	if state.plan.Result.SessionRef != nil {
		record, _ := json.Marshal(map[string]any{
			"type":      "thread.started",
			"thread_id": state.plan.Result.SessionRef.ID,
		})
		if err := observe(append(record, '\n')); err != nil {
			return codex.EffectResult{}, err
		}
	}
	record, _ := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "conformance-final",
			"type": "agent_message",
			"text": state.plan.Result.Content,
		},
	})
	if err := observe(record); err != nil {
		return codex.EffectResult{}, err
	}
	effectResult := codex.EffectResult{}
	if state.plan.Result.Diagnostics != nil {
		effectResult.DurationMillis = state.plan.Result.Diagnostics.DurationMillis
		effectResult.Metadata = state.plan.Result.Diagnostics.Metadata
	}
	return effectResult, nil
}

func (state *codexConformanceState) observe() executiontest.Observation {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observation.Clone()
}
