package claude_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestClaudeAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		Provider:   providers.IDClaude,
		NewAdapter: newClaudeConformanceAdapter,
		NewRoot:    newClaudeConformanceRoot,
	})
}

func newClaudeConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalog, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalog,
		execution.Registration{
			Provider: providers.IDClaude,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalog, executionService)
}

type claudeConformanceState struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	observation executiontest.Observation
	started     chan struct{}
	startOnce   sync.Once
}

func newClaudeConformanceAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &claudeConformanceState{
		plan:    plan,
		started: make(chan struct{}),
	}
	effect := claude.EffectFunc(state.execute)
	return executiontest.Adapter{
		Attempt: claude.NewRegistration(effect).Attempt,
		Observe: state.observe,
		Started: state.started,
	}
}

func (state *claudeConformanceState) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (claude.EffectResult, error) {
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
		request.ResumeSession.ID = "claude-effect-mutated"
	}
	if state.plan.WaitForContext {
		<-ctx.Done()
		return claude.EffectResult{}, ctx.Err()
	}
	if state.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
	}
	if state.plan.Failure != nil {
		return claude.EffectResult{}, state.plan.Failure
	}
	if state.plan.Result.SessionRef != nil {
		record, _ := json.Marshal(map[string]any{
			"type":       "system",
			"subtype":    "init",
			"session_id": state.plan.Result.SessionRef.ID,
		})
		if err := observe(append(record, '\n')); err != nil {
			return claude.EffectResult{}, err
		}
	}
	resultRecord, _ := json.Marshal(map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     state.plan.Result.Content,
		"session_id": conformanceSessionID(state.plan.Result.SessionRef),
	})
	if err := observe(append(resultRecord, '\n')); err != nil {
		return claude.EffectResult{}, err
	}
	effectResult := claude.EffectResult{}
	if state.plan.Result.Diagnostics != nil {
		effectResult.DurationMillis = state.plan.Result.Diagnostics.DurationMillis
		effectResult.Metadata = state.plan.Result.Diagnostics.Metadata
	}
	return effectResult, nil
}

func conformanceSessionID(session *providers.SessionRef) string {
	if session == nil {
		return ""
	}
	return session.ID
}

func (state *claudeConformanceState) observe() executiontest.Observation {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observation.Clone()
}
