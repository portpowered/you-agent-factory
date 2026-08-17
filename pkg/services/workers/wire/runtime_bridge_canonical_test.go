package wire

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

func TestCanonicalRuntimeBridgeHelpers(t *testing.T) {
	t.Parallel()

	hooks := LocalRuntimeHooks()
	if hooks.MarkLoadFinished == nil || hooks.MarkLoadRequested == nil {
		t.Fatal("LocalRuntimeHooks() returned incomplete model recording hooks")
	}
	if NewMockCommandRunner(nil, nil, stubCanonicalCommandRunner{}) == nil {
		t.Fatal("NewMockCommandRunner() returned nil")
	}

	resolved, err := ResolveTemplateFields(
		"{{.Context.WorkDir}}/sub",
		map[string]string{"NAME": "{{.Context.WorkDir}}"},
		nil,
		&workers.Context{WorkDirectory: "reviewer"},
		"",
	)
	if err != nil {
		t.Fatalf("ResolveTemplateFields() error = %v", err)
	}
	if resolved.WorkingDirectory != "reviewer/sub" || resolved.Env["NAME"] != "reviewer" {
		t.Fatalf("resolved fields = %#v", resolved)
	}
}

func TestCanonicalMockCommandRunnerUsesDetachedOverride(t *testing.T) {
	t.Parallel()

	runner := NewContextualMockWorkerCommandRunner(canonicalCommandRunnerFunc(
		func(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
			return workers.CommandResult{Stdout: []byte("forwarded " + request.Command)}, nil
		},
	))
	result, err := runner.Run(context.Background(), workers.CommandRequest{Command: "codex"})
	if err != nil || string(result.Stdout) != "forwarded codex" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}

	configured := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeAccept}}}
	ctx := workerexecution.WithMockWorkerOutputPolicy(
		workerexecution.WithMockWorkersConfig(context.Background(), configured),
		workers.OutputPolicy{DecisionEnvelope: true},
	)
	result, err = runner.Run(ctx, workers.CommandRequest{Command: "codex"})
	if err != nil || !strings.Contains(string(result.Stdout), "ACCEPTED") {
		t.Fatalf("Run(mock) = %#v, %v", result, err)
	}
}

type stubCanonicalCommandRunner struct{}

func (stubCanonicalCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (stubCanonicalCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

type canonicalCommandRunnerFunc func(context.Context, workers.CommandRequest) (workers.CommandResult, error)

func (runner canonicalCommandRunnerFunc) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	return runner(ctx, request)
}
