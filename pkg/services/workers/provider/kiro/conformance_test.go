package kiro_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	kiropkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/kiro"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

func TestKiroIntegrationSatisfiesApplicableSharedContract(t *testing.T) {
	t.Parallel()

	integration := kiropkg.NewIntegration(kiropkg.IntegrationDependencies{
		CommandRunner: &conformanceRunner{},
	})
	if err := inference.ValidateIdentity(integration.Identity()); err != nil {
		t.Fatalf("ValidateIdentity() error = %v", err)
	}
	if integration.Identity() != "kiro" {
		t.Fatalf("Identity() = %q, want kiro", integration.Identity())
	}

	maximum := integration.MaximumCapabilities()
	if err := inference.ValidateMaximumCapabilities(maximum); err != nil {
		t.Fatalf("ValidateMaximumCapabilities() error = %v", err)
	}
	wantMaximum := []inference.Capability{
		inference.CapabilityPromptSubmission,
		inference.CapabilitySessionResume,
		inference.CapabilityMessageSnapshots,
	}
	if got := maximum.Values(); !reflect.DeepEqual(got, wantMaximum) {
		t.Fatalf("MaximumCapabilities() = %v, want %v", got, wantMaximum)
	}

	discovery, err := integration.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if err := inference.ValidateDiscovery(discovery); err != nil {
		t.Fatalf("ValidateDiscovery() error = %v", err)
	}
	if discovery.Readiness() != inference.ReadinessReady {
		t.Fatalf("Discover() readiness = %q, want ready", discovery.Readiness())
	}

	request := kiroConformanceRequest("inv-kiro-capabilities")
	negotiated, err := integration.Capabilities(context.Background(), request)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if err := inference.ValidateNegotiatedCapabilities(maximum, negotiated); err != nil {
		t.Fatalf("ValidateNegotiatedCapabilities() error = %v", err)
	}
	for _, required := range request.RequiredCapabilities().Values() {
		if !negotiated.Has(required) {
			t.Fatalf("Capabilities() omitted required capability %q", required)
		}
	}
}

func TestBuiltInRegistrySelectsKiroByCanonicalIdentityAndAlias(t *testing.T) {
	t.Parallel()

	providers := newKiroRegistry(t, &conformanceRunner{})
	for _, identity := range []string{" KIRO ", "kiro-cli"} {
		entry, err := providers.Lookup(identity)
		if err != nil {
			t.Fatalf("Lookup(%q) error = %v", identity, err)
		}
		if entry.Identity() != "kiro" {
			t.Fatalf("Lookup(%q) identity = %q, want kiro", identity, entry.Identity())
		}
		integration, err := providers.Integration(identity)
		if err != nil {
			t.Fatalf("Integration(%q) error = %v", identity, err)
		}
		if integration.Identity() != "kiro" {
			t.Fatalf("Integration(%q) identity = %q, want kiro", identity, integration.Identity())
		}
	}
	if providers.UsesNativeRunner("kiro") {
		t.Fatal("UsesNativeRunner(kiro) = true, want neutral conductor route")
	}
	if !providers.UsesNativeRunner(workers.RunnerIDCodex) {
		t.Fatal("UsesNativeRunner(codex) = false, want retained native route")
	}
}

func TestConductorInvokesKiroThroughInjectedCommandBoundary(t *testing.T) {
	t.Parallel()

	runner := &conformanceRunner{result: workerprocess.CommandResult{
		Stdout: []byte("kiro conductor answer"),
	}}
	destination := &conformanceDestination{}
	err := conductor.New(newKiroRegistry(t, runner)).Invoke(
		context.Background(),
		"kiro-cli",
		kiroConformanceRequest("inv-kiro-conductor"),
		destination,
	)
	if err != nil {
		t.Fatalf("conductor.Invoke(kiro-cli) error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.calls)
	}
	if runner.request.Command != "kiro-cli" {
		t.Fatalf("command = %q, want kiro-cli", runner.request.Command)
	}
	if !containsConformanceArgPair(runner.request.Args, "--resume-id", kiroSessionID) {
		t.Fatalf("args = %#v, want --resume-id %s", runner.request.Args, kiroSessionID)
	}
	if destination.closes != 1 || destination.completion == nil {
		t.Fatalf("destination closes = %d completion = %#v, want exactly one close", destination.closes, destination.completion)
	}
	response := destination.completion.Response()
	if response == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want successful response", destination.completion)
	}
	if response.Content() != "kiro conductor answer" {
		t.Fatalf("response content = %q, want kiro conductor answer", response.Content())
	}
	if len(destination.events) != 3 {
		t.Fatalf("events = %d, want final-only lifecycle", len(destination.events))
	}
}

func TestRegistrySelectedKiroFailureClosesExactlyOnce(t *testing.T) {
	t.Parallel()

	runner := &conformanceRunner{result: workerprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`{"error":{"type":"authentication_error","message":"token=customer-secret"}}`),
	}}
	destination := &conformanceDestination{}
	integration, err := newKiroRegistry(t, runner).Integration("kiro")
	if err != nil {
		t.Fatalf("Integration(kiro) error = %v", err)
	}
	if err := inference.ExecuteInvocation(
		context.Background(),
		integration,
		kiroConformanceRequest("inv-kiro-failure"),
		destination,
	); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	if destination.closes != 1 || destination.completion == nil {
		t.Fatalf("destination closes = %d completion = %#v, want exactly one close", destination.closes, destination.completion)
	}
	failure := destination.completion.Failure()
	if failure == nil || destination.completion.Response() != nil {
		t.Fatalf("completion = %#v, want normalized failure", destination.completion)
	}
	if failure.Kind() != inference.FailureAuthentication ||
		failure.Message() != "Kiro authentication failed. Sign in again and retry." {
		t.Fatalf(
			"failure = %q/%q, want authentication/Kiro authentication failed. Sign in again and retry.",
			failure.Kind(),
			failure.Message(),
		)
	}
	if len(destination.events) != 0 {
		t.Fatalf("failure events = %#v, want none", destination.events)
	}
}

const kiroSessionID = "675f9238-5f05-456c-9a9f-f8fe486f49e4"

func kiroConformanceRequest(invocationID string) inference.InvocationRequest {
	session := inference.NewProviderSession("kiro", "session_id", kiroSessionID, nil)
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID:    invocationID,
		Model:           "claude-sonnet-4",
		SystemPrompt:    "Follow the factory instructions.",
		UserMessage:     "complete the requested work",
		ProviderSession: &session,
		Required: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
			inference.CapabilitySessionResume,
			inference.CapabilityMessageSnapshots,
		),
	})
}

func newKiroRegistry(t *testing.T, runner workerprocess.CommandRunner) *registry.Registry {
	t.Helper()
	registrations, err := registry.BuiltInRegistrations(registry.BuiltInDependencies{
		CommandRunner: runner,
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

type conformanceRunner struct {
	calls   int
	request workerprocess.CommandRequest
	result  workerprocess.CommandResult
	err     error
}

func (r *conformanceRunner) Run(
	_ context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	r.calls++
	r.request = request
	return r.result, r.err
}

type conformanceDestination struct {
	events     []inference.EventDraft
	completion *inference.Completion
	closes     int
}

func (d *conformanceDestination) WriteEvent(
	_ context.Context,
	event inference.EventDraft,
) error {
	d.events = append(d.events, event)
	return nil
}

func (d *conformanceDestination) Close(
	_ context.Context,
	completion inference.Completion,
) error {
	d.closes++
	cloned := completion
	d.completion = &cloned
	return nil
}

func containsConformanceArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
