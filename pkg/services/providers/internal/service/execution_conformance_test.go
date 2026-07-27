package service_test

import (
	"context"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestStreamingAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		NewAdapter:       newStreamingAdapter,
		NewRoot:          newConformanceRoot,
		SupportsProgress: true,
	})
}

func TestFinalOnlyAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		NewAdapter: newFinalOnlyAdapter,
		NewRoot:    newConformanceRoot,
	})
}

func newConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalogService, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalogService, executionService)
}

type streamingAdapter struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	started     chan struct{}
	startOnce   sync.Once
	observation executiontest.Observation
}

func newStreamingAdapter(plan executiontest.Plan) executiontest.Adapter {
	adapter := &streamingAdapter{
		plan:    plan,
		started: make(chan struct{}),
	}
	return executiontest.Adapter{
		Attempt: adapter.attempt,
		Observe: adapter.observe,
		Started: adapter.started,
	}
}

func (adapter *streamingAdapter) attempt(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter.recordStart(request)
	defer adapter.recordCleanup()
	if adapter.plan.MutateRequest {
		request.ResumeSession.ID = "streaming-adapter-mutated"
	}
	if adapter.plan.WaitForContext {
		<-ctx.Done()
		return providers.ExecuteResult{}, ctx.Err()
	}
	return adapter.plan.Result, adapter.plan.Failure
}

func (adapter *streamingAdapter) recordStart(request providers.ExecuteRequest) {
	adapter.mu.Lock()
	adapter.observation.Calls++
	adapter.observation.Requests = append(
		adapter.observation.Requests,
		request.Clone(),
	)
	adapter.mu.Unlock()
	adapter.startOnce.Do(func() { close(adapter.started) })
}

func (adapter *streamingAdapter) recordCleanup() {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.observation.Cleanups++
}

func (adapter *streamingAdapter) observe() executiontest.Observation {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.observation.Clone()
}

type finalOnlyState struct {
	mu          sync.Mutex
	observation executiontest.Observation
}

func newFinalOnlyAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &finalOnlyState{}
	started := make(chan struct{})
	var startOnce sync.Once
	attempt := func(
		ctx context.Context,
		request providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		state.mu.Lock()
		state.observation.Calls++
		state.observation.Requests = append(
			state.observation.Requests,
			request.Clone(),
		)
		state.mu.Unlock()
		startOnce.Do(func() { close(started) })
		defer func() {
			state.mu.Lock()
			state.observation.Cleanups++
			state.mu.Unlock()
		}()
		if plan.MutateRequest {
			request.ResumeSession.ID = "final-adapter-mutated"
		}
		if plan.WaitForContext {
			<-ctx.Done()
			return providers.ExecuteResult{}, ctx.Err()
		}
		return plan.Result, plan.Failure
	}
	return executiontest.Adapter{
		Attempt: attempt,
		Started: started,
		Observe: func() executiontest.Observation {
			state.mu.Lock()
			defer state.mu.Unlock()
			return state.observation.Clone()
		},
	}
}
