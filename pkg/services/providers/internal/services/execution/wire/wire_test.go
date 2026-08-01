package wire

import (
	"context"
	"errors"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/effects"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
)

type recordingPlatformRunner struct {
	calls int
}

type streamingPlatformRunner struct{}

func (r *recordingPlatformRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{Stdout: []byte("ok")}, nil
}

func (streamingPlatformRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

func (streamingPlatformRunner) RunStreaming(
	_ context.Context,
	_ platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte("streamed"))
	}
	return platformprocess.CommandResult{Stdout: []byte("streamed")}, nil
}

func TestAdaptPlatformCommandRunnerPropagatesFallbackObserverFailure(t *testing.T) {
	t.Parallel()

	observerErr := errors.New("fallback output observer failed")
	runner, ok := AdaptPlatformCommandRunner(&recordingPlatformRunner{}).(interface {
		RunStreaming(context.Context, effects.CommandRequest, effects.OutputChunkObserver) (effects.CommandResult, error)
	})
	if !ok {
		t.Fatal("adapted runner does not expose Providers streaming effect")
	}
	_, err := runner.RunStreaming(t.Context(), effects.CommandRequest{}, func(string, []byte) error {
		return observerErr
	})
	if !errors.Is(err, observerErr) {
		t.Fatalf("RunStreaming() error = %v, want observer failure", err)
	}
}

func TestAdaptPlatformCommandRunnerPropagatesStreamingObserverFailure(t *testing.T) {
	t.Parallel()

	observerErr := errors.New("streaming output observer failed")
	runner, ok := AdaptPlatformCommandRunner(streamingPlatformRunner{}).(interface {
		RunStreaming(context.Context, effects.CommandRequest, effects.OutputChunkObserver) (effects.CommandResult, error)
	})
	if !ok {
		t.Fatal("adapted runner does not expose Providers streaming effect")
	}
	_, err := runner.RunStreaming(t.Context(), effects.CommandRequest{}, func(string, []byte) error {
		return observerErr
	})
	if !errors.Is(err, observerErr) {
		t.Fatalf("RunStreaming() error = %v, want observer failure", err)
	}
}

func TestBuiltInDependenciesFromProvidersRunnerConstructsCodexAndClaudeEffects(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{}
	deps := BuiltInDependenciesFromRunner(AdaptPlatformCommandRunner(runner))
	if deps.Codex == nil || deps.Claude == nil {
		t.Fatalf("built-in dependencies = %#v, want codex and claude effects", deps)
	}
	if deps.Antigravity != nil {
		t.Fatalf("built-in Antigravity effect = %#v, want nil without PTY platform dependencies", deps.Antigravity)
	}
}

func TestBuiltInDependenciesFromProvidersRunnerConstructsAgyPTYEffectWithAllocator(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{}
	deps := BuiltInDependenciesFromRunner(AdaptPlatformCommandRunner(runner), BuiltInRunnerPlatformDependencies{
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

func TestNewBuiltInServiceUsesProvidersRunnerDependencies(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() error = %v", err)
	}
	runner := &recordingPlatformRunner{}
	service, err := NewBuiltInService(
		catalogService,
		BuiltInDependenciesFromRunner(AdaptPlatformCommandRunner(runner)),
	)
	if err != nil || service == nil {
		t.Fatalf("NewBuiltInService() = (%v, %v), want execution service", service, err)
	}
	if got := executionservice.BuiltInRegistrations(BuiltInDependenciesFromRunner(AdaptPlatformCommandRunner(runner))); len(got) != 3 {
		t.Fatalf("built-in registrations = %d, want 3 antigravity/codex/claude adapters", len(got))
	}
}
