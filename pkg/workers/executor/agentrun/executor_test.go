package agentrun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type staticInferencer struct {
	response string
	err      error
	delay    time.Duration
}

func (s staticInferencer) Infer(ctx context.Context, _ messages.InferenceRequest) (messages.InferenceResult, error) {
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return messages.InferenceResult{}, ctx.Err()
		}
	}
	if s.err != nil {
		return messages.InferenceResult{}, s.err
	}
	return messages.InferenceResult{
		Message: messages.NewTextMessage(messages.RoleAssistant, s.response),
	}, nil
}

func (s staticInferencer) InferStream(ctx context.Context, req messages.InferenceRequest) (<-chan messages.StreamMessage, error) {
	result, err := s.Infer(ctx, req)
	ch := make(chan messages.StreamMessage, 4)
	go func() {
		defer close(ch)
		if err != nil {
			ch <- messages.StreamMessage{Type: messages.StreamTypeError, Value: messages.NewErrorValue(err.Error())}
			return
		}
		text := result.Message.TextContent()
		ch <- messages.StreamMessage{Type: messages.StreamTypeTextDelta, Value: messages.NewTextDeltaValue(text)}
		ch <- messages.StreamMessage{Type: messages.StreamTypeMessageEnd, Value: messages.NewMessageEndValue(messages.TokenUsage{})}
	}()
	return ch, nil
}

type recordingHarnessAdapter struct {
	lastInput HarnessInput
	result    HarnessResult
	err       error
}

func (adapter *recordingHarnessAdapter) Execute(_ context.Context, input HarnessInput) (HarnessResult, error) {
	adapter.lastInput = input
	if adapter.err != nil {
		return HarnessResult{}, adapter.err
	}
	return adapter.result, nil
}

type stubRunner struct {
	response string
	err      error
	delay    time.Duration
	mu       sync.Mutex
	calls    int
}

func (runner *stubRunner) Execute(ctx context.Context, _ interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()
	if runner.delay > 0 {
		timer := time.NewTimer(runner.delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return interfaces.RunnerExecutionResult{}, ctx.Err()
		}
	}
	if runner.err != nil {
		return interfaces.RunnerExecutionResult{}, runner.err
	}
	return interfaces.RunnerExecutionResult{Content: runner.response}, nil
}

func testAgentRunRequest() interfaces.WorkstationExecutionRequest {
	return interfaces.WorkstationExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
			DispatchID:      "dispatch-1",
			TransitionID:    "transition-1",
			WorkerType:      "agent-worker",
			WorkstationName: "execute-story",
		},
		WorkerType:   "agent-worker",
		SystemPrompt: "You are an agent.",
		UserMessage:  "Complete the story.",
	}
}

func TestLibraryHarnessAdapter_SuccessfulCompletion(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter()
	result, err := adapter.Execute(context.Background(), HarnessInput{
		SystemPrompt: "system",
		UserMessage:  "hello",
		Inferencer:   staticInferencer{response: "done"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.FinalText != "done" {
		t.Fatalf("FinalText = %q, want done", result.FinalText)
	}
}

func TestLibraryHarnessAdapter_CancellationStopsLoop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter := NewLibraryHarnessAdapter()
	_, err := adapter.Execute(ctx, HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{response: "ignored", delay: time.Second},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
}

func TestLibraryHarnessAdapter_HarnessFailure(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter()
	_, err := adapter.Execute(context.Background(), HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{err: errors.New("model exploded")},
	})
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
}

func TestAgentRunExecutor_MapsSuccessfulCompletionToWorkResult(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{
		result: HarnessResult{FinalText: "final answer"},
	}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "unused"},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if result.Output != "final answer" {
		t.Fatalf("Output = %q, want final answer", result.Output)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticExecutionBehavior] != ExecutionBehaviorAgentRun {
		t.Fatalf("Diagnostics = %#v, want agent_run execution behavior", result.Diagnostics)
	}
}

func TestAgentRunExecutor_HarnessFailureSurfacesAgentRunFailureClass(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{err: errors.New("loop failed")}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassHarnessRuntime {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassHarnessRuntime)
	}
	if result.Error == "" || result.Error == "provider error" {
		t.Fatalf("Error = %q, want agent run harness failure wording", result.Error)
	}
}

func TestAgentRunExecutor_TimeoutSurfacesAgentRunTimeoutClass(t *testing.T) {
	t.Parallel()

	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "slow", delay: 200 * time.Millisecond},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := executor.Execute(ctx, testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassTimeout {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassTimeout)
	}
}

type staticRuntimeConfig struct {
	Workers map[string]*interfaces.WorkerConfig
}

func (cfg staticRuntimeConfig) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := cfg.Workers[name]
	return worker, ok
}

func (cfg staticRuntimeConfig) Workstation(string) (*interfaces.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (cfg staticRuntimeConfig) RuntimeBaseDir() string { return "" }
func (cfg staticRuntimeConfig) FactoryDir() string     { return "" }
