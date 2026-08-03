package wire

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRuntimeBridgeLocalRuntimeHooksConstructs(t *testing.T) {
	t.Parallel()

	_ = LocalRuntimeHooks()
}

func TestRuntimeBridgeNewMockCommandRunnerDecoratesNext(t *testing.T) {
	t.Parallel()

	next := stubRuntimeBridgeCommandRunner{}
	decorated := NewMockCommandRunner(nil, nil, next)
	if decorated == nil {
		t.Fatal("NewMockCommandRunner() returned nil")
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
