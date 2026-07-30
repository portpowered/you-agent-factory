package agentrun

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models"
)

type staticInferencer struct {
	response string
	err      error
	delay    time.Duration
}

func TestAgentRunInferenceRequestFallsBackToModelProviderRunner(t *testing.T) {
	processEnvironment := []string{"PATH=C:\\Tools", "USERPROFILE=C:\\Users\\customer"}
	req := agentRunInferenceRequest(
		workerexecution.WorkstationExecutionRequest{
			Dispatch:           work.WorkDispatch{DispatchID: "dispatch"},
			ProcessEnvironment: processEnvironment,
		},
		&interfaces.FactoryWorkerConfig{ModelProvider: "codex"},
	)
	if req.RunnerID != "codex" {
		t.Fatalf("RunnerID = %q, want model provider fallback", req.RunnerID)
	}
	if len(req.ProcessEnvironment) != 2 || req.ProcessEnvironment[0] != processEnvironment[0] || req.ProcessEnvironment[1] != processEnvironment[1] {
		t.Fatalf("ProcessEnvironment = %#v, want host execution environment", req.ProcessEnvironment)
	}
	processEnvironment[0] = "PATH=mutated"
	if req.ProcessEnvironment[0] == processEnvironment[0] {
		t.Fatal("ProcessEnvironment aliases workstation request")
	}
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
	response    string
	err         error
	delay       time.Duration
	mu          sync.Mutex
	calls       int
	lastRequest workerexecution.RunnerExecutionRequest
}

func (runner *stubRunner) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.lastRequest = request
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

func (runner *stubRunner) executionRequest() workerexecution.RunnerExecutionRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.lastRequest
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

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
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

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
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

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
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
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "unused"}, nil,

		harness, nil, time.Now)

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
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{}, nil,

		harness, nil, time.Now)

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

	harness := &recordingHarnessAdapter{err: modelhost.ErrHostCapacityExhausted}
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {
					Type:          interfaces.WorkerTypeAgent,
					Model:         "OMNIVOICE_Q4_K_M",
					ModelLocality: interfaces.ModelLocalityLocal,
				},
			},
		},
		&stubRunner{}, nil,

		harness, nil, time.Now)

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

	readinessErr := &modelhost.HostReadinessError{
		Snapshot: modelhost.HostReadinessSnapshot{
			Identity:       modelhost.HostIdentity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: managedruntime.ReadinessStateMissing,
			FailureClass:   modelhost.HostFailureClassMissingAssets,
		},
		Cause: modelhost.ErrHostRuntimeNotReady,
	}
	harness := &recordingHarnessAdapter{err: readinessErr}
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {
					Type:          interfaces.WorkerTypeAgent,
					Model:         "OMNIVOICE_Q4_K_M",
					ModelLocality: interfaces.ModelLocalityLocal,
				},
			},
		},
		&stubRunner{}, nil,

		harness, nil, time.Now)

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
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "slow", delay: 200 * time.Millisecond},
		localToolFileSystem{},
		time.Now,
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

func TestAgentRunExecutor_RequestModelSelectionOverridesWorkerWithoutMutation(t *testing.T) {
	t.Parallel()

	worker := &interfaces.FactoryWorkerConfig{
		Type:          interfaces.WorkerTypeAgent,
		Model:         "configured-model",
		ModelProvider: "configured-provider",
	}
	runner := &stubRunner{response: "done"}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": worker,
			},
		},
		runner,
		localToolFileSystem{},
		time.Now,
	)
	request := testAgentRunRequest()
	request.Model = "requested-model"
	request.ModelProvider = "requested-provider"

	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	inferenceRequest := runner.executionRequest()
	if inferenceRequest.Model != request.Model || inferenceRequest.ModelProvider != request.ModelProvider {
		t.Fatalf(
			"runner selection = %q/%q, want %q/%q",
			inferenceRequest.Model,
			inferenceRequest.ModelProvider,
			request.Model,
			request.ModelProvider,
		)
	}
	if worker.Model != "configured-model" || worker.ModelProvider != "configured-provider" {
		t.Fatalf("configured worker mutated to %q/%q", worker.Model, worker.ModelProvider)
	}
}

func TestEffectiveAgentRunWorkerDefinition_EmptyInterpolationPreservesOperatorOverride(t *testing.T) {
	t.Run("authored placeholders resolve empty", func(t *testing.T) {
		worker := &interfaces.FactoryWorkerConfig{Model: "${model}", ModelProvider: "${provider}"}
		effective := effectiveAgentRunWorkerDefinition(workerexecution.WorkstationExecutionRequest{}, worker)
		if effective.Model != "" || effective.ModelProvider != "" {
			t.Fatalf("effective provider/model = %q/%q, want empty resolved placeholders", effective.ModelProvider, effective.Model)
		}
	})

	t.Run("operator values survive empty invocation values", func(t *testing.T) {
		worker := &interfaces.FactoryWorkerConfig{Model: "operator-model", ModelProvider: "CODEX"}
		effective := effectiveAgentRunWorkerDefinition(workerexecution.WorkstationExecutionRequest{}, worker)
		if effective.Model != worker.Model || effective.ModelProvider != worker.ModelProvider {
			t.Fatalf("effective provider/model = %q/%q, want %q/%q", effective.ModelProvider, effective.Model, worker.ModelProvider, worker.Model)
		}
	})
}

type staticRuntimeConfig struct {
	Workers map[string]*interfaces.FactoryWorkerConfig
}

func (cfg staticRuntimeConfig) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
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

	executor := NewAgentRunExecutor(staticRuntimeConfig{}, &stubRunner{}, localToolFileSystem{}, time.Now)
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

func TestAgentRunExecutorRequiresInjectedClock(t *testing.T) {
	executor := NewAgentRunExecutor(staticRuntimeConfig{}, &stubRunner{}, localToolFileSystem{}, nil)
	if _, err := executor.Execute(context.Background(), testAgentRunRequest()); err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("Execute() error = %v, want missing-clock failure", err)
	}
}

func TestEvaluateAgentRunOutcome_StopTokenAndContinueSemantics(t *testing.T) {
	t.Parallel()

	worker := &interfaces.FactoryWorkerConfig{StopToken: "<COMPLETE>"}
	if got := evaluateAgentRunOutcome("done\n<COMPLETE>", worker); got != workerexecution.OutcomeAccepted {
		t.Fatalf("stop token outcome = %s, want ACCEPTED", got)
	}
	if got := evaluateAgentRunOutcome("completion uses <COMPLETE>\n<CONTINUE>", worker); got != workerexecution.OutcomeContinue {
		t.Fatalf("final continue outcome = %s, want CONTINUE", got)
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

func TestNewAgentRunExecutorWithDependencies_ConfiguresLogger(t *testing.T) {
	t.Parallel()

	logger := spyLogger{}
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{},
		logger,
		nil,
		nil,
		time.Now,
	)

	if executor.logger == nil {
		t.Fatal("expected logger to be configured")
	}
}

func TestAgentRunExecutor_RecordsAgentRunResponseEvent(t *testing.T) {
	t.Parallel()

	var recorded []workerexecution.AgentRunResponseEvent
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
		},
		&stubRunner{response: "done"},
		nil,
		&recordingHarnessAdapter{
			result: HarnessResult{FinalText: "done"},
		},
		func(event workerexecution.AgentRunResponseEvent) {
			recorded = append(recorded, event)
		},
		func() time.Time {
			return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
		},
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
	diagnostics, err := workerexecution.SafeWorkDiagnosticsFromEventPayload(recorded[0].Payload.Diagnostics)
	if err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if diagnostics == nil || diagnostics.AgentRun == nil {
		t.Fatalf("payload diagnostics = %#v, want agentRun inspection", diagnostics)
	}
}
