package wire

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestNewServiceExecuteRunsScriptInferenceAndAgentAttempts(t *testing.T) {
	t.Parallel()
	fixture := newStatelessTestFixture(t)
	for _, test := range statelessHappyPathCases() {
		t.Run(test.name, func(t *testing.T) {
			assertStatelessHappyPath(t, fixture.service, test)
		})
	}
	if fixture.command.calls.Load() != 1 || fixture.local.calls.Load() != 1 ||
		fixture.provider.executeCalls.Load() != 1 {
		t.Fatalf("attempt effects = command %d model %d provider %d, want one each",
			fixture.command.calls.Load(), fixture.local.calls.Load(), fixture.provider.executeCalls.Load())
	}
}

func TestNewServiceExecuteUsesPerCallTargetSelections(t *testing.T) {
	t.Parallel()
	fixture := newStatelessTestFixture(t)

	for _, command := range []string{"script-a", "script-b"} {
		_, err := fixture.service.Execute(context.Background(), workers.ExecuteRequest{
			Correlation: workers.ExecutionCorrelation{
				DispatchID: command,
				AttemptID:  "attempt-" + command,
			},
			Target: workers.ExecutionTarget{
				WorkerName: "script-worker",
				RunnerID:   runners.ScriptIdentity,
				Command:    command,
				Args:       []string{"--selected", command},
			},
		})
		if err != nil {
			t.Fatalf("script Execute(%q) error = %v", command, err)
		}
	}
	commands := fixture.command.Requests()
	if len(commands) != 2 {
		t.Fatalf("script command requests = %d, want 2", len(commands))
	}
	for index, want := range []string{"script-a", "script-b"} {
		if commands[index].Command != want {
			t.Fatalf("script request[%d].Command = %q, want %q", index, commands[index].Command, want)
		}
		if len(commands[index].Args) != 2 || commands[index].Args[1] != want {
			t.Fatalf("script request[%d].Args = %#v, want selected args", index, commands[index].Args)
		}
	}

	for _, model := range []string{"model-a", "model-b"} {
		_, err := fixture.service.Execute(context.Background(), workers.ExecuteRequest{
			Correlation: workers.ExecutionCorrelation{
				DispatchID: model,
				AttemptID:  "attempt-" + model,
			},
			Target: workers.ExecutionTarget{
				WorkerName: runners.InferenceIdentity,
				RunnerID:   runners.InferenceIdentity,
				Model:      workers.ModelReference{Name: model},
			},
			Input: workers.ExecutionInput{ModelOperation: "generate"},
		})
		if err != nil {
			t.Fatalf("inference Execute(%q) error = %v", model, err)
		}
	}
	models := fixture.local.Requests()
	if len(models) != 2 {
		t.Fatalf("inference requests = %d, want 2", len(models))
	}
	for index, want := range []string{"model-a", "model-b"} {
		if models[index].Worker.Model != want {
			t.Fatalf("inference request[%d].Worker.Model = %q, want %q", index, models[index].Worker.Model, want)
		}
	}
}

func TestNewServiceExecuteNormalizesOutcomeAndOutputContractPolicy(t *testing.T) {
	t.Parallel()
	fixture := newStatelessTestFixture(t)
	base := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-outcome",
			AttemptID:  "attempt-outcome",
		},
		Target: workers.ExecutionTarget{
			WorkerName: runners.AgentIdentity,
			RunnerID:   runners.AgentIdentity,
			Provider: workers.ProviderReference{
				ID: string(providers.IDCodex),
			},
			Prompt: workers.PromptPolicy{UserMessage: "review this result"},
		},
	}
	cases := []struct {
		name    string
		content string
		output  workers.OutputPolicy
		want    workers.ExecutionOutcome
		text    string
	}{
		{
			name:    "accepted",
			content: `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`,
			output:  workers.OutputPolicy{DecisionEnvelope: true},
			want:    workers.ExecutionOutcomeAccepted,
			text:    "ship",
		},
		{
			name:    "continue",
			content: `{"decision":"CONTINUE","feedback":"add tests","output":"next"}`,
			output:  workers.OutputPolicy{DecisionEnvelope: true},
			want:    workers.ExecutionOutcomeContinue,
			text:    "next",
		},
		{
			name:    "rejected",
			content: `{"decision":"REJECTED","feedback":"not ready","output":"stop"}`,
			output:  workers.OutputPolicy{DecisionEnvelope: true},
			want:    workers.ExecutionOutcomeRejected,
			text:    "stop",
		},
		{
			name:    "structured-contract",
			content: `{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"speech","unexpected_speech":false,"verdict":"pass","confidence":0.9}`,
			output:  workers.OutputPolicy{Contract: "structured-clip-qa/v1"},
			want:    workers.ExecutionOutcomeAccepted,
			text:    `{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"speech","unexpected_speech":false,"verdict":"pass","confidence":0.9}`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture.provider.SetContent(test.content)
			request := base.Clone()
			request.Target.Output = test.output
			result, err := fixture.service.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Outcome != test.want {
				t.Fatalf("outcome = %q, failure = %#v, output = %#v, want %q", result.Outcome, result.Failure, result.Output, test.want)
			}
			if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != test.text {
				t.Fatalf("output = %#v, want %q", result.Output, test.text)
			}
		})
	}
}

type statelessTestFixture struct {
	service  workers.Service
	provider *statelessTestProviders
	command  *statelessTestCommandRunner
	local    *statelessTestLocalInvoker
}

func newStatelessTestFixture(t *testing.T) statelessTestFixture {
	t.Helper()

	provider := &statelessTestProviders{}
	command := &statelessTestCommandRunner{}
	local := &statelessTestLocalInvoker{}
	service, err := NewService(
		runners.AgentDependencies{
			Providers: provider,
			Publish:   func(workers.ProgressFragment) {},
		},
		runners.ScriptConfig{
			Command:          "fixture-script",
			FactoryDirectory: "factory-root",
		},
		runners.ScriptDependencies{
			CommandRunner: command,
			FactoryDocs:   func(string) (map[string]string, error) { return nil, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:  "local-inference",
				Type:  factorydefinitions.WorkerTypeInference,
				Model: "local-model",
			},
		},
		runners.InferenceDependencies{Models: local},
		nil,
		nil,
		func() time.Time { return time.Unix(1, 0) },
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if command.calls.Load() != 0 || local.calls.Load() != 0 || provider.executeCalls.Load() != 0 {
		t.Fatalf("construction effects = command %d model %d provider %d, want zero",
			command.calls.Load(),
			local.calls.Load(),
			provider.executeCalls.Load(),
		)
	}
	return statelessTestFixture{service: service, provider: provider, command: command, local: local}
}

func TestNewServiceRejectsMissingConstructionPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*statelessConstructionInputs)
	}{
		{name: "agent providers", mutate: func(input *statelessConstructionInputs) { input.agentDependencies.Providers = nil }},
		{name: "agent publisher", mutate: func(input *statelessConstructionInputs) { input.agentDependencies.Publish = nil }},
		{name: "script command", mutate: func(input *statelessConstructionInputs) {
			input.scriptConfig.Command = ""
			input.scriptConfig.RequestSelected = false
		}},
		{name: "script command runner", mutate: func(input *statelessConstructionInputs) { input.scriptDependencies.CommandRunner = nil }},
		{name: "script factory docs", mutate: func(input *statelessConstructionInputs) { input.scriptDependencies.FactoryDocs = nil }},
		{name: "script clock", mutate: func(input *statelessConstructionInputs) { input.scriptDependencies.Now = nil }},
		{name: "script publisher", mutate: func(input *statelessConstructionInputs) { input.scriptDependencies.Publish = nil }},
		{name: "script recorder", mutate: func(input *statelessConstructionInputs) { input.scriptDependencies.Record = nil }},
		{name: "inference worker", mutate: func(input *statelessConstructionInputs) { input.inferenceConfig.Worker.Name = "" }},
		{name: "inference models", mutate: func(input *statelessConstructionInputs) { input.inferenceDependencies.Models = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := newStatelessConstructionInputs()
			test.mutate(&input)
			if _, err := NewService(
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
			); err == nil {
				t.Fatal("NewService() error = nil, want construction validation error")
			}
		})
	}
}

func TestNewServiceRejectsMissingExecuteClock(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	if _, err := NewService(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	); err == nil {
		t.Fatal("NewService() error = nil, want missing Execute clock error")
	}
}

type statelessConstructionInputs struct {
	agentDependencies     runners.AgentDependencies
	scriptConfig          runners.ScriptConfig
	scriptDependencies    runners.ScriptDependencies
	inferenceConfig       runners.InferenceConfig
	inferenceDependencies runners.InferenceDependencies
}

func newStatelessConstructionInputs() statelessConstructionInputs {
	return statelessConstructionInputs{
		agentDependencies: runners.AgentDependencies{
			Providers: &statelessTestProviders{},
			Publish:   func(workers.ProgressFragment) {},
		},
		scriptConfig: runners.ScriptConfig{
			Command:          "fixture-script",
			FactoryDirectory: "factory-root",
		},
		scriptDependencies: runners.ScriptDependencies{
			CommandRunner: &statelessTestCommandRunner{},
			FactoryDocs:   func(string) (map[string]string, error) { return nil, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		inferenceConfig: runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:  "local-inference",
				Type:  factorydefinitions.WorkerTypeInference,
				Model: "local-model",
			},
		},
		inferenceDependencies: runners.InferenceDependencies{
			Models: &statelessTestLocalInvoker{},
		},
	}
}

type statelessHappyPathCase struct {
	name    string
	request workers.ExecuteRequest
	want    string
}

func statelessHappyPathCases() []statelessHappyPathCase {
	return []statelessHappyPathCase{
		{
			name: "script",
			request: workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{
					DispatchID: "dispatch-script",
					AttemptID:  "attempt-script",
				},
				Target: workers.ExecutionTarget{
					WorkerName:      "script-worker",
					WorkstationName: "script-workstation",
					RunnerID:        runners.ScriptIdentity,
				},
			},
			want: "script-output",
		},
		{
			name: "inference",
			request: workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{
					DispatchID: "dispatch-inference",
					AttemptID:  "attempt-inference",
				},
				Target: workers.ExecutionTarget{
					WorkerName: runners.InferenceIdentity,
					RunnerID:   runners.InferenceIdentity,
					Model: workers.ModelReference{
						Name: "local-model",
					},
				},
				Input: workers.ExecutionInput{
					ModelOperation: "generate",
				},
			},
			want: "inference-output",
		},
		{
			name: "agent",
			request: workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{
					DispatchID: "dispatch-agent",
					AttemptID:  "attempt-agent",
				},
				Target: workers.ExecutionTarget{
					WorkerName: runners.AgentIdentity,
					RunnerID:   runners.AgentIdentity,
					Provider: workers.ProviderReference{
						ID: string(providers.IDCodex),
					},
					Prompt: workers.PromptPolicy{
						UserMessage: "agent prompt",
					},
				},
			},
			want: "agent-output",
		},
	}
}

func assertStatelessHappyPath(
	t *testing.T,
	service workers.Service,
	test statelessHappyPathCase,
) {
	t.Helper()
	result, err := service.Execute(context.Background(), test.request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Correlation != test.request.Correlation {
		t.Fatalf("correlation = %#v, want %#v", result.Correlation, test.request.Correlation)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != test.want {
		t.Fatalf("output = %#v, want %q", result.Output, test.want)
	}
}

type statelessTestCommandRunner struct {
	calls atomic.Int32
	mu    sync.Mutex
	seen  []workers.CommandRequest
}

func (runner *statelessTestCommandRunner) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	runner.record(request)
	_ = ctx
	return workers.CommandResult{Stdout: []byte("script-output")}, nil
}

func (runner *statelessTestCommandRunner) RunStreaming(
	ctx context.Context,
	request workers.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	runner.record(request)
	_ = ctx
	return workers.CommandResult{Stdout: []byte("script-output")}, nil
}

func (runner *statelessTestCommandRunner) record(request workers.CommandRequest) {
	runner.calls.Add(1)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.seen = append(runner.seen, workers.CloneSubprocessExecutionRequest(request))
}

func (runner *statelessTestCommandRunner) Requests() []workers.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	result := make([]workers.CommandRequest, len(runner.seen))
	for index, request := range runner.seen {
		result[index] = workers.CloneSubprocessExecutionRequest(request)
	}
	return result
}

type statelessTestLocalInvoker struct {
	calls atomic.Int32
	mu    sync.Mutex
	seen  []models.LocalInvocationRequest
}

func (invoker *statelessTestLocalInvoker) InvokeLocal(
	ctx context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	invoker.calls.Add(1)
	invoker.mu.Lock()
	invoker.seen = append(invoker.seen, request)
	invoker.mu.Unlock()
	_ = ctx
	return models.LocalInvocationResult{
		Handled: true,
		Content: "inference-output",
	}, nil
}

func (invoker *statelessTestLocalInvoker) Requests() []models.LocalInvocationRequest {
	invoker.mu.Lock()
	defer invoker.mu.Unlock()
	return append([]models.LocalInvocationRequest(nil), invoker.seen...)
}

type statelessTestProviders struct {
	providers.Service
	executeCalls atomic.Int32
	mu           sync.Mutex
	content      string
}

func (provider *statelessTestProviders) SetContent(content string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.content = content
}

func (*statelessTestProviders) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	switch strings.ToLower(strings.TrimSpace(request.Identity)) {
	case "codex", "openai":
		return providers.ResolveIdentityResult{ID: providers.IDCodex}, nil
	default:
		return providers.ResolveIdentityResult{}, providers.ErrUnknownProvider
	}
}

func (*statelessTestProviders) ValidatePrerequisites(
	_ context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if request.ID != providers.IDCodex {
		return providers.ErrUnknownProvider
	}
	return nil
}

func (provider *statelessTestProviders) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	provider.executeCalls.Add(1)
	provider.mu.Lock()
	content := provider.content
	provider.mu.Unlock()
	if content == "" {
		content = "agent-output"
	}
	return providers.ExecuteResult{
		Content: content,
		SessionRef: &providers.SessionRef{
			Provider: request.Provider,
			Kind:     providers.SessionIDKind,
			ID:       "session-" + request.AttemptID,
		},
	}, nil
}

var _ providers.Service = (*statelessTestProviders)(nil)
