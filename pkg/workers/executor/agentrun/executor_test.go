package agentrun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
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

func (runner *stubRunner) Execute(ctx context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()
	if runner.delay > 0 {
		timer := time.NewTimer(runner.delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return workerexecution.RunnerExecutionResult{}, ctx.Err()
		}
	}
	if runner.err != nil {
		return workerexecution.RunnerExecutionResult{}, runner.err
	}
	return workerexecution.RunnerExecutionResult{Content: runner.response}, nil
}

func testAgentRunRequest() workerexecution.WorkstationExecutionRequest {
	return workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{
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
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {Type: workertaxonomy.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "unused"},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {Type: workertaxonomy.WorkerTypeAgent},
			},
		},
		&stubRunner{},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassHarnessRuntime {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassHarnessRuntime)
	}
	if result.Error == "" || result.Error == "provider error" {
		t.Fatalf("Error = %q, want agent run harness failure wording", result.Error)
	}
}

func TestAgentRunExecutor_ModelhostLeaseDeniedSurfacesAgentRunLeaseFailureClass(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{err: modelhost.ErrCapacityExhausted}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {
					Type:          workertaxonomy.WorkerTypeAgent,
					Model:         "OMNIVOICE_Q4_K_M",
					ModelLocality: workerconfig.ModelLocalityLocal,
				},
			},
		},
		&stubRunner{},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassLeaseDenied {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassLeaseDenied)
	}
	if result.Diagnostics.Metadata[DiagnosticRecoveryAction] == "" {
		t.Fatal("expected recovery action for lease denial")
	}
}

func TestAgentRunExecutor_ModelhostReadinessFailureSurfacesAgentRunModelNotReadyClass(t *testing.T) {
	t.Parallel()

	readinessErr := &modelhost.ReadinessError{
		Snapshot: modelhost.ReadinessSnapshot{
			Identity:       modelhost.Identity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: managedruntime.ReadinessStateMissing,
			FailureClass:   modelhost.FailureClassMissingAssets,
		},
		Cause: modelhost.ErrRuntimeNotReady,
	}
	harness := &recordingHarnessAdapter{err: readinessErr}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {
					Type:          workertaxonomy.WorkerTypeAgent,
					Model:         "OMNIVOICE_Q4_K_M",
					ModelLocality: workerconfig.ModelLocalityLocal,
				},
			},
		},
		&stubRunner{},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassModelNotReady {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassModelNotReady)
	}
}

func TestAgentRunExecutor_TimeoutSurfacesAgentRunTimeoutClass(t *testing.T) {
	t.Parallel()

	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {Type: workertaxonomy.WorkerTypeAgent},
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
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassTimeout {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassTimeout)
	}
}

type staticRuntimeConfig struct {
	Workers map[string]*workerconfig.Config
}

func (cfg staticRuntimeConfig) Worker(name string) (*workerconfig.Config, bool) {
	worker, ok := cfg.Workers[name]
	return worker, ok
}

func (cfg staticRuntimeConfig) Workstation(string) (*interfaces.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (cfg staticRuntimeConfig) RuntimeBaseDir() string { return "" }
func (cfg staticRuntimeConfig) FactoryDir() string     { return "" }

func TestAgentRunExecutor_MissingWorkerConfigFails(t *testing.T) {
	t.Parallel()

	executor := NewAgentRunExecutor(staticRuntimeConfig{}, &stubRunner{})
	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "worker config not found: agent-worker" {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestEvaluateAgentRunOutcome_StopTokenAndContinueSemantics(t *testing.T) {
	t.Parallel()

	worker := &workerconfig.Config{StopToken: "<COMPLETE>"}
	if got := evaluateAgentRunOutcome("done <COMPLETE>", worker); got != workerexecution.OutcomeAccepted {
		t.Fatalf("stop token outcome = %s, want ACCEPTED", got)
	}
	if got := evaluateAgentRunOutcome("still working <CONTINUE>", worker); got != workerexecution.OutcomeContinue {
		t.Fatalf("continue outcome = %s, want CONTINUE", got)
	}
	if got := evaluateAgentRunOutcome("review cannot proceed <REJECTED>", worker); got != workerexecution.OutcomeRejected {
		t.Fatalf("rejected outcome = %s, want REJECTED", got)
	}
	if got := evaluateAgentRunOutcome("plain output", nil); got != workerexecution.OutcomeAccepted {
		t.Fatalf("nil worker outcome = %s, want ACCEPTED", got)
	}
}

type spyLogger struct{}

func (spyLogger) Debug(string, ...any)   {}
func (spyLogger) Info(string, ...any)    {}
func (spyLogger) Warn(string, ...any)    {}
func (spyLogger) Error(string, ...any)   {}
func (spyLogger) Verbose(string, ...any) {}

func TestWithAgentRunLogger_ConfiguresExecutor(t *testing.T) {
	t.Parallel()

	logger := spyLogger{}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {Type: workertaxonomy.WorkerTypeAgent},
			},
		},
		&stubRunner{},
		WithAgentRunLogger(logger),
	)
	if executor.logger == nil {
		t.Fatal("expected logger to be configured")
	}
}

func TestAgentRunExecutor_RecordsAgentRunResponseEvent(t *testing.T) {
	t.Parallel()

	var recorded []workerexecution.AgentRunResponseEvent
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {Type: workertaxonomy.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "done"},
		WithAgentRunHarness(&recordingHarnessAdapter{
			result: HarnessResult{FinalText: "done"},
		}),
		WithAgentRunEventRecorder(func(event workerexecution.AgentRunResponseEvent) {
			recorded = append(recorded, event)
		}),
		WithAgentRunClock(func() time.Time {
			return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
		}),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if len(recorded) != 1 {
		t.Fatalf("recorded events = %d, want 1", len(recorded))
	}
	diagnostics, err := workerdiagnostics.SafeWorkDiagnosticsFromEventPayload(recorded[0].Payload.Diagnostics)
	if err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if diagnostics == nil || diagnostics.AgentRun == nil {
		t.Fatalf("payload diagnostics = %#v, want agentRun inspection", diagnostics)
	}
}
