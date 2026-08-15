package replay

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestReplaySideEffectsSatisfyProvidersRootPorts proves replay side effects
// satisfy the Providers root and the Workers command effect at the
// Recordings/Workers boundary.
func TestReplaySideEffectsSatisfyProvidersRootPorts(t *testing.T) {
	t.Parallel()

	sideEffects, err := NewSideEffects(
		testFactorySnapshotDecoder,
		testRuntimeConfigDecoder,
		replaySideEffectArtifact(t),
	)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	var provider providers.Service = sideEffects
	var runner workers.CommandRunner = sideEffects
	if provider == nil || runner == nil {
		t.Fatal("replay side effects must satisfy workers root ports")
	}

	providerRequest := providers.ExecuteRequest{
		AttemptID:       "provider-dispatch",
		Provider:        providers.ID("claude"),
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Model:           "claude-3-5-haiku-20241022",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
		Correlation: providers.ExecuteCorrelation{
			DispatchID: "dispatch-correlated",
			ReplayKey:  "process/trace-1/work-1",
			TraceID:    "trace-1",
			WorkIDs:    []string{"work-1"},
		},
	}
	resp, err := provider.Execute(context.Background(), providerRequest)
	if err != nil {
		t.Fatalf("Execute through providers.Service: %v", err)
	}
	if resp.Content != "recorded provider output" {
		t.Fatalf("provider content = %q, want recorded provider output", resp.Content)
	}

	commandRequest := workers.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
		Execution: work.ExecutionMetadata{
			ReplayKey: "process/trace-2/work-2",
			TraceID:   "trace-2",
			WorkIDs:   []string{"work-2"},
		},
	}
	result, err := runner.Run(context.Background(), commandRequest)
	if err != nil {
		t.Fatalf("Run through workers.CommandRunner: %v", err)
	}
	if string(result.Stdout) != "recorded script output\n" {
		t.Fatalf("stdout = %q, want recorded script output", result.Stdout)
	}
}

func TestReplaySideEffectsProvidersBoundaryPreservesSelectionContinuationAndFailures(t *testing.T) {
	assertReplayCatalogBoundary(t)
	assertReplayExecutionBoundary(t)
	assertReplayContinuationBoundary(t)
	assertReplayControlBoundary(t)
	assertReplayFailureBoundary(t)
	assertReplayDirectDiagnostics(t)
	assertReplayClassifiedFailures(t)
	assertReplayRecordedFailure(t)
}

func newReplayBoundaryEffects(t *testing.T) *SideEffects {
	t.Helper()
	sideEffects, err := NewSideEffects(
		testFactorySnapshotDecoder,
		testRuntimeConfigDecoder,
		replaySideEffectArtifact(t),
	)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}
	return sideEffects
}

func replayBoundaryProviderRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		AttemptID:       "provider-dispatch",
		Provider:        providers.IDClaude,
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Model:           "claude-3-5-haiku-20241022",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
		InputBindings:   map[string][]string{"prompt": {"user prompt"}},
		Correlation: providers.ExecuteCorrelation{
			ReplayKey: "process/trace-1/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}
}

func assertReplayCatalogBoundary(t *testing.T) {
	t.Helper()
	root := newReplayBoundaryEffects(t)
	if listed, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{}); err != nil || listed.Providers != nil {
		t.Fatalf("ListProviders() = %#v, %v; want empty replay catalog", listed, err)
	}
	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDClaude})
	if err != nil || got.Provider.ID != providers.IDClaude {
		t.Fatalf("GetProvider() = %#v, %v; want requested recorded identity", got, err)
	}
	if _, err := root.GetProvider(context.Background(), providers.GetProviderRequest{}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("GetProvider(empty) = %v, want ErrInvalidID", err)
	}
	identity, err := root.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{Identity: "claude"})
	if err != nil || identity.ID != providers.IDClaude {
		t.Fatalf("ResolveIdentity() = %#v, %v; want claude", identity, err)
	}
	if _, err := root.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ResolveIdentity(empty) = %v, want ErrInvalidID", err)
	}
	assertReplaySelectionBoundary(t, root)
	if err := root.ValidatePrerequisites(context.Background(), providers.ValidatePrerequisitesRequest{ID: providers.IDClaude}); err != nil {
		t.Fatalf("ValidatePrerequisites() = %v", err)
	}
	if err := root.ValidatePrerequisites(context.Background(), providers.ValidatePrerequisitesRequest{}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ValidatePrerequisites(empty) = %v, want ErrInvalidID", err)
	}
}

func assertReplaySelectionBoundary(t *testing.T, root providers.Service) {
	t.Helper()
	for _, selection := range []struct {
		request providers.ResolveSelectionRequest
		want    providers.ID
	}{
		{request: providers.ResolveSelectionRequest{Workstation: "claude", Factory: "factory"}, want: providers.IDClaude},
		{request: providers.ResolveSelectionRequest{Factory: "claude", ModelProvider: "model"}, want: providers.IDClaude},
		{request: providers.ResolveSelectionRequest{ModelProvider: "claude"}, want: providers.IDClaude},
	} {
		resolved, resolveErr := root.ResolveSelection(context.Background(), selection.request)
		if resolveErr != nil || resolved.Provider != selection.want {
			t.Fatalf("ResolveSelection(%#v) = %#v, %v; want %s", selection.request, resolved, resolveErr, selection.want)
		}
	}
	if _, err := root.ResolveSelection(context.Background(), providers.ResolveSelectionRequest{}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ResolveSelection(empty) = %v, want ErrInvalidID", err)
	}
}

func assertReplayExecutionBoundary(t *testing.T) {
	t.Helper()
	providerRequest := replayBoundaryProviderRequest()

	if _, err := newReplayBoundaryEffects(t).Execute(context.Background(), providers.ExecuteRequest{Provider: providers.IDClaude}); !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("Execute(invalid request) = %v, want ErrExecuteFailed", err)
	}
	executed, err := newReplayBoundaryEffects(t).Execute(context.Background(), providerRequest)
	if err != nil || executed.Content != "recorded provider output" || executed.Diagnostics == nil || executed.Diagnostics.Metadata[workers.ProviderResponseMetadataCompletionEvidence] != "provider_response" {
		t.Fatalf("Execute() = %#v, %v; want recorded content and safe diagnostics", executed, err)
	}
}

func assertReplayContinuationBoundary(t *testing.T) {
	t.Helper()
	providerRequest := replayBoundaryProviderRequest()
	continued, err := newReplayBoundaryEffects(t).Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "recorded-session"},
		Attempt:   providerRequest,
	})
	if err != nil || continued.Outcome != providers.ContinuationOutcomeResumed || continued.Reference.ID != "recorded-session" || continued.Result.Content != "recorded provider output" {
		t.Fatalf("Continue() = %#v, %v; want resumed recorded attempt", continued, err)
	}
	if _, err := newReplayBoundaryEffects(t).Continue(context.Background(), providers.ContinueRequest{Reference: providers.SessionRef{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "recorded-session"}}); !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("Continue(invalid) = %v, want ErrInvalidContinuationRequest", err)
	}
	if _, err := newReplayBoundaryEffects(t).Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "recorded-session"},
		Attempt:   providers.ExecuteRequest{Provider: providers.IDClaude, AttemptID: "missing-dispatch", Correlation: providers.ExecuteCorrelation{ReplayKey: "missing-replay-key"}},
	}); err == nil {
		t.Fatal("Continue(unmatched) = nil, want replay execution failure")
	}
}

func assertReplayControlBoundary(t *testing.T) {
	t.Helper()
	root := newReplayBoundaryEffects(t)
	control, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{Provider: providers.IDClaude, AttemptID: "provider-dispatch", Action: providers.ControlActionCancel})
	if err != nil || control.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt() = %#v, %v; want unsupported", control, err)
	}
	if _, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{Provider: providers.IDClaude, Action: providers.ControlActionCancel}); !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAttempt(invalid) = %v, want ErrInvalidControlRequest", err)
	}
}

func assertReplayFailureBoundary(t *testing.T) {
	t.Helper()
	providerRequest := replayBoundaryProviderRequest()
	missing, missingErr := newReplayBoundaryEffects(t).Execute(context.Background(), providers.ExecuteRequest{
		AttemptID: "missing-dispatch", Provider: providers.IDClaude,
		Correlation: providers.ExecuteCorrelation{ReplayKey: "missing-replay-key"},
	})
	var replayFailure providers.ExecuteFailure
	if missingErr == nil || missing != (providers.ExecuteResult{}) || !errors.As(missingErr, &replayFailure) || replayFailure.Kind != providers.ExecuteFailureKindUnknown {
		t.Fatalf("Execute(unmatched) = %#v, %v; want normalized unknown replay failure", missing, missingErr)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, canceledErr := newReplayBoundaryEffects(t).Execute(canceled, providerRequest)
	if !errors.Is(canceledErr, providers.ErrExecuteCancelled) {
		t.Fatalf("Execute(canceled) = %v, want ErrExecuteCancelled", canceledErr)
	}
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer deadlineCancel()
	_, deadlineErr := newReplayBoundaryEffects(t).Execute(deadline, providerRequest)
	if !errors.Is(deadlineErr, providers.ErrExecuteTimeout) {
		t.Fatalf("Execute(deadline) = %v, want ErrExecuteTimeout", deadlineErr)
	}
}

func assertReplayDirectDiagnostics(t *testing.T) {
	t.Helper()
	direct := &SideEffects{records: []sideEffectRecord{{
		dispatch: replayDispatch{
			dispatchID: "dispatch-direct",
			dispatch:   work.WorkDispatch{Execution: work.ExecutionMetadata{ReplayKey: "direct-replay"}},
		},
		completion: &replayCompletion{
			result: workers.WorkResult{
				Outcome: workers.OutcomeAccepted, Output: "direct provider output",
				ProviderSession: &workers.ProviderSessionMetadata{Provider: "claude", Kind: "thread", ID: "direct-session"},
			},
			diagnostics: &workers.WorkDiagnostics{
				Provider: &workers.ProviderDiagnostic{ResponseMetadata: map[string]string{"provider": "claude"}},
				Command:  &workers.CommandDiagnostic{Command: "claude", Args: []string{"--json"}, Env: map[string]string{"KEY": "value"}, Duration: 1500 * time.Millisecond},
				Panic:    &workers.PanicDiagnostic{Message: "bounded", Stack: "bounded-stack"},
			},
		},
		hasCompletion: true,
	}}}
	directResult, directErr := direct.Execute(context.Background(), providers.ExecuteRequest{
		AttemptID: "dispatch-direct", Provider: providers.IDClaude,
		Correlation: providers.ExecuteCorrelation{ReplayKey: "direct-replay"},
	})
	if directErr != nil || directResult.Content != "direct provider output" || directResult.SessionRef == nil || directResult.SessionRef.ID != "direct-session" || directResult.Diagnostics == nil || directResult.Diagnostics.Command == nil || directResult.Diagnostics.Command.DurationMS != 1500 || directResult.Diagnostics.Panic == nil {
		t.Fatalf("Execute(direct diagnostics) = %#v, %v; want detached session, command, and panic facts", directResult, directErr)
	}
}

func assertReplayClassifiedFailures(t *testing.T) {
	t.Helper()
	for _, failureCase := range []struct {
		workerType workers.WorkFailureType
		provider   providers.ExecuteFailureKind
	}{
		{workers.WorkFailureTypeAuthFailure, providers.ExecuteFailureKindAuthentication},
		{workers.WorkFailureTypePermanentBadRequest, providers.ExecuteFailureKindInvalidRequest},
		{workers.WorkFailureTypeThrottled, providers.ExecuteFailureKindThrottled},
		{workers.WorkFailureTypeTimeout, providers.ExecuteFailureKindTimeout},
		{workers.WorkFailureTypeMisconfigured, providers.ExecuteFailureKindMisconfigured},
	} {
		replayFailure := &SideEffects{records: []sideEffectRecord{{
			dispatch: replayDispatch{dispatchID: "dispatch-classified-failure", dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{ReplayKey: "classified-failure"}}},
			completion: &replayCompletion{result: workers.WorkResult{
				Outcome: workers.OutcomeFailed, Error: "recorded provider failure",
				FailureMetadata: &workers.WorkFailureMetadata{Type: failureCase.workerType},
			}},
			hasCompletion: true,
		}}}
		_, failureErr := replayFailure.Execute(context.Background(), providers.ExecuteRequest{
			AttemptID: "dispatch-classified-failure", Provider: providers.IDClaude,
			Correlation: providers.ExecuteCorrelation{ReplayKey: "classified-failure"},
		})
		var normalized providers.ExecuteFailure
		if failureErr == nil || !errors.As(failureErr, &normalized) || normalized.Kind != failureCase.provider {
			t.Fatalf("Execute(%s) = %v, want normalized %s failure", failureCase.workerType, failureErr, failureCase.provider)
		}
	}
}

func assertReplayRecordedFailure(t *testing.T) {
	t.Helper()
	failureArtifact := testReplayArtifact(
		t,
		replayDispatchCreatedEvent(t, work.WorkDispatch{
			DispatchID: "dispatch-provider-failure", TransitionID: "process", WorkerType: "worker-a", WorkstationName: "process",
			Execution: work.ExecutionMetadata{ReplayKey: "process/trace-failure/work-failure", TraceID: "trace-failure", WorkIDs: []string{"work-failure"}},
		}, 2),
		replayDispatchCompletedEvent(t, "completion-provider-failure", workers.WorkResult{
			DispatchID: "dispatch-provider-failure", TransitionID: "process", Outcome: workers.OutcomeFailed, Error: "provider throttled",
			FailureMetadata: &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyThrottle, Type: workers.WorkFailureTypeThrottled},
		}, 3),
	)
	assignEventSequences(failureArtifact.Events)
	failureEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, failureArtifact)
	if err != nil {
		t.Fatalf("NewSideEffects(failure): %v", err)
	}
	_, failureErr := failureEffects.Execute(context.Background(), providers.ExecuteRequest{
		AttemptID: "dispatch-provider-failure", Provider: providers.IDClaude,
		WorkerType: "worker-a", WorkstationName: "process",
		Correlation: providers.ExecuteCorrelation{ReplayKey: "process/trace-failure/work-failure", TraceID: "trace-failure", WorkIDs: []string{"work-failure"}},
	})
	var throttled providers.ExecuteFailure
	if failureErr == nil || !errors.As(failureErr, &throttled) || throttled.Kind != providers.ExecuteFailureKindThrottled {
		t.Fatalf("Execute(recorded failure) = %v, want normalized throttled failure", failureErr)
	}
}

func TestReplaySideEffectsCommandBoundaryPreservesFallbackAndTimeout(t *testing.T) {
	fallback := &SideEffects{records: []sideEffectRecord{{
		dispatch:      replayDispatch{dispatchID: "dispatch-command-fallback", dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{ReplayKey: "command-fallback"}}},
		completion:    &replayCompletion{result: workers.WorkResult{Outcome: workers.OutcomeFailed, Output: "stdout", Error: "stderr"}},
		hasCompletion: true,
	}}}
	result, err := fallback.Run(context.Background(), workers.CommandRequest{Command: "echo", Execution: work.ExecutionMetadata{ReplayKey: "command-fallback"}})
	if err != nil || string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" || result.ExitCode != 1 {
		t.Fatalf("Run(fallback) = %#v, %v; want output, error, and failed exit code", result, err)
	}

	timedOut := &SideEffects{records: []sideEffectRecord{{
		dispatch:      replayDispatch{dispatchID: "dispatch-command-timeout", dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{ReplayKey: "command-timeout"}}},
		completion:    &replayCompletion{result: workers.WorkResult{Outcome: workers.OutcomeAccepted}, diagnostics: &workers.WorkDiagnostics{Command: &workers.CommandDiagnostic{TimedOut: true}}},
		hasCompletion: true,
	}}}
	_, err = timedOut.Run(context.Background(), workers.CommandRequest{Command: "echo", Execution: work.ExecutionMetadata{ReplayKey: "command-timeout"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run(timeout) = %v, want context deadline exceeded", err)
	}

	missing := &SideEffects{records: []sideEffectRecord{{
		dispatch: replayDispatch{dispatchID: "dispatch-command-missing", dispatch: work.WorkDispatch{TransitionID: "process", Execution: work.ExecutionMetadata{ReplayKey: "command-missing"}}},
	}}}
	if _, err := missing.Run(context.Background(), workers.CommandRequest{Command: "echo", Execution: work.ExecutionMetadata{ReplayKey: "command-missing"}}); err == nil || !strings.Contains(err.Error(), "has no completion") {
		t.Fatalf("Run(missing completion) = %v, want explicit missing-completion error", err)
	}

	noDiagnostics := &SideEffects{records: []sideEffectRecord{{
		dispatch:      replayDispatch{dispatchID: "dispatch-no-diagnostics", dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{ReplayKey: "execute-no-diagnostics"}}},
		completion:    &replayCompletion{result: workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: "output"}},
		hasCompletion: true,
	}}}
	if result, err := noDiagnostics.Execute(context.Background(), providers.ExecuteRequest{AttemptID: "dispatch-no-diagnostics", Provider: providers.IDClaude, Correlation: providers.ExecuteCorrelation{ReplayKey: "execute-no-diagnostics"}}); err != nil || result.Content != "output" || result.Diagnostics == nil || result.Diagnostics.Metadata[workers.ProviderResponseMetadataCompletionEvidence] != "provider_response" || result.Diagnostics.Command != nil || result.Diagnostics.Panic != nil {
		t.Fatalf("Execute(no diagnostics) = %#v, %v; want completion evidence without unsafe diagnostics", result, err)
	}
}

func TestReplayRuntimeWorkersByNameUsesEffectiveRuntimeDefinitions(t *testing.T) {
	factory := &interfaces.FactoryConfig{Workers: []interfaces.FactoryWorkerConfig{{Name: "worker-a", Type: "agent"}, {Name: "worker-missing", Type: "agent"}}}
	runtime := replayRuntimeWorkerLookup{workers: map[string]interfaces.FactoryWorkerConfig{
		"worker-a": {Name: "worker-a", Type: "agent", Provider: "codex"},
	}}
	workersByName := runtimeWorkersByName(factory, runtime)
	if len(workersByName) != 1 || workersByName["worker-a"].Provider != "codex" {
		t.Fatalf("runtimeWorkersByName() = %#v, want only effective worker-a definition", workersByName)
	}
	worker := workersByName["worker-a"]
	worker.Provider = "mutated"
	workersByName["worker-a"] = worker
	if runtime.workers["worker-a"].Provider != "codex" {
		t.Fatal("runtimeWorkersByName() returned the mutable runtime definition")
	}
}

type replayRuntimeWorkerLookup struct {
	workers map[string]interfaces.FactoryWorkerConfig
}

func (lookup replayRuntimeWorkerLookup) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	worker, ok := lookup.workers[name]
	if !ok {
		return nil, false
	}
	return &worker, true
}

func (replayRuntimeWorkerLookup) Workstation(string) (*interfaces.FactoryWorkstationConfig, bool) {
	return nil, false
}
