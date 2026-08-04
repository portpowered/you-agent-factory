package wire

import (
	"context"
	"errors"
	"reflect"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
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
