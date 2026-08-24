package wire

import (
	"context"
	"errors"
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

func TestNewServiceExecuteManagedInferenceUsesModelsDespiteProviderRunner(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	local := &statelessTestLocalInvoker{}
	delegate := &statelessInferenceDelegate{}
	input.inferenceDependencies = runners.InferenceDependencies{
		Models:   local,
		Delegate: delegate,
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
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-inference",
			RuntimeID:        "runtime-inference",
			GenerationID:     "generation-inference",
			DispatchID:       "dispatch-inference",
			AttemptID:        "attempt-inference",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "selected-inference-worker",
			WorkerType: factorydefinitions.WorkerTypeInference,
			RunnerID:   workers.RunnerIDCodex,
			Provider:   workers.ProviderReference{ID: workers.RunnerIDCodex},
			Model: workers.ModelReference{
				Name:     "selected-model",
				Provider: workers.RunnerIDCodex,
				Locality: models.RuntimeModelLocalityLocal,
			},
		},
		Input: workers.ExecutionInput{ModelOperation: "invoke"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || result.Output.Primary[0].Text != "inference-output" {
		t.Fatalf("Execute() result = %#v, want accepted Models output", result)
	}
	if local.calls.Load() != 1 {
		t.Fatalf("Models calls = %d, want one generic invocation", local.calls.Load())
	}
	if delegate.request.RunnerID != "" {
		t.Fatalf("delegate request = %#v, want no provider fallback after Models success", delegate.request)
	}
}

func TestNewServiceExecuteUsesPerCallTargetSelections(t *testing.T) {
	t.Parallel()
	fixture := newStatelessTestFixture(t)

	for _, command := range []string{"script-a", "script-b"} {
		_, err := fixture.service.Execute(context.Background(), workers.ExecuteRequest{
			Correlation: workers.ExecutionCorrelation{
				FactorySessionID: "session-stateless",
				RuntimeID:        "runtime-stateless",
				GenerationID:     "generation-stateless",
				DispatchID:       command,
				AttemptID:        "attempt-" + command,
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
				FactorySessionID: "session-stateless",
				RuntimeID:        "runtime-stateless",
				GenerationID:     "generation-stateless",
				DispatchID:       model,
				AttemptID:        "attempt-" + model,
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
		if models[index].Model.NameOrURI != want {
			t.Fatalf("inference request[%d].Model = %q, want %q", index, models[index].Model.NameOrURI, want)
		}
	}
}

func TestNewServiceExecuteNormalizesOutcomeAndOutputContractPolicy(t *testing.T) {
	t.Parallel()
	fixture := newStatelessTestFixture(t)
	base := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-stateless",
			RuntimeID:        "runtime-stateless",
			GenerationID:     "generation-stateless",
			DispatchID:       "dispatch-outcome",
			AttemptID:        "attempt-outcome",
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

func TestNewMockServiceExecutesMockThroughCanonicalWorkersBehavior(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	config := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "mock-worker",
		RunType:    workers.MockWorkerRunTypeAccept,
	}}}
	var observations []workers.ExecutionObservation
	service, err := NewMockService(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		config,
		MockDependencies{},
		func(_ context.Context, observation workers.ExecutionObservation) error {
			observations = append(observations, observation)
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
		t.Fatalf("NewMockService() error = %v", err)
	}

	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-mock",
			RuntimeID:        "runtime-mock",
			GenerationID:     "generation-mock",
			DispatchID:       "dispatch-mock",
			AttemptID:        "attempt-mock",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "mock-worker",
			RunnerID:   "mock",
		},
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("mock Execute() error = %v", err)
	}
	if result.Correlation != request.Correlation {
		t.Fatalf("mock correlation = %#v, want %#v", result.Correlation, request.Correlation)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || len(result.Output.Primary) != 1 ||
		result.Output.Primary[0].Text != "mock worker accepted" {
		t.Fatalf("mock result = %#v, want accepted canonical output", result)
	}
	if len(observations) != 2 || observations[0].Kind != workers.ExecutionObservationKindStarted ||
		observations[1].Kind != workers.ExecutionObservationKindCompleted {
		t.Fatalf("mock observations = %#v, want started and completed", observations)
	}
	for index, observation := range observations {
		if observation.Correlation != request.Correlation {
			t.Fatalf("mock observation[%d].Correlation = %#v, want %#v", index, observation.Correlation, request.Correlation)
		}
	}
}

func TestNewMockServiceRequiresExplicitMockComposition(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	if _, err := NewMockService(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		nil,
		MockDependencies{},
		nil,
		nil,
		func() time.Time { return time.Unix(1, 0) },
		nil,
		nil,
		nil,
		nil,
	); err == nil {
		t.Fatal("NewMockService() error = nil, want explicit mock configuration error")
	}
}

func TestNewServiceRejectsMockSelectionWithoutExplicitMockComposition(t *testing.T) {
	t.Parallel()

	fixture := newStatelessTestFixture(t)
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-production-mock",
			RuntimeID:        "runtime-production-mock",
			GenerationID:     "generation-production-mock",
			DispatchID:       "dispatch-production-mock",
			AttemptID:        "attempt-production-mock",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "mock-worker",
			RunnerID:   runners.MockIdentity,
		},
		Input: workers.ExecutionInput{
			MockWorkers: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
				WorkerName: "mock-worker",
				RunType:    workers.MockWorkerRunTypeAccept,
			}}},
		},
	}

	result, err := fixture.service.Execute(context.Background(), request)
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("production mock Execute() error = %v, want invalid execute request", err)
	}
	if result.Correlation != (workers.ExecutionCorrelation{}) ||
		result.Outcome != "" || len(result.Output.Primary) != 0 || result.Failure != nil {
		t.Fatalf("production mock result = %#v, want no started result", result)
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
			Providers:         provider,
			Publish:           func(workers.ProgressFragment) {},
			DecisionEnvelopes: statelessDecisionEnvelopeDouble{},
		},
		runners.ScriptConfig{
			Command:          "fixture-script",
			FactoryDirectory: "factory-root",
		},
		ScriptDependencies{
			CommandRunner: command,
			FactoryDocs:   func(string) (map[string]string, error) { return nil, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:          "local-inference",
				Type:          factorydefinitions.WorkerTypeInference,
				Model:         "local-model",
				ModelLocality: models.RuntimeModelLocalityLocal,
			},
			Scope: statelessTestScope(),
		},
		runners.InferenceDependencies{Models: local},
		nil,
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
		nil,
	); err == nil {
		t.Fatal("NewService() error = nil, want missing Execute clock error")
	}
}

type statelessConstructionInputs struct {
	agentDependencies     runners.AgentDependencies
	scriptConfig          runners.ScriptConfig
	scriptDependencies    ScriptDependencies
	inferenceConfig       runners.InferenceConfig
	inferenceDependencies runners.InferenceDependencies
}

func statelessTestScope() models.RuntimeScopeRef {
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:stateless-tests")
	if err != nil {
		panic(err)
	}
	return scope
}

func newStatelessConstructionInputs() statelessConstructionInputs {
	return statelessConstructionInputs{
		agentDependencies: runners.AgentDependencies{
			Providers:         &statelessTestProviders{},
			Publish:           func(workers.ProgressFragment) {},
			DecisionEnvelopes: statelessDecisionEnvelopeDouble{},
		},
		scriptConfig: runners.ScriptConfig{
			Command:          "fixture-script",
			FactoryDirectory: "factory-root",
		},
		scriptDependencies: ScriptDependencies{
			CommandRunner: &statelessTestCommandRunner{},
			FactoryDocs:   func(string) (map[string]string, error) { return nil, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		inferenceConfig: runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:          "local-inference",
				Type:          factorydefinitions.WorkerTypeInference,
				Model:         "local-model",
				ModelLocality: models.RuntimeModelLocalityLocal,
			},
			Scope: statelessTestScope(),
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
					FactorySessionID: "session-stateless",
					RuntimeID:        "runtime-stateless",
					GenerationID:     "generation-stateless",
					DispatchID:       "dispatch-script",
					AttemptID:        "attempt-script",
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
					FactorySessionID: "session-stateless",
					RuntimeID:        "runtime-stateless",
					GenerationID:     "generation-stateless",
					DispatchID:       "dispatch-inference",
					AttemptID:        "attempt-inference",
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
					FactorySessionID: "session-stateless",
					RuntimeID:        "runtime-stateless",
					GenerationID:     "generation-stateless",
					DispatchID:       "dispatch-agent",
					AttemptID:        "attempt-agent",
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
	seen  []platformprocess.CommandRequest
}

func (runner *statelessTestCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.record(request)
	_ = ctx
	return platformprocess.CommandResult{Stdout: []byte("script-output")}, nil
}

func (runner *statelessTestCommandRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	runner.record(request)
	_ = ctx
	return platformprocess.CommandResult{Stdout: []byte("script-output")}, nil
}

func (runner *statelessTestCommandRunner) record(request platformprocess.CommandRequest) {
	runner.calls.Add(1)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.seen = append(runner.seen, clonePlatformCommandRequest(request))
}

func (runner *statelessTestCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	result := make([]platformprocess.CommandRequest, len(runner.seen))
	for index, request := range runner.seen {
		result[index] = clonePlatformCommandRequest(request)
	}
	return result
}

func clonePlatformCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	clone := request
	clone.Args = append([]string(nil), request.Args...)
	clone.Stdin = append([]byte(nil), request.Stdin...)
	clone.Env = append([]string(nil), request.Env...)
	return clone
}

type statelessTestLocalInvoker struct {
	calls atomic.Int32
	mu    sync.Mutex
	seen  []models.InvokeModelRequest
}

type statelessInferenceDelegate struct {
	request workers.RunnerExecutionRequest
}

func (delegate *statelessInferenceDelegate) Execute(
	_ context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	delegate.request = workers.CloneProviderInferenceRequest(request)
	return workers.RunnerExecutionResult{Content: "delegate-output"}, nil
}

func (invoker *statelessTestLocalInvoker) InvokeModel(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	invoker.calls.Add(1)
	invoker.mu.Lock()
	invoker.seen = append(invoker.seen, request)
	invoker.mu.Unlock()
	_ = ctx
	return models.InvokeModelResult{
		Status: models.ModelInvocationStatusCompleted,
		Outputs: []models.InferenceOutput{{
			Name: "text", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: "inference-output",
		}},
	}, nil
}

func (invoker *statelessTestLocalInvoker) Requests() []models.InvokeModelRequest {
	invoker.mu.Lock()
	defer invoker.mu.Unlock()
	return append([]models.InvokeModelRequest(nil), invoker.seen...)
}

type statelessTestProviders struct {
	statelessProviderContract
	executeCalls atomic.Int32
	mu           sync.Mutex
	content      string
	requests     []providers.ExecuteRequest
}

type statelessProviderOverride struct {
	statelessProviderContract
	calls   atomic.Int32
	mu      sync.Mutex
	request providers.ExecuteRequest
	content string
}

func (provider *statelessProviderOverride) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	identity := strings.ToLower(strings.TrimSpace(request.Identity))
	if identity == "" {
		identity = string(providers.IDCodex)
	}
	switch identity {
	case string(providers.IDCodex), "openai":
		return providers.ResolveIdentityResult{ID: providers.IDCodex}, nil
	default:
		return providers.ResolveIdentityResult{}, providers.ErrUnknownProvider
	}
}

func (provider *statelessProviderOverride) ValidatePrerequisites(
	_ context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if request.ID == "" {
		request.ID = providers.IDCodex
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if request.ID != providers.IDCodex {
		return providers.ErrUnknownProvider
	}
	return nil
}

func (provider *statelessProviderOverride) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return providers.ExecuteResult{}, err
	}
	provider.calls.Add(1)
	provider.mu.Lock()
	provider.request = request.Clone()
	content := provider.content
	provider.mu.Unlock()
	if content == "" {
		content = "override output\n<COMPLETE>"
	}
	return providers.ExecuteResult{
		Content: content,
		SessionRef: &providers.SessionRef{
			Provider: request.Provider,
			Kind:     providers.SessionIDKind,
			ID:       "session-attempt-override",
		},
	}, nil
}

func (provider *statelessProviderOverride) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	result, err := provider.Execute(ctx, request.Attempt)
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    result,
	}, nil
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
	provider.requests = append(provider.requests, request.Clone())
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

func (provider *statelessTestProviders) Requests() []providers.ExecuteRequest {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	requests := make([]providers.ExecuteRequest, len(provider.requests))
	for index, request := range provider.requests {
		requests[index] = request.Clone()
	}
	return requests
}

var _ providers.Service = (*statelessTestProviders)(nil)

// statelessDecisionEnvelopeDouble stands in for the Factory Definitions owner of
// decision-envelope interpretation. Composition tests assert that the stateless
// Workers root consults the injected owner; the envelope grammar itself is owned
// and tested inside invocation policy, so this double maps only the exact
// payloads these cases send.
type statelessDecisionEnvelopeDouble struct{}

func (statelessDecisionEnvelopeDouble) UsesDecisionEnvelopeOutcome(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) bool {
	return workstation != nil &&
		workstation.OutcomeFormat == factorydefinitions.DecisionEnvelopeOutcomeFormat
}

func (statelessDecisionEnvelopeDouble) UsesGoalRoutingDecisionEnvelope(
	*factorydefinitions.FactoryWorkstationConfig,
) bool {
	return false
}

func (statelessDecisionEnvelopeDouble) WorkResultFromDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workers.WorkResult {
	result := workers.WorkResult{DispatchID: dispatchID, TransitionID: transitionID}
	switch strings.TrimSpace(raw) {
	case `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`:
		result.Outcome = workers.OutcomeAccepted
		result.Feedback = "ready"
		result.Output = "ship"
	case `{"decision":"CONTINUE","feedback":"add tests","output":"next"}`:
		result.Outcome = workers.OutcomeContinue
		result.Feedback = "add tests"
		result.Output = "next"
	case `{"decision":"REJECTED","feedback":"not ready","output":"stop"}`:
		result.Outcome = workers.OutcomeRejected
		result.Feedback = "not ready"
		result.Output = "stop"
	default:
		result.Outcome = factorydefinitions.MalformedEnvelopeFailureOutcome
		result.Error = "reviewer decision envelope invalid"
	}
	return result
}

func (double statelessDecisionEnvelopeDouble) WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workers.WorkResult {
	result := double.WorkResultFromDecisionEnvelopeJSONOrFailed(dispatchID, transitionID, raw)
	if result.Outcome == workers.OutcomeAccepted {
		result.SelectedClassificationLabel = "accepted"
	}
	return result
}
