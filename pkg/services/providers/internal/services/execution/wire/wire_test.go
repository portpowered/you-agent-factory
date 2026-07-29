package wire

import (
	"context"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type recordingWorkersRunner struct {
	calls int
}

func (r *recordingWorkersRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	r.calls++
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

type recordingPlatformRunner struct {
	calls int
}

func (r *recordingPlatformRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{Stdout: []byte("ok")}, nil
}

func TestBuiltInDependenciesFromWorkersRunnerConstructsCodexClaudeAndCursorEffects(t *testing.T) {
	t.Parallel()

	runner := &recordingWorkersRunner{}
	deps := BuiltInDependenciesFromWorkersRunner(runner)
	if deps.Codex == nil || deps.Claude == nil || deps.Cursor == nil {
		t.Fatalf("built-in dependencies = %#v, want codex, claude, and cursor effects", deps)
	}
	if deps.Antigravity != nil {
		t.Fatalf("built-in Antigravity effect = %#v, want nil without PTY platform dependencies", deps.Antigravity)
	}
}

func TestBuiltInDependenciesFromRunnerAdaptsPlatformRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{}
	deps := BuiltInDependenciesFromRunner(runner)
	if deps.Codex == nil || deps.Claude == nil || deps.Cursor == nil {
		t.Fatalf("built-in dependencies = %#v, want codex, claude, and cursor effects", deps)
	}
	if deps.Antigravity != nil {
		t.Fatalf("built-in Antigravity effect = %#v, want nil without PTY platform dependencies", deps.Antigravity)
	}
}

func TestBuiltInDependenciesFromWorkersRunnerConstructsAgyPTYEffectWithAllocator(t *testing.T) {
	t.Parallel()

	runner := &recordingWorkersRunner{}
	deps := BuiltInDependenciesFromWorkersRunner(runner, BuiltInRunnerPlatformDependencies{
		AgyPTY: AgyPTYPlatformDependencies{
			Allocator: &agypty.MockAllocator{},
			Locator:   platformprocess.HostExecutableLocator{},
			Inspector: platformfilesystem.Local{},
		},
	})
	if deps.Antigravity == nil {
		t.Fatalf("built-in Agy effect = nil, want PTY effect when allocator is configured")
	}
}

func TestNewBuiltInServiceUsesWorkersRunnerDependencies(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() error = %v", err)
	}
	runner := &recordingWorkersRunner{}
	service, err := NewBuiltInService(
		catalogService,
		BuiltInDependenciesFromWorkersRunner(runner),
	)
	if err != nil || service == nil {
		t.Fatalf("NewBuiltInService() = (%v, %v), want execution service", service, err)
	}
	if got := executionservice.BuiltInRegistrations(BuiltInDependenciesFromWorkersRunner(runner)); len(got) != 4 {
		t.Fatalf("built-in registrations = %d, want 4 antigravity/codex/claude/cursor adapters", len(got))
	}
}
