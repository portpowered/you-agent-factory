package wire

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
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

func TestBuiltInDependenciesFromWorkersRunnerConstructsCodexAndClaudeEffects(t *testing.T) {
	t.Parallel()

	runner := &recordingWorkersRunner{}
	deps := BuiltInDependenciesFromWorkersRunner(runner)
	if deps.Codex == nil || deps.Claude == nil {
		t.Fatalf("built-in dependencies = %#v, want codex and claude effects", deps)
	}
}

func TestBuiltInDependenciesFromRunnerAdaptsPlatformRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{}
	deps := BuiltInDependenciesFromRunner(runner)
	if deps.Codex == nil || deps.Claude == nil {
		t.Fatalf("built-in dependencies = %#v, want codex and claude effects", deps)
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
	if got := executionservice.BuiltInRegistrations(BuiltInDependenciesFromWorkersRunner(runner)); len(got) != 2 {
		t.Fatalf("built-in registrations = %d, want 2 codex/claude adapters", len(got))
	}
}
