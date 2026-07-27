package wire

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/testkit"
)

const agentFixtureExecutionFailure = "fixture execution failure"

func TestNewAgentRegistryIsInertAndExecutesOneDetachedProviderAttempt(t *testing.T) {
	fake := newAgentProvidersFake()
	registry, err := NewAgentRegistry(runners.AgentDependencies{
		Providers: fake,
		Publish:   agentNoopPublisher,
	})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error = %v", err)
	}
	if fake.calls.Load() != 0 {
		t.Fatalf("construction Providers.Execute calls = %d, want 0", fake.calls.Load())
	}

	first, err := registry.Resolve(runners.ResolutionRequest{
		Identity: agent.Identity,
		RequiredCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
			workers.RunnerOptionalCapabilityStructuredOutput,
			workers.RunnerOptionalCapabilityWorkingDirectory,
			workers.RunnerOptionalCapabilityWorktree,
		},
	})
	if err != nil {
		t.Fatalf("Resolve(agent) error = %v", err)
	}
	if first.Identity != agent.Identity ||
		first.Metadata.ID != agent.Identity ||
		first.Metadata.DisplayName != "Agent" ||
		first.Runner == nil {
		t.Fatalf("Resolve(agent) = %#v, want complete Agent binding", first)
	}
	if fake.calls.Load() != 0 {
		t.Fatalf("resolution Providers.Execute calls = %d, want 0", fake.calls.Load())
	}

	first.Metadata.Capabilities.Optional[0].Detail = "caller-mutated"
	second, err := registry.Resolve(runners.ResolutionRequest{Identity: agent.Identity})
	if err != nil {
		t.Fatalf("second Resolve(agent) error = %v", err)
	}
	if reflect.DeepEqual(first.Metadata, second.Metadata) {
		t.Fatalf("second metadata = %#v, want detached registry snapshot", second.Metadata)
	}

	result, err := second.Runner.Execute(t.Context(), agentRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want 1", fake.calls.Load())
	}
	assertAgentProviderRequest(t, fake.Request())
	assertAgentResult(t, result)

	result.ProviderSession.ID = "caller-mutated"
	result.Diagnostics.Metadata["fixture"] = "caller-mutated"
	result.Diagnostics.Provider.ResponseMetadata["fixture"] = "caller-mutated"
	if fake.result.SessionRef.ID != "provider-session-1" ||
		fake.result.Diagnostics.Metadata["fixture"] != "detached" {
		t.Fatal("Runner result retained Providers-owned mutable values")
	}
}

func TestAgentRunnerThroughRegistryConformsToCommonContract(t *testing.T) {
	fake := newAgentProvidersFake()
	registry, err := NewAgentRegistry(runners.AgentDependencies{
		Providers: fake,
		Publish:   agentNoopPublisher,
	})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error = %v", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: agent.Identity})
	if err != nil {
		t.Fatalf("Resolve(agent) error = %v", err)
	}

	valid := agentRequest()
	invalid := workers.CloneProviderInferenceRequest(valid)
	invalid.Dispatch.DispatchID = ""
	unsupported := workers.CloneProviderInferenceRequest(valid)
	unsupported.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
		workers.RunnerOptionalCapabilityImageInput,
	}
	failure := workers.CloneProviderInferenceRequest(valid)
	failure.UserMessage = agentFixtureExecutionFailure

	testkit.Run(t, testkit.Subject{
		Runner:             binding.Runner,
		ValidRequest:       valid,
		InvalidRequest:     invalid,
		UnsupportedRequest: unsupported,
		FailureRequest:     failure,
		ExpectedResult:     expectedAgentResult(),
		AssertCaptured: func(t *testing.T) {
			t.Helper()
			assertAgentProviderRequest(t, fake.Request())
		},
	})
}

func TestAgentRunnerSnapshotsRequestBeforeProviderAttempt(t *testing.T) {
	fake := &blockingAgentProvidersFake{
		agentProvidersFake: newAgentProvidersFake(),
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	registry, err := NewAgentRegistry(runners.AgentDependencies{
		Providers: fake,
		Publish:   agentNoopPublisher,
	})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error = %v", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: agent.Identity})
	if err != nil {
		t.Fatalf("Resolve(agent) error = %v", err)
	}

	request := agentRequest()
	done := make(chan error, 1)
	go func() {
		_, executeErr := binding.Runner.Execute(t.Context(), request)
		done <- executeErr
	}()
	<-fake.entered
	request.RunnerID = string(providers.IDGemini)
	request.Dispatch.DispatchID = "caller-mutated"
	request.SystemPrompt = "caller-mutated"
	request.UserMessage = "caller-mutated"
	request.OutputSchema = "caller-mutated"
	request.SessionID = "caller-mutated"
	request.WorkingDirectory = "caller-mutated"
	request.Worktree = "caller-mutated"
	request.EnvVars = map[string]string{"FIXTURE": "caller-mutated"}
	close(fake.release)
	if err := <-done; err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertAgentProviderRequest(t, fake.Request())
	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want 1", fake.calls.Load())
	}
}

func TestNewAgentRegistryRejectsMissingProvidersRoot(t *testing.T) {
	_, err := NewAgentRegistry(runners.AgentDependencies{
		Publish: agentNoopPublisher,
	})
	if err == nil {
		t.Fatal("NewAgentRegistry() error = nil, want missing Providers root")
	}
}

func TestNewAgentRegistryRejectsMissingProgressPublisher(t *testing.T) {
	_, err := NewAgentRegistry(runners.AgentDependencies{
		Providers: newAgentProvidersFake(),
	})
	if err == nil {
		t.Fatal("NewAgentRegistry() error = nil, want missing progress publisher")
	}
}

type agentProvidersFake struct {
	mu      sync.Mutex
	request providers.ExecuteRequest
	result  providers.ExecuteResult
	calls   atomic.Int32
}

type blockingAgentProvidersFake struct {
	*agentProvidersFake
	entered chan struct{}
	release chan struct{}
}

var _ providers.Service = (*agentProvidersFake)(nil)

func agentNoopPublisher(workers.ProgressFragment) {}

func newAgentProvidersFake() *agentProvidersFake {
	return &agentProvidersFake{result: providers.ExecuteResult{
		Content: "fixture output",
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 42,
			Metadata:       map[string]string{"fixture": "detached"},
		},
	}}
}

func (fake *agentProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.mu.Unlock()
	if request.UserMessage == agentFixtureExecutionFailure {
		return providers.ExecuteResult{}, errors.New("deterministic fixture failure")
	}
	return fake.result, nil
}

func (fake *blockingAgentProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	close(fake.entered)
	<-fake.release
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.mu.Unlock()
	return fake.result, nil
}

func (*agentProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*agentProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *agentProvidersFake) Request() providers.ExecuteRequest {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.request.Clone()
}

func agentRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-agent-1",
			WorkerType:      "goal-executor",
			WorkstationName: "execute-goal",
			InputTokens: []any{map[string]any{
				"nested": []any{"dispatch-original"},
			}},
		},
		RunnerID:         string(providers.IDCodex),
		WorkerType:       "goal-executor",
		WorkstationType:  "execute-goal",
		SystemPrompt: "system fixture",
		UserMessage:  "user fixture",
		OutputSchema: `{"type":"object"}`,
		SessionID:    "resume-session-1",
		InputTokens: []any{map[string]any{
			"nested": []any{"original"},
		}},
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot: "prompt",
			Content: []work.WorkContentPart{{
				Type:     work.WorkContentPartTypeText,
				Text:     "original",
				Metadata: map[string]any{"nested": []any{"metadata-original"}},
			}},
		}},
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
		EnvVars:          map[string]string{"FIXTURE": "original"},
		WorkingDirectory: "C:/fixture/work",
		Worktree:         "C:/fixture/worktree",
	}
}

func expectedAgentResult() workers.RunnerExecutionResult {
	return workers.RunnerExecutionResult{
		Content: "fixture output",
		ProviderSession: &workers.ProviderSessionMetadata{
			Provider: string(providers.IDCodex),
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
		Diagnostics: &workers.WorkDiagnostics{
			Provider: &workers.ProviderDiagnostic{
				Provider: string(providers.IDCodex),
				ResponseMetadata: map[string]string{
					"fixture": "detached",
					workers.ProviderResponseMetadataDurationMS: "42",
				},
			},
			Metadata: map[string]string{
				"fixture": "detached",
				workers.ProviderResponseMetadataDurationMS: "42",
			},
		},
	}
}

func assertAgentProviderRequest(t *testing.T, request providers.ExecuteRequest) {
	t.Helper()
	want := providers.ExecuteRequest{
		Provider:         providers.IDCodex,
		AttemptID:        "dispatch-agent-1",
		WorkerType:       "goal-executor",
		WorkstationName:  "execute-goal",
		SystemPrompt:     "system fixture",
		UserMessage:      "user fixture",
		OutputSchema:     `{"type":"object"}`,
		WorkingDirectory: "C:/fixture/work",
		Worktree:         "C:/fixture/worktree",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "resume-session-1",
		},
		EnvVars: map[string]string{"FIXTURE": "original"},
	}
	if !reflect.DeepEqual(request, want) {
		t.Fatalf("Providers.Execute request = %#v, want %#v", request, want)
	}
}

func assertAgentResult(t *testing.T, result workers.RunnerExecutionResult) {
	t.Helper()
	if !reflect.DeepEqual(result, expectedAgentResult()) {
		t.Fatalf("Runner result = %#v, want %#v", result, expectedAgentResult())
	}
}
