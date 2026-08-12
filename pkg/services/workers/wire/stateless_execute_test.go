package wire

import (
	"context"
	"strings"
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
}

func (runner *statelessTestCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	runner.calls.Add(1)
	return workers.CommandResult{Stdout: []byte("script-output")}, nil
}

func (runner *statelessTestCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	runner.calls.Add(1)
	return workers.CommandResult{Stdout: []byte("script-output")}, nil
}

type statelessTestLocalInvoker struct {
	calls atomic.Int32
}

func (invoker *statelessTestLocalInvoker) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	invoker.calls.Add(1)
	return models.LocalInvocationResult{
		Handled: true,
		Content: "inference-output",
	}, nil
}

type statelessTestProviders struct {
	providers.Service
	executeCalls atomic.Int32
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
	return providers.ExecuteResult{
		Content: "agent-output",
		SessionRef: &providers.SessionRef{
			Provider: request.Provider,
			Kind:     providers.SessionIDKind,
			ID:       "session-" + request.AttemptID,
		},
	}, nil
}

var _ providers.Service = (*statelessTestProviders)(nil)
