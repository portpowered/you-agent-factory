package wire

import (
	"context"
	"errors"
	"reflect"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
)

type recordingWorkersRunner struct {
	calls int
}

func (r *recordingWorkersRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{Stdout: []byte("ok")}, nil
}

type recordingPlatformRunner struct {
	calls   int
	request platformprocess.CommandRequest
	result  platformprocess.CommandResult
	err     error
}

func (r *recordingPlatformRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	r.request = request
	if r.result.Stdout == nil && r.result.Stderr == nil && r.result.ExitCode == 0 {
		r.result.Stdout = []byte("ok")
	}
	return r.result, r.err
}

type recordingStreamingPlatformRunner struct {
	recordingPlatformRunner
	stream    func(platformprocess.OutputChunkObserver)
	streamErr error
}

func (r *recordingStreamingPlatformRunner) RunStreaming(
	_ context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	r.calls++
	r.request = request
	if r.stream != nil {
		r.stream(observer)
	}
	return r.result, r.streamErr
}

func TestBuiltInDependenciesFromWorkersRunnerConstructsCodexAndClaudeEffects(t *testing.T) {
	t.Parallel()

	runner := &recordingWorkersRunner{}
	deps := BuiltInDependenciesFromWorkersRunner(runner)
	if deps.Codex == nil || deps.Claude == nil {
		t.Fatalf("built-in dependencies = %#v, want codex and claude effects", deps)
	}
	if deps.Antigravity != nil {
		t.Fatalf("built-in Antigravity effect = %#v, want nil without PTY platform dependencies", deps.Antigravity)
	}
}

func TestBuiltInDependenciesFromRunnerAdaptsPlatformRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{}
	deps := BuiltInDependenciesFromRunner(runner)
	if deps.Codex == nil || deps.Claude == nil {
		t.Fatalf("built-in dependencies = %#v, want codex and claude effects", deps)
	}
	if deps.Antigravity != nil {
		t.Fatalf("built-in Antigravity effect = %#v, want nil without PTY platform dependencies", deps.Antigravity)
	}
}

func TestBuiltInDependenciesFromCommandRunnerUsesAgyCommandEffect(t *testing.T) {
	t.Parallel()

	runner := &recordingWorkersRunner{}
	deps := BuiltInDependenciesFromCommandRunner(
		providerservice.AdaptCommandRunner(runner),
		BuiltInRunnerPlatformDependencies{
			AgyCommandRunner: providerservice.AdaptCommandRunner(runner),
		},
	)
	if deps.Antigravity == nil {
		t.Fatalf("built-in Agy effect = nil, want command effect when runner is configured")
	}
}

func TestAdaptPlatformCommandRunnerRunMapsRequestAndResult(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{
		result: platformprocess.CommandResult{
			Stdout:   []byte("stdout"),
			Stderr:   []byte("stderr"),
			ExitCode: 7,
		},
	}
	adapted := AdaptPlatformCommandRunner(runner)
	if adapted == nil {
		t.Fatal("AdaptPlatformCommandRunner() = nil, want adapter")
	}
	request := providerservice.CommandRequest{
		Command:          "provider",
		Args:             []string{"--print"},
		Stdin:            []byte("input"),
		Env:              []string{"MODE=test"},
		WorkDir:          "C:\\factory",
		FactorySessionID: "factory-session-1",
	}
	result, err := adapted.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(runner.request, platformprocess.CommandRequest{
		Command:          "provider",
		Args:             []string{"--print"},
		Stdin:            []byte("input"),
		Env:              []string{"MODE=test"},
		WorkDir:          "C:\\factory",
		ExecutionScopeID: "factory-session-1",
	}) {
		t.Fatalf("platform request = %#v, want mapped request", runner.request)
	}
	if !reflect.DeepEqual(result, providerservice.CommandResult{
		Stdout:   []byte("stdout"),
		Stderr:   []byte("stderr"),
		ExitCode: 7,
	}) {
		t.Fatalf("adapted result = %#v, want mapped result", result)
	}
}

func TestAdaptPlatformCommandRunnerStreamingFallsBackAndPublishesOutput(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{result: platformprocess.CommandResult{
		Stdout: []byte("stdout"),
		Stderr: []byte("stderr"),
	}}
	adapted := AdaptPlatformCommandRunner(runner)
	streaming, ok := adapted.(providerservice.StreamingCommandRunner)
	if !ok {
		t.Fatal("AdaptPlatformCommandRunner() does not expose streaming effect")
	}
	var streams []string
	var chunks []string
	result, err := streaming.RunStreaming(context.Background(), providerservice.CommandRequest{}, func(stream string, chunk []byte) error {
		streams = append(streams, stream)
		chunks = append(chunks, string(chunk))
		return nil
	})
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if !reflect.DeepEqual(streams, []string{providerservice.OutputStreamStdout, providerservice.OutputStreamStderr}) || !reflect.DeepEqual(chunks, []string{"stdout", "stderr"}) {
		t.Fatalf("published output = (%v, %v), want stdout/stderr chunks", streams, chunks)
	}
	if !reflect.DeepEqual(result, providerservice.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr")}) {
		t.Fatalf("fallback result = %#v, want completed result", result)
	}
}

func TestAdaptPlatformCommandRunnerStreamingPreservesObserverError(t *testing.T) {
	t.Parallel()

	runner := &recordingStreamingPlatformRunner{
		recordingPlatformRunner: recordingPlatformRunner{result: platformprocess.CommandResult{ExitCode: 3}},
		stream: func(observer platformprocess.OutputChunkObserver) {
			observer("stdout", []byte("first"))
			observer("stderr", []byte("second"))
		},
	}
	adapted := AdaptPlatformCommandRunner(runner)
	streaming, ok := adapted.(providerservice.StreamingCommandRunner)
	if !ok {
		t.Fatal("AdaptPlatformCommandRunner() does not expose streaming effect")
	}
	wantErr := errors.New("observer stopped")
	var calls int
	result, err := streaming.RunStreaming(context.Background(), providerservice.CommandRequest{}, func(_ string, _ []byte) error {
		calls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunStreaming() error = %v, want observer error", err)
	}
	if calls != 1 {
		t.Fatalf("observer calls = %d, want one after observer failure", calls)
	}
	if result.ExitCode != 3 {
		t.Fatalf("streaming result = %#v, want exit code preserved", result)
	}
}

func TestAdaptPlatformCommandRunnerNilAndEmptyOutput(t *testing.T) {
	t.Parallel()

	if AdaptPlatformCommandRunner(nil) != nil {
		t.Fatal("AdaptPlatformCommandRunner(nil) returned an adapter")
	}
	if err := publishCompleteOutput(nil, nil, nil); err != nil {
		t.Fatalf("publishCompleteOutput(nil) error = %v", err)
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

func TestNewAgyPTYAllocatorRequiresExplicitPlatformEffects(t *testing.T) {
	t.Parallel()

	allocator, err := NewAgyPTYAllocator(nil, nil)
	if !errors.Is(err, agypty.ErrHostRequired) {
		t.Fatalf("NewAgyPTYAllocator(nil, nil) = (%v, %v), want host validation error", allocator, err)
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
	if got := executionservice.BuiltInRegistrations(BuiltInDependenciesFromWorkersRunner(runner)); len(got) != 3 {
		t.Fatalf("built-in registrations = %d, want 3 antigravity/codex/claude adapters", len(got))
	}
	if got := executionservice.BuiltInRegistrations(); len(got) != 3 {
		t.Fatalf("default built-in registrations = %d, want 3 unavailable adapter bindings", len(got))
	}
	if got := BuiltInRegistrations(); len(got) != 3 {
		t.Fatalf("wire built-in registrations = %d, want 3 unavailable adapter bindings", len(got))
	}
}

func TestNewACPRegistrationRoutesOnlyPrivateContinuationInput(t *testing.T) {
	t.Parallel()

	fake := &continuationACPServiceFake{}
	registration := NewACPRegistration("cursor", fake)
	request := providers.ExecuteRequest{Provider: "cursor", AttemptID: "attempt-1"}
	if _, err := registration.Attempt(context.Background(), request); err != nil {
		t.Fatalf("Attempt() error = %v", err)
	}
	if !reflect.DeepEqual(fake.executeRequest, request) {
		t.Fatalf("ACP Execute request = %#v, want %#v", fake.executeRequest, request)
	}

	reference := providers.SessionRef{Provider: "cursor", Kind: providers.SessionIDKind, ID: "session-1"}
	if _, err := registration.Continue(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: request,
		ResumeSession:  &reference,
	}); err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if fake.continueReference != reference || !reflect.DeepEqual(fake.continueRequest, request) {
		t.Fatalf("ACP Continue = (%#v, %#v), want (%#v, %#v)", fake.continueRequest, fake.continueReference, request, reference)
	}
	_, err := registration.Continue(context.Background(), execution.ContinuationRequest{ExecuteRequest: request})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("Continue(missing private reference) error = %#v, want invalid request", err)
	}
}

type continuationACPServiceFake struct {
	acp.Service
	executeRequest    providers.ExecuteRequest
	continueRequest   providers.ExecuteRequest
	continueReference providers.SessionRef
}

func (fake *continuationACPServiceFake) Execute(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeRequest = request
	return providers.ExecuteResult{}, nil
}

func (fake *continuationACPServiceFake) Continue(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
	reference providers.SessionRef,
) (providers.ExecuteResult, error) {
	fake.continueRequest = request
	fake.continueReference = reference
	return providers.ExecuteResult{}, nil
}

var _ acp.ContinuationService = (*continuationACPServiceFake)(nil)
