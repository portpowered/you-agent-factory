package wire

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

type concurrentAgentExecutionResult struct {
	index  int
	result workers.ExecuteResult
	err    error
}

func TestNewServiceExecuteUsesProcessProviderOverrideForAgentRequests(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	override := &statelessProviderOverride{}
	service, err := NewService(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		nil,
		nil,
		func() time.Time { return time.Unix(1, 0) },
		nil,
		nil,
		nil,
		nil,
		override,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var events []workers.InferenceEvent
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-override",
			RuntimeID:        "runtime-override",
			GenerationID:     "generation-override",
			DispatchID:       "dispatch-override",
			AttemptID:        "attempt-override",
		},
		Target: workers.ExecutionTarget{
			WorkerName: runners.AgentIdentity,
			RunnerID:   runners.AgentIdentity,
			Provider:   workers.ProviderReference{ID: string(providers.IDCodex)},
			Output:     workers.OutputPolicy{StopToken: "<COMPLETE>"},
			Prompt:     workers.PromptPolicy{UserMessage: "override prompt"},
		},
		Input: workers.ExecutionInput{
			InferenceEventRecorder: func(event workers.InferenceEvent) {
				events = append(events, event)
			},
		},
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	gotCorrelation := override.request.Correlation
	wantCorrelation := request.Correlation
	if override.calls.Load() != 1 {
		t.Fatalf("provider override calls/request = %d/%#v, want one detached request", override.calls.Load(), override.request)
	}
	if !reflect.DeepEqual(gotCorrelation, providers.ExecuteCorrelation{
		FactorySessionID: wantCorrelation.FactorySessionID,
		RuntimeID:        wantCorrelation.RuntimeID,
		GenerationID:     wantCorrelation.GenerationID,
		DispatchID:       wantCorrelation.DispatchID,
		AttemptID:        wantCorrelation.AttemptID,
		RequestID:        wantCorrelation.RequestID,
		TraceID:          wantCorrelation.TraceID,
	}) {
		t.Fatalf("provider override correlation = %#v, want %#v", gotCorrelation, wantCorrelation)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || len(result.Output.Primary) != 1 ||
		result.Output.Primary[0].Text != "override output\n<COMPLETE>" {
		t.Fatalf("override result = %#v, want accepted normalized output", result)
	}
	if len(events) != 2 || events[0].Request == nil || events[1].Response == nil {
		t.Fatalf("inference events = %#v, want request and response", events)
	}
	if events[0].Request.InferenceRequestID == "" || events[1].Response.InferenceRequestID != events[0].Request.InferenceRequestID {
		t.Fatalf("inference request correlation = %#v, want matching IDs", events)
	}
	if events[1].Response.Continuation == nil || events[1].Response.Continuation.ProviderSessionID != "session-attempt-override" {
		t.Fatalf("provider continuation event = %#v, want override session identity", events[1].Response.Continuation)
	}
}

func TestNewServiceExecuteDetachedAgentRunPreservesGoalDecisionEnvelope(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	override := &statelessProviderOverride{
		content: `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`,
	}
	service, err := NewService(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		nil,
		nil,
		func() time.Time { return time.Unix(1, 0) },
		nil,
		nil,
		nil,
		nil,
		override,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-goal",
			RuntimeID:        "runtime-goal",
			GenerationID:     "generation-goal",
			DispatchID:       "dispatch-goal",
			AttemptID:        "attempt-goal",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      runners.AgentIdentity,
			WorkstationName: "execute-goal",
			RunnerID:        runners.AgentIdentity,
			Provider:        workers.ProviderReference{ID: string(providers.IDCodex)},
			Prompt:          workers.PromptPolicy{UserMessage: "complete this goal"},
			Tools: workers.ToolPolicy{
				AgentLoop: true,
			},
			Output: workers.OutputPolicy{
				Format:                      factorydefinitions.DecisionEnvelopeOutcomeFormat,
				DecisionEnvelope:            true,
				GoalRoutingDecisionEnvelope: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, failure = %#v, want ACCEPTED", result.Outcome, result.Failure)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "ship" {
		t.Fatalf("output = %#v, want the decision-envelope primary output", result.Output)
	}
	if result.Output.Feedback != "ready" || result.Output.Classification != "accepted" {
		t.Fatalf("output metadata = %#v, want feedback and goal classification", result.Output)
	}
	if override.calls.Load() != 1 {
		t.Fatalf("provider override calls = %d, want one detached AGENT_RUN attempt", override.calls.Load())
	}
}

func TestNewServiceExecuteConcurrentAgentAttemptsPreserveCorrelationContinuationAndTerminal(t *testing.T) {
	t.Parallel()

	const attemptCount = 8
	provider := &statelessTestProviders{}
	input := newStatelessConstructionInputs()
	input.agentDependencies.Providers = provider
	var observationsMu sync.Mutex
	observations := make(map[string][]workers.ExecutionObservation, attemptCount)
	service, err := NewService(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		func(_ context.Context, observation workers.ExecutionObservation) error {
			observationsMu.Lock()
			defer observationsMu.Unlock()
			observations[observation.Correlation.DispatchID] = append(
				observations[observation.Correlation.DispatchID], observation.Clone(),
			)
			return nil
		},
		nil,
		func() time.Time { return time.Unix(1, 0) },
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	results := make(chan concurrentAgentExecutionResult, attemptCount)
	var wg sync.WaitGroup
	for index := 0; index < attemptCount; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatchID := fmt.Sprintf("concurrent-agent-dispatch-%d", index)
			attemptID := fmt.Sprintf("concurrent-agent-attempt-%d", index)
			result, executeErr := service.Execute(context.Background(), workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{
					FactorySessionID: "session-concurrent-agent",
					RuntimeID:        "runtime-concurrent-agent",
					GenerationID:     "generation-concurrent-agent",
					DispatchID:       dispatchID,
					AttemptID:        attemptID,
				},
				Target: workers.ExecutionTarget{
					WorkerName: runners.AgentIdentity,
					RunnerID:   runners.AgentIdentity,
					Provider:   workers.ProviderReference{ID: string(providers.IDCodex)},
					Prompt:     workers.PromptPolicy{UserMessage: "concurrent agent prompt"},
				},
			})
			results <- concurrentAgentExecutionResult{index: index, result: result, err: executeErr}
		}()
	}
	wg.Wait()
	close(results)

	for item := range results {
		assertConcurrentAgentExecutionResult(t, item, &observationsMu, observations)
	}

	providerRequests := provider.Requests()
	if len(providerRequests) != attemptCount {
		t.Fatalf("provider requests = %d, want one per concurrent attempt", len(providerRequests))
	}
	seenAttempts := make(map[string]struct{}, attemptCount)
	for _, request := range providerRequests {
		if _, exists := seenAttempts[request.AttemptID]; exists {
			t.Fatalf("provider request duplicated attempt ID %q", request.AttemptID)
		}
		seenAttempts[request.AttemptID] = struct{}{}
		if request.AttemptID == "" || !strings.HasPrefix(request.AttemptID, "concurrent-agent-dispatch-") {
			t.Fatalf("provider request = %#v, want its own detached attempt identity", request)
		}
		wantAttemptID := strings.Replace(request.AttemptID, "-dispatch-", "-attempt-", 1)
		if request.Correlation.DispatchID == "" ||
			request.Correlation.AttemptID == "" ||
			request.Correlation.DispatchID != request.AttemptID ||
			request.Correlation.AttemptID != wantAttemptID {
			t.Fatalf("provider request correlation = %#v, want dispatch identity and distinct caller attempt", request.Correlation)
		}
	}
}

func assertConcurrentAgentExecutionResult(
	t *testing.T,
	item concurrentAgentExecutionResult,
	observationsMu *sync.Mutex,
	observations map[string][]workers.ExecutionObservation,
) {
	t.Helper()
	if item.err != nil {
		t.Fatalf("attempt %d Execute() error = %v", item.index, item.err)
	}
	dispatchID := fmt.Sprintf("concurrent-agent-dispatch-%d", item.index)
	attemptID := fmt.Sprintf("concurrent-agent-attempt-%d", item.index)
	wantCorrelation := workers.ExecutionCorrelation{
		FactorySessionID: "session-concurrent-agent",
		RuntimeID:        "runtime-concurrent-agent",
		GenerationID:     "generation-concurrent-agent",
		DispatchID:       dispatchID,
		AttemptID:        attemptID,
	}
	if item.result.Outcome != workers.ExecutionOutcomeAccepted || item.result.Correlation != wantCorrelation {
		t.Fatalf("attempt %d result = %#v, want accepted result with independent correlation %#v", item.index, item.result, wantCorrelation)
	}
	if item.result.Continuation == nil || item.result.Continuation.ProviderSessionID != "session-"+dispatchID {
		t.Fatalf("attempt %d continuation = %#v, want provider session for %s", item.index, item.result.Continuation, dispatchID)
	}

	observationsMu.Lock()
	attemptObservations := append([]workers.ExecutionObservation(nil), observations[dispatchID]...)
	observationsMu.Unlock()
	if len(attemptObservations) != 2 ||
		attemptObservations[0].Kind != workers.ExecutionObservationKindStarted ||
		attemptObservations[1].Kind != workers.ExecutionObservationKindCompleted ||
		attemptObservations[1].Correlation != wantCorrelation {
		t.Fatalf("attempt %d observations = %#v, want exactly one started and one correlated terminal completion", item.index, attemptObservations)
	}
}
