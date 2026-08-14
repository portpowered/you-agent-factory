package wire

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution"
)

func TestRuntimeBridgeLocalRuntimeHooksConstructs(t *testing.T) {
	t.Parallel()

	_ = LocalRuntimeHooks()
}

func TestRuntimeBridgeCompatibilityConstructorsValidateDelegatedInputs(t *testing.T) {
	t.Parallel()

	if _, err := NewConfiguredRuntime(
		nil,
		nil,
		models.RuntimeScopeRef{},
		nil,
		nil,
		nil,
		nil,
		nil,
		false,
		"",
		"",
		"",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		false,
		false,
		false,
		nil,
		nil,
		nil,
	); err == nil {
		t.Fatal("NewConfiguredRuntime() error = nil, want delegated construction error")
	}
	if _, err := BuildRuntimeExecutors(nil, nil, nil, "", nil, nil, false, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("BuildRuntimeExecutors() error = nil, want unsupported runtime error")
	}
	if _, err := NewInvocation(nil, nil, nil, nil, nil, nil, nil, nil, ""); err == nil {
		t.Fatal("NewInvocation() error = nil, want delegated validation error")
	}
	if _, err := NewInvocationWithProgress(nil, nil, nil, nil, nil, nil, nil, nil, "", nil); err == nil {
		t.Fatal("NewInvocationWithProgress() error = nil, want delegated validation error")
	}
	if _, err := NewProviderFromCommandRunner(nil, nil, nil, nil, nil, nil, nil, nil, ""); err == nil {
		t.Fatal("NewProviderFromCommandRunner() error = nil, want delegated provider error")
	}
}

func TestDefaultBindingAssemblerPreservesRoleAndSelection(t *testing.T) {
	t.Parallel()

	role := workers.RuntimeBuildRoleRequest{
		Name: "reviewer",
		Kind: workers.RuntimeBuildRoleKindWorker,
	}
	selection := workers.ResolvedRunnerSelection{
		RunnerID: workers.RunnerIDCodex,
		Source:   workers.RunnerSelectionSourceFactory,
	}
	binding, err := defaultBindingAssembler(
		context.Background(),
		role,
		workers.RuntimeBuildOpeningOptions{},
		selection,
	)
	if err != nil {
		t.Fatalf("defaultBindingAssembler() error = %v", err)
	}
	if binding.RoleName != role.Name || binding.RoleKind != role.Kind || binding.RunnerSelection != selection {
		t.Fatalf("binding = %#v, want role and selection preserved", binding)
	}
}

func TestRuntimeBridgeNewMockCommandRunnerDecoratesNext(t *testing.T) {
	t.Parallel()

	next := stubRuntimeBridgeCommandRunner{}
	decorated := NewMockCommandRunner(nil, nil, next)
	if decorated == nil {
		t.Fatal("NewMockCommandRunner() returned nil")
	}
}

func TestContextualMockWorkerCommandRunnerUsesDetachedOverrideAndStreaming(t *testing.T) {
	t.Parallel()

	next := contextualMockCommandRunnerFunc(func(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
		return workers.CommandResult{Stdout: []byte("forwarded " + request.Command), Stderr: []byte("warning")}, nil
	})
	runner := NewContextualMockWorkerCommandRunner(next)
	request := workers.CommandRequest{Command: "codex", WorkerType: "worker", WorkstationName: "station"}
	result, err := runner.Run(context.Background(), request)
	if err != nil || string(result.Stdout) != "forwarded codex" {
		t.Fatalf("Run(without override) = %#v, %v; want forwarded command", result, err)
	}

	config := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "worker", WorkstationName: "station", RunType: workers.MockWorkerRunTypeAccept,
	}}}
	ctx := workerexecution.WithMockWorkerOutputPolicy(
		workerexecution.WithMockWorkersConfig(context.Background(), config),
		workers.OutputPolicy{DecisionEnvelope: true},
	)
	result, err = runner.Run(ctx, request)
	if err != nil || !strings.Contains(string(result.Stdout), "ACCEPTED") {
		t.Fatalf("Run(with override) = %#v, %v, want decision envelope", result, err)
	}
	var streams []string
	streaming, ok := runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	})
	if !ok {
		t.Fatal("contextual runner does not expose RunStreaming")
	}
	if _, err := streaming.RunStreaming(ctx, request, func(_ string, chunk []byte) {
		streams = append(streams, string(chunk))
	}); err != nil || len(streams) != 1 {
		t.Fatalf("RunStreaming(with override) error=%v streams=%#v, want one output chunk", err, streams)
	}
}

func TestContextualMockWorkerCommandRunnerFailsClosedAndPublishesFallbackOutput(t *testing.T) {
	t.Parallel()

	request := workers.CommandRequest{Command: "codex"}
	withoutNext := NewContextualMockWorkerCommandRunner(nil)
	if _, err := withoutNext.Run(context.Background(), request); err == nil {
		t.Fatal("Run(nil next) error = nil")
	}
	streamingWithoutNext, ok := withoutNext.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	})
	if !ok {
		t.Fatal("contextual runner does not expose RunStreaming")
	}
	if _, err := streamingWithoutNext.RunStreaming(context.Background(), request, nil); err == nil {
		t.Fatal("RunStreaming(nil next) error = nil")
	}

	wantErr := errors.New("next failed")
	next := contextualMockCommandRunnerFunc(func(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
		return workers.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr")}, wantErr
	})
	runner := NewContextualMockWorkerCommandRunner(next)
	var streams []string
	result, err := runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	}).RunStreaming(context.Background(), request, func(stream string, chunk []byte) {
		streams = append(streams, stream+":"+string(chunk))
	})
	if !errors.Is(err, wantErr) || string(result.Stdout) != "stdout" || len(streams) != 2 {
		t.Fatalf("RunStreaming(fallback) = %#v, %v, streams=%#v", result, err, streams)
	}
	if _, err := runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	}).RunStreaming(context.Background(), request, nil); !errors.Is(err, wantErr) {
		t.Fatalf("RunStreaming(nil observer) error = %v, want %v", err, wantErr)
	}

	direct := NewContextualMockWorkerCommandRunner(streamingContextualMockRunner{})
	streamingDirect, ok := direct.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	})
	if !ok {
		t.Fatal("contextual runner does not expose direct streaming")
	}
	if result, err := streamingDirect.RunStreaming(context.Background(), request, nil); err != nil || string(result.Stdout) != "direct" {
		t.Fatalf("RunStreaming(direct) = %#v, %v", result, err)
	}

	config := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeAccept}}}
	configured := workerexecution.WithMockWorkersConfig(context.Background(), config)
	if _, err := streamingDirect.RunStreaming(configured, request, nil); err != nil {
		t.Fatalf("RunStreaming(configured, nil observer) error = %v", err)
	}
}

func TestResolveTemplateFieldsResolvesTemplates(t *testing.T) {
	t.Parallel()

	resolved, err := ResolveTemplateFields(
		"{{.Context.WorkDir}}/sub",
		map[string]string{"NAME": "{{.Context.WorkDir}}"},
		nil,
		&workers.Context{WorkDirectory: "reviewer"},
		"",
	)
	if err != nil {
		t.Fatalf("ResolveTemplateFields() = %v", err)
	}
	if resolved.WorkingDirectory != "reviewer/sub" {
		t.Fatalf("WorkingDirectory = %q, want reviewer/sub", resolved.WorkingDirectory)
	}
	if resolved.Env["NAME"] != "reviewer" {
		t.Fatalf("Env[NAME] = %q, want reviewer", resolved.Env["NAME"])
	}
}

func TestResolveTemplateFieldsPropagatesTemplateError(t *testing.T) {
	t.Parallel()

	if _, err := ResolveTemplateFields("{{.Bogus", nil, nil, &workers.Context{}, ""); err == nil {
		t.Fatal("ResolveTemplateFields() with malformed template = nil error, want non-nil")
	}
}

type stubRuntimeBridgeCommandRunner struct{}

type contextualMockCommandRunnerFunc func(context.Context, workers.CommandRequest) (workers.CommandResult, error)

func (runner contextualMockCommandRunnerFunc) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	return runner(ctx, request)
}

type streamingContextualMockRunner struct{}

func (streamingContextualMockRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("direct")}, nil
}

func (streamingContextualMockRunner) RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("direct")}, nil
}

func (stubRuntimeBridgeCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (stubRuntimeBridgeCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}
