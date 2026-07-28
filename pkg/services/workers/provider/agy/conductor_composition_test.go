package agy_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

func TestBuiltInRegistrySelectsAgyThroughAuthoritativeManifestIdentity(t *testing.T) {
	t.Parallel()

	providers := newProductionAgyRegistry(t)
	entry, err := providers.Lookup(" AGY ")
	if err != nil {
		t.Fatalf("Lookup(agy) error = %v", err)
	}
	if entry.Identity() != inference.Identity(modelprovider.ProviderAgy) {
		t.Fatalf("Lookup identity = %q, want agy", entry.Identity())
	}
	integration, err := providers.Integration(string(modelprovider.ProviderAgy))
	if err != nil {
		t.Fatalf("Integration(agy) error = %v", err)
	}
	if integration.Identity() != inference.Identity(modelprovider.ProviderAgy) {
		t.Fatalf("Integration identity = %q, want agy", integration.Identity())
	}
	maximum := integration.MaximumCapabilities()
	if !maximum.Has(inference.CapabilityPromptSubmission) || !maximum.Has(inference.CapabilityMessageSnapshots) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt_submission and message_snapshots", maximum.Values())
	}
	if providers.UsesNativeRunner(string(modelprovider.ProviderAgy)) {
		t.Fatal("UsesNativeRunner(agy) = true, want conductor route for migrated Agy")
	}
}

func TestConductorInvokesAgyWithoutConcreteProviderSwitch(t *testing.T) {
	t.Parallel()

	mock := &agypty.MockAllocator{
		Result: agypty.SessionResult{ExitCode: 0, CleanedText: "agy conductor answer"},
	}
	providers := newAgyRegistryWithPTY(t, mock)
	destination := &recordingDestination{}

	err := conductor.New(providers).Invoke(
		context.Background(),
		string(modelprovider.ProviderAgy),
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-agy-conductor",
			Model:        "agy-default",
			UserMessage:  "say hello",
			Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
			Execution: workers.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "inv-agy-conductor",
				},
				WorkingDirectory: t.TempDir(),
			},
		}),
		destination,
	)
	if err != nil {
		t.Fatalf("conductor.Invoke(agy) error = %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want successful response", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "agy conductor answer" {
		t.Fatalf("response content = %q, want agy conductor answer", got)
	}
	if len(mock.Sessions) != 1 {
		t.Fatalf("pty sessions = %d, want 1", len(mock.Sessions))
	}
}

func TestConductorRejectsAgyCapabilityEscalationBeforeProviderIO(t *testing.T) {
	t.Parallel()

	mock := &agypty.MockAllocator{
		Result: agypty.SessionResult{ExitCode: 0, CleanedText: "should-not-run"},
	}
	providers := newAgyRegistryWithPTY(t, mock)
	destination := &recordingDestination{}

	err := conductor.New(providers).Invoke(
		context.Background(),
		string(modelprovider.ProviderAgy),
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-agy-escalate",
			UserMessage:  "hello",
			Required: inference.NewCapabilitySet(
				inference.CapabilityPromptSubmission,
				inference.CapabilityNativeStreaming,
			),
		}),
		destination,
	)
	if err == nil {
		t.Fatal("conductor.Invoke(escalation) error = nil, want rejection")
	}
	var rejection *conductor.Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("error = %v, want *conductor.Rejection", err)
	}
	if rejection.Invariant() != conductor.InvariantCapabilityEscalation {
		t.Fatalf("Invariant() = %q, want %q", rejection.Invariant(), conductor.InvariantCapabilityEscalation)
	}
	if rejection.Capability() != string(inference.CapabilityNativeStreaming) {
		t.Fatalf("Capability() = %q, want native_streaming", rejection.Capability())
	}
	if len(mock.Sessions) != 0 {
		t.Fatalf("provider I/O occurred: pty sessions = %d", len(mock.Sessions))
	}
	if destination.completion != nil {
		t.Fatalf("destination received completion %#v despite preflight rejection", destination.completion)
	}
}

func TestConductorClassifiesAgyNativeFailureSafely(t *testing.T) {
	t.Parallel()

	mock := &agypty.MockAllocator{
		Result: agypty.SessionResult{
			ExitCode: 1,
			RawBytes: []byte("failed reading /tmp/secret-key and private prompt"),
		},
	}
	providers := newAgyRegistryWithPTY(t, mock)
	destination := &recordingDestination{}

	err := conductor.New(providers).Invoke(
		context.Background(),
		string(modelprovider.ProviderAgy),
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-agy-failure",
			UserMessage:  "private prompt",
			Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		}),
		destination,
	)
	if err != nil {
		t.Fatalf("conductor.Invoke(failure) error = %v", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil {
		t.Fatalf("completion = %#v, want normalized failure", destination.completion)
	}
	failure := destination.completion.Failure()
	if strings.Contains(failure.Message(), "/tmp/") ||
		strings.Contains(failure.Message(), "secret-key") ||
		strings.Contains(failure.Message(), "private prompt") {
		t.Fatalf("failure message leaked unsafe detail: %q", failure.Message())
	}
}

func newProductionAgyRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	registrations, err := registry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := registry.New(registrations...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}

func newAgyRegistryWithPTY(t *testing.T, allocator *agypty.MockAllocator) *registry.Registry {
	t.Helper()
	providersService, err := providerswire.NewService(
		providerswire.WithCommandRunner(testutil.NewProviderCommandRunner()),
		providerswire.WithAgyPTY(providerswire.AgyPTYPlatformDependencies{
			Allocator: allocator,
			Locator:   platformprocess.HostExecutableLocator{},
			Inspector: platformfilesystem.Local{},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	registrations, err := registry.BuiltInRegistrations(registry.BuiltInDependencies{
		ProvidersService: providersService,
	})
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := registry.New(registrations...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}

type recordingDestination struct {
	events     []inference.EventDraft
	completion *inference.Completion
}

func (d *recordingDestination) WriteEvent(_ context.Context, event inference.EventDraft) error {
	d.events = append(d.events, event)
	return nil
}

func (d *recordingDestination) Close(_ context.Context, completion inference.Completion) error {
	clone := completion
	d.completion = &clone
	return nil
}
