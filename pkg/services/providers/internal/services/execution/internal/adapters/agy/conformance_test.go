package agy_test

import (
	"context"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestAgyAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		Provider:   providers.IDAntigravity,
		NewAdapter: newAgyConformanceAdapter,
		NewRoot:    newAgyConformanceRoot,
	})
}

func newAgyConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalog, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalog,
		execution.Registration{
			Provider: providers.IDAntigravity,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalog, executionService)
}

type agyConformanceState struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	observation executiontest.Observation
	started     chan struct{}
	startOnce   sync.Once
}

func newAgyConformanceAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &agyConformanceState{
		plan:    plan,
		started: make(chan struct{}),
	}
	effect := agy.EffectFunc(state.execute)
	return executiontest.Adapter{
		Attempt: agy.NewRegistration(effect).Attempt,
		Observe: state.observe,
		Started: state.started,
	}
}

func (state *agyConformanceState) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (agy.EffectResult, error) {
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
		request.ResumeSession.ID = "agy-effect-mutated"
	}
	if state.plan.WaitForContext {
		<-ctx.Done()
		return agy.EffectResult{}, ctx.Err()
	}
	if state.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
	}
	if state.plan.Failure != nil {
		return agy.EffectResult{}, state.plan.Failure
	}
	if state.plan.Result.Content != "" {
		if err := observe([]byte(state.plan.Result.Content)); err != nil {
			return agy.EffectResult{}, err
		}
	}
	effectResult := agy.EffectResult{}
	if state.plan.Result.Diagnostics != nil {
		effectResult.DurationMillis = state.plan.Result.Diagnostics.DurationMillis
		effectResult.Metadata = state.plan.Result.Diagnostics.Metadata
	}
	if state.plan.Result.SessionRef != nil {
		session := state.plan.Result.SessionRef.Clone()
		effectResult.SessionRef = &session
	}
	return effectResult, nil
}

func (state *agyConformanceState) observe() executiontest.Observation {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observation.Clone()
}
