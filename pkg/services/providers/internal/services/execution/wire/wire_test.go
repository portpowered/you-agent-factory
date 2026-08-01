package wire

import (
	"context"
	"errors"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
)

type recordingPlatformRunner struct {
	calls int
}

type streamingPlatformRunner struct{}

type failingPlatformRunner struct {
	err error
}

type failingStreamingPlatformRunner struct {
	err error
}

type sequenceClock struct {
	times []time.Time
	index int
}

func (clock *sequenceClock) Now() time.Time {
	if clock.index >= len(clock.times) {
		return clock.times[len(clock.times)-1]
	}
	value := clock.times[clock.index]
	clock.index++
	return value
}

func (r *recordingPlatformRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{Stdout: []byte("ok")}, nil
}

func (r failingPlatformRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("fallback output")}, r.err
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

func (r failingStreamingPlatformRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, r.err
}

func (r failingStreamingPlatformRunner) RunStreaming(
	_ context.Context,
	_ platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte("streamed"))
	}
	return platformprocess.CommandResult{Stdout: []byte("streamed")}, r.err
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

func TestAdaptPlatformCommandRunnerPreservesFallbackObserverAndRunnerFailures(t *testing.T) {
	t.Parallel()

	commandErr := errors.New("fallback command failed")
	observerErr := errors.New("fallback output observer failed")
	runner, ok := AdaptPlatformCommandRunner(failingPlatformRunner{err: commandErr}).(interface {
		RunStreaming(context.Context, effects.CommandRequest, effects.OutputChunkObserver) (effects.CommandResult, error)
	})
	if !ok {
		t.Fatal("adapted runner does not expose Providers streaming effect")
	}
	_, err := runner.RunStreaming(t.Context(), effects.CommandRequest{}, func(string, []byte) error {
		return observerErr
	})
	if !errors.Is(err, commandErr) || !errors.Is(err, observerErr) {
		t.Fatalf("RunStreaming() error = %v, want command and observer failures", err)
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

func TestAdaptPlatformCommandRunnerPreservesStreamingObserverAndRunnerFailures(t *testing.T) {
	t.Parallel()

	commandErr := errors.New("streaming command failed")
	observerErr := errors.New("streaming output observer failed")
	runner, ok := AdaptPlatformCommandRunner(failingStreamingPlatformRunner{err: commandErr}).(interface {
		RunStreaming(context.Context, effects.CommandRequest, effects.OutputChunkObserver) (effects.CommandResult, error)
	})
	if !ok {
		t.Fatal("adapted runner does not expose Providers streaming effect")
	}
	_, err := runner.RunStreaming(t.Context(), effects.CommandRequest{}, func(string, []byte) error {
		return observerErr
	})
	if !errors.Is(err, commandErr) || !errors.Is(err, observerErr) {
		t.Fatalf("RunStreaming() error = %v, want command and observer failures", err)
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

func TestBuiltInDependenciesFromProvidersRunnerInjectsClockIntoCommandEffects(t *testing.T) {
	t.Parallel()

	clock := &sequenceClock{times: []time.Time{
		time.Unix(100, 0),
		time.Unix(137, 0),
	}}
	deps := BuiltInDependenciesFromRunner(
		AdaptPlatformCommandRunner(&recordingPlatformRunner{}),
		BuiltInRunnerPlatformDependencies{Clock: clock},
	)
	result, err := deps.Codex.Execute(
		context.Background(),
		providers.ExecuteRequest{
			Provider:    providers.IDCodex,
			AttemptID:   "clock-injection",
			UserMessage: "perform work",
		},
		func([]byte) error { return nil },
	)
	if err != nil {
		t.Fatalf("Codex effect execution = %v, want success", err)
	}
	if result.DurationMillis != 37*1000 {
		t.Fatalf("Codex effect duration = %d, want %d", result.DurationMillis, 37*1000)
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
