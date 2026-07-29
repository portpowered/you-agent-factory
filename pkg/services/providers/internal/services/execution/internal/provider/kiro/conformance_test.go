package kiro_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	kiropkg "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/kiro"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/registry"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	kiroSessionID      = "675f9238-5f05-456c-9a9f-f8fe486f49e4"
	kiroEmittedSession = "f2946a26-3735-4b08-8d05-c928010302d5"
)

func TestKiroIntegrationSatisfiesSharedContractSurface(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("unused")})
	integration := kiroIntegrationWithRunner(t, runner)
	assertKiroContractIdentity(t, integration)
	maximum := assertKiroContractMaximumCapabilities(t, integration)
	assertKiroContractDiscovery(t, integration)
	assertKiroContractNegotiatedCapabilities(t, integration, maximum)
}

func assertKiroContractIdentity(t *testing.T, integration inference.Integration) {
	t.Helper()
	if err := inference.ValidateIdentity(integration.Identity()); err != nil {
		t.Fatalf("ValidateIdentity() error = %v", err)
	}
	if integration.Identity() != "kiro" {
		t.Fatalf("Identity() = %q, want kiro", integration.Identity())
	}
}

func assertKiroContractMaximumCapabilities(t *testing.T, integration inference.Integration) inference.CapabilitySet {
	t.Helper()
	maximum := integration.MaximumCapabilities()
	if err := inference.ValidateMaximumCapabilities(maximum); err != nil {
		t.Fatalf("ValidateMaximumCapabilities() error = %v", err)
	}
	for _, required := range []inference.Capability{
		inference.CapabilityPromptSubmission,
		inference.CapabilitySessionResume,
		inference.CapabilityMessageSnapshots,
	} {
		if !maximum.Has(required) {
			t.Fatalf("MaximumCapabilities() = %v, want %q", maximum.Values(), required)
		}
	}
	return maximum
}

func assertKiroContractDiscovery(t *testing.T, integration inference.Integration) {
	t.Helper()
	discovery, err := integration.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if err := inference.ValidateDiscovery(discovery); err != nil {
		t.Fatalf("ValidateDiscovery() error = %v", err)
	}
	if discovery.Readiness() != inference.ReadinessReady {
		t.Fatalf("Discover() readiness = %q, want %q", discovery.Readiness(), inference.ReadinessReady)
	}
}

func assertKiroContractNegotiatedCapabilities(
	t *testing.T,
	integration inference.Integration,
	maximum inference.CapabilitySet,
) {
	t.Helper()
	request := kiroConformanceRequest("inv-kiro-contract")
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

	providers := newKiroRegistry(t, testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("unused"),
	}))
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
}

func TestKiroSuccessConformanceThroughRegistryAndExecuteInvocation(t *testing.T) {
	t.Parallel()

	const content = "kiro conformance answer"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte(content)})
	integration := registryKiroIntegration(t, runner)
	destination := &conformanceDestination{}

	request := kiroConformanceRequest("inv-kiro-success")
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	if destination.closes != 1 || destination.completion == nil {
		t.Fatalf("destination closes = %d completion = %#v, want exactly one close", destination.closes, destination.completion)
	}
	response := destination.completion.Response()
	if response == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want successful response", destination.completion)
	}
	if response.Content() != content {
		t.Fatalf("response content = %q, want %q", response.Content(), content)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.CallCount())
	}
	if runner.LastRequest().Command != "kiro-cli" {
		t.Fatalf("command = %q, want kiro-cli", runner.LastRequest().Command)
	}
	if !containsConformanceArgPair(runner.LastRequest().Args, "--resume-id", kiroSessionID) {
		t.Fatalf("args = %#v, want --resume-id %s", runner.LastRequest().Args, kiroSessionID)
	}
	assertConformanceSuccessEvents(t, destination.events, request.InvocationID(), content)
}

func TestKiroSuccessPreservesEmittedSessionRef(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("continued answer"),
		Stderr: []byte(`{"session_id":"` + kiroEmittedSession + `"}`),
	})
	destination := &conformanceDestination{}
	request := kiroConformanceRequest("inv-kiro-session")
	if err := inference.ExecuteInvocation(
		context.Background(),
		registryKiroIntegration(t, runner),
		request,
		destination,
	); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	session := destination.completion.Response().ProviderSession()
	if session == nil || session.ID() != kiroEmittedSession {
		t.Fatalf("ProviderSession() = %#v, want emitted session %s", session, kiroEmittedSession)
	}
}

type kiroFailureConformanceCase struct {
	name        string
	result      platformprocess.CommandResult
	wantKind    inference.FailureKind
	wantMessage string
	wantRetry   bool
	wantSession string
	reject      []string
}

func kiroFailureConformanceCases() []kiroFailureConformanceCase {
	return []kiroFailureConformanceCase{
		{
			name: "Authentication",
			result: platformprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"error":{"type":"authentication_error","message":"Bearer private-token"}}`),
			},
			wantKind:    inference.FailureAuthentication,
			wantMessage: "Kiro authentication failed. Sign in again and retry.",
			reject:      []string{"Bearer", "private-token"},
		},
		{
			name: "InvalidRequest",
			result: platformprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"status":422,"message":"private customer request"}`),
			},
			wantKind:    inference.FailureInvalidRequest,
			wantMessage: "Kiro rejected the request as invalid.",
			reject:      []string{"private customer request"},
		},
		{
			name: "Throttled",
			result: platformprocess.CommandResult{
				ExitCode: 1,
				Stdout:   []byte(`{"error":{"code":"ThrottlingException","message":"capacity secret"}}`),
				Stderr:   []byte("ERROR: authentication required"),
			},
			wantKind:    inference.FailureThrottled,
			wantMessage: "Kiro is temporarily unavailable due to usage or capacity limits.",
			wantRetry:   true,
			reject:      []string{"authentication required", "capacity secret"},
		},
		{
			name: "ServiceUnavailable",
			result: platformprocess.CommandResult{
				ExitCode: 1,
				Stderr: []byte(
					`{"event":"session.created","session_id":"` + kiroSessionID + `"}` + "\n" +
						`{"type":"error","errorType":"ServiceUnavailableException","message":"host /tmp/private"}`,
				),
			},
			wantKind:    inference.FailureDependency,
			wantMessage: "Kiro encountered a temporary service error.",
			wantSession: kiroSessionID,
			reject:      []string{"/tmp/private"},
		},
		{
			name: "UnknownSafeFallback",
			result: platformprocess.CommandResult{
				ExitCode: 17,
				Stderr:   []byte(`Error: failed reading C:\Users\alice\private\project`),
			},
			wantKind:    inference.FailureUnknown,
			wantMessage: "kiro-cli exited with code 17",
			reject:      []string{`C:\Users\alice`, "private"},
		},
	}
}

func assertKiroFailureConformance(t *testing.T, tc kiroFailureConformanceCase) {
	t.Helper()
	runner := testutil.NewProviderCommandRunner(tc.result)
	integration := kiroIntegrationWithRunner(t, runner)
	destination := &conformanceDestination{}
	request := kiroFailureConformanceRequest("inv-kiro-failure-" + strings.ToLower(tc.name))

	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	if destination.closes != 1 || destination.completion == nil {
		t.Fatalf("destination closes = %d completion = %#v, want exactly one close", destination.closes, destination.completion)
	}
	failure := destination.completion.Failure()
	if failure == nil || destination.completion.Response() != nil {
		t.Fatalf("completion = %#v, want normalized failure only", destination.completion)
	}
	if failure.Kind() != tc.wantKind {
		t.Fatalf("failure kind = %q, want %q", failure.Kind(), tc.wantKind)
	}
	if failure.Message() != tc.wantMessage {
		t.Fatalf("failure message = %q, want %q", failure.Message(), tc.wantMessage)
	}
	if failure.Retryable() != tc.wantRetry {
		t.Fatalf("failure retryable = %v, want %v", failure.Retryable(), tc.wantRetry)
	}
	assertKiroFailureSession(t, failure.ProviderSession(), tc.wantSession)
	for _, rejected := range tc.reject {
		if strings.Contains(failure.Message(), rejected) {
			t.Fatalf("failure message leaked %q: %q", rejected, failure.Message())
		}
	}
	if len(destination.events) != 1 {
		t.Fatalf("failure path events = %#v, want one synthesized error event", destination.events)
	}
	draft := destination.events[0].Draft()
	if draft.Kind != workers.KindError || draft.Phase != workers.PhaseFailed {
		t.Fatalf("failure event = %#v, want synthesized error.failed", draft)
	}
}

func TestKiroFailureConformanceThroughExecuteInvocation(t *testing.T) {
	t.Parallel()

	for _, tc := range kiroFailureConformanceCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertKiroFailureConformance(t, tc)
		})
	}
}

func kiroFailureConformanceRequest(invocationID string) inference.InvocationRequest {
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: invocationID,
		Model:        "claude-sonnet-4",
		SystemPrompt: "Follow the factory instructions.",
		UserMessage:  "complete the requested work",
		Required: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
			inference.CapabilitySessionResume,
			inference.CapabilityMessageSnapshots,
		),
	})
}

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

func registryKiroIntegration(t *testing.T, runner platformprocess.CommandRunner) inference.Integration {
	t.Helper()
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
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
	integration, err := providers.Integration("kiro")
	if err != nil {
		t.Fatalf("Integration(kiro) error = %v", err)
	}
	return integration
}

func kiroIntegrationWithRunner(t *testing.T, runner platformprocess.CommandRunner) inference.Integration {
	t.Helper()
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return kiropkg.NewIntegration(kiropkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
}

func newKiroRegistry(t *testing.T, runner platformprocess.CommandRunner) *registry.Registry {
	t.Helper()
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
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

func assertConformanceSuccessEvents(t *testing.T, events []inference.EventDraft, invocationID, content string) {
	t.Helper()
	if len(events) != 3 {
		t.Fatalf("events = %#v, want run started, message completed, run completed", drafts(events))
	}
	assertEventPhase(t, events[0], workers.KindRun, workers.PhaseStarted)
	assertEventPhase(t, events[1], workers.KindMessage, workers.PhaseCompleted)
	assertEventPhase(t, events[2], workers.KindRun, workers.PhaseCompleted)
	for index, event := range events {
		draft := event.Draft()
		if draft.RunID != invocationID {
			t.Fatalf("event[%d] run ID = %q, want %q", index, draft.RunID, invocationID)
		}
		if draft.Provenance.Provider != "kiro" {
			t.Fatalf("event[%d] provider = %q, want kiro", index, draft.Provenance.Provider)
		}
	}
	message := events[1].Draft()
	if message.ItemID != "kiro-final" {
		t.Fatalf("message item ID = %q, want kiro-final", message.ItemID)
	}
	if !strings.Contains(string(message.Payload), content) {
		t.Fatalf("message payload = %s, want content %q", message.Payload, content)
	}
}

func assertEventPhase(t *testing.T, event inference.EventDraft, kind workers.Kind, phase workers.Phase) {
	t.Helper()
	draft := event.Draft()
	if draft.Kind != kind || draft.Phase != phase {
		t.Fatalf("event = %s/%s, want %s/%s", draft.Kind, draft.Phase, kind, phase)
	}
}

func assertKiroFailureSession(t *testing.T, session *inference.ProviderSession, wantSession string) {
	t.Helper()
	if wantSession == "" {
		if session != nil {
			t.Fatalf("failure Provider Session = %#v, want nil", session)
		}
		return
	}
	if session == nil || session.Provider() != "kiro" || session.ID() != wantSession {
		t.Fatalf(
			"failure Provider Session = %#v, want provider=kiro id=%q",
			session,
			wantSession,
		)
	}
}

func drafts(events []inference.EventDraft) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		draft := event.Draft()
		out = append(out, string(draft.Kind)+":"+string(draft.Phase))
	}
	return out
}

type conformanceDestination struct {
	events     []inference.EventDraft
	completion *inference.Completion
	closes     int
}

func (d *conformanceDestination) WriteEvent(_ context.Context, event inference.EventDraft) error {
	d.events = append(d.events, event)
	return nil
}

func (d *conformanceDestination) Close(_ context.Context, completion inference.Completion) error {
	d.closes++
	clone := completion
	d.completion = &clone
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
