package opencode_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	opencode "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestOpenCodeAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		Provider:   providers.IDOpenCode,
		NewAdapter: newOpenCodeConformanceAdapter,
		NewRoot:    newOpenCodeConformanceRoot,
	})
}

func newOpenCodeConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalog, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalog,
		execution.Registration{
			Provider: providers.IDOpenCode,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalog, executionService)
}

type openCodeConformanceState struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	observation executiontest.Observation
	started     chan struct{}
	startOnce   sync.Once
}

func newOpenCodeConformanceAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &openCodeConformanceState{
		plan:    plan,
		started: make(chan struct{}),
	}
	effect := opencode.EffectFunc(state.execute)
	return executiontest.Adapter{
		Attempt: opencode.NewRegistration(effect).Attempt,
		Observe: state.observe,
		Started: state.started,
	}
}

func (state *openCodeConformanceState) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (opencode.EffectResult, error) {
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
		request.ResumeSession.ID = "opencode-effect-mutated"
	}
	if state.plan.WaitForContext {
		<-ctx.Done()
		return opencode.EffectResult{}, ctx.Err()
	}
	if state.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
	}
	if state.plan.Failure != nil {
		return opencode.EffectResult{}, state.plan.Failure
	}
	if err := emitOpenCodeConformanceStream(observe, state.plan.Result); err != nil {
		return opencode.EffectResult{}, err
	}
	effectResult := opencode.EffectResult{}
	if state.plan.Result.Diagnostics != nil {
		effectResult.DurationMillis = state.plan.Result.Diagnostics.DurationMillis
		effectResult.Metadata = state.plan.Result.Diagnostics.Metadata
	}
	return effectResult, nil
}

func emitOpenCodeConformanceStream(
	observe func([]byte) error,
	result providers.ExecuteResult,
) error {
	sessionID := ""
	if result.SessionRef != nil {
		sessionID = result.SessionRef.ID
		record, _ := json.Marshal(map[string]any{
			"type": "step_start", "sessionID": sessionID,
		})
		if err := observe(append(record, '\n')); err != nil {
			return err
		}
	}
	end := int64(1)
	textRecord := map[string]any{
		"type": "text",
		"part": map[string]any{
			"id": "conformance-message", "text": result.Content,
			"time": map[string]any{"end": end},
		},
	}
	if sessionID != "" {
		textRecord["sessionID"] = sessionID
	}
	record, _ := json.Marshal(textRecord)
	return observe(append(record, '\n'))
}

func (state *openCodeConformanceState) observe() executiontest.Observation {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observation.Clone()
}
