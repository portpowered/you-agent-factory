package gemini_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	geminipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

// Gemini advertises prompt_submission + message_snapshots only. Shared
// testkit.Run also requires streaming/tool factories; this suite proves the
// public Integration contract through ExecuteInvocation for the postures that
// apply to Gemini's support/capability set.
func TestGeminiIntegrationSatisfiesSharedContractSurface(t *testing.T) {
	t.Parallel()

	integration := geminipkg.NewIntegration(geminipkg.IntegrationDependencies{
		CommandRunner: &conformanceCommandRunner{result: workerprocess.CommandResult{Stdout: []byte("unused")}},
	})
	if err := inference.ValidateIdentity(integration.Identity()); err != nil {
		t.Fatalf("ValidateIdentity() error = %v", err)
	}
	if integration.Identity() != inference.Identity("gemini") {
		t.Fatalf("Identity() = %q, want gemini", integration.Identity())
	}
	maximum := integration.MaximumCapabilities()
	if err := inference.ValidateMaximumCapabilities(maximum); err != nil {
		t.Fatalf("ValidateMaximumCapabilities() error = %v", err)
	}
	if !maximum.Has(inference.CapabilityPromptSubmission) || !maximum.Has(inference.CapabilityMessageSnapshots) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt_submission and message_snapshots", maximum.Values())
	}
	if maximum.Has(inference.CapabilityNativeStreaming) ||
		maximum.Has(inference.CapabilityMessageDeltas) ||
		maximum.Has(inference.CapabilityToolLifecycle) {
		t.Fatalf("MaximumCapabilities() = %v, must not advertise streaming or tool lifecycle", maximum.Values())
	}

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

	request := conformanceRequest(
		"inv-gemini-contract",
		inference.CapabilityPromptSubmission,
		inference.CapabilityMessageSnapshots,
	)
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

func TestGeminiSuccessConformanceThroughRegistryAndExecuteInvocation(t *testing.T) {
	t.Parallel()

	const content = "gemini conformance answer"
	runner := &conformanceCommandRunner{result: workerprocess.CommandResult{Stdout: []byte(content)}}
	integration := registryGeminiIntegration(t, runner)
	destination := &conformanceDestination{}

	request := conformanceRequest(
		"inv-gemini-success",
		inference.CapabilityPromptSubmission,
		inference.CapabilityMessageSnapshots,
	)
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
	if runner.calls != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.calls)
	}
	assertConformanceSuccessEvents(t, destination.events, request.InvocationID(), content)
}

func TestGeminiFailureConformanceThroughExecuteInvocation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		result      workerprocess.CommandResult
		err         error
		wantKind    inference.FailureKind
		wantMessage string
		reject      []string
	}{
		{
			name: "Authentication",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED","message":"token=customer-secret"}}`),
			},
			wantKind:    inference.FailureAuthentication,
			wantMessage: "Gemini authentication failed.",
			reject:      []string{"token=", "customer-secret"},
		},
		{
			name: "InvalidRequest",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"error":{"code":400,"message":"bad prompt /tmp/secret-path"}}`),
			},
			wantKind:    inference.FailureInvalidRequest,
			wantMessage: "Gemini rejected the request.",
			reject:      []string{"/tmp/", "secret-path"},
		},
		{
			name: "Throttled",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota path /private/key leaked"}}`),
			},
			wantKind:    inference.FailureThrottled,
			wantMessage: "The provider is rate limited; retry after capacity becomes available.",
			reject:      []string{"/private/", "leaked"},
		},
		{
			name: "StructuredTimeout",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"error":{"status":"DEADLINE_EXCEEDED","message":"User prompt: private"}}`),
			},
			wantKind:    inference.FailureTimeout,
			wantMessage: geminipkg.TimeoutFailureMessage,
			reject:      []string{"User prompt", "private"},
		},
		{
			name:        "CommandDeadlineTimeout",
			err:         context.DeadlineExceeded,
			result:      workerprocess.CommandResult{ExitCode: 1, Stderr: []byte("token=customer-secret-value")},
			wantKind:    inference.FailureTimeout,
			wantMessage: geminipkg.TimeoutFailureMessage,
			reject:      []string{"token=", "customer-secret"},
		},
		{
			name: "UnknownSafeFallback",
			result: workerprocess.CommandResult{
				ExitCode: 17,
				Stderr:   []byte("Error report written to .gemini/tmp/private-report.json"),
			},
			wantKind:    inference.FailureUnknown,
			wantMessage: "gemini exited with code 17",
			reject:      []string{".gemini/tmp/", "private-report"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &conformanceCommandRunner{result: tc.result, err: tc.err}
			integration := geminipkg.NewIntegration(geminipkg.IntegrationDependencies{CommandRunner: runner})
			destination := &conformanceDestination{}
			request := conformanceRequest("inv-gemini-failure-"+strings.ToLower(tc.name), inference.CapabilityPromptSubmission)

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
			for _, rejected := range tc.reject {
				if strings.Contains(failure.Message(), rejected) {
					t.Fatalf("failure message leaked %q: %q", rejected, failure.Message())
				}
			}
			if len(destination.events) != 0 {
				t.Fatalf("failure path emitted progress events: %#v", destination.events)
			}
		})
	}
}

func registryGeminiIntegration(t *testing.T, runner workerprocess.CommandRunner) inference.Integration {
	t.Helper()
	registrations, err := registry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	replaced, err := registry.ReplaceCatalogIntegration(
		registrations,
		"gemini",
		geminipkg.NewIntegration(geminipkg.IntegrationDependencies{CommandRunner: runner}),
	)
	if err != nil {
		t.Fatalf("ReplaceCatalogIntegration() error = %v", err)
	}
	providers, err := registry.New(replaced...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	integration, err := providers.Integration("gemini")
	if err != nil {
		t.Fatalf("Integration(gemini) error = %v", err)
	}
	return integration
}

func conformanceRequest(invocationID string, required ...inference.Capability) inference.InvocationRequest {
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: invocationID,
		Model:        "gemini-2.5-pro",
		UserMessage:  "deterministic gemini conformance prompt",
		Required:     inference.NewCapabilitySet(required...),
	})
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
		if draft.Provenance.Provider != "gemini" {
			t.Fatalf("event[%d] provider = %q, want gemini", index, draft.Provenance.Provider)
		}
	}
	message := events[1].Draft()
	if message.ItemID != "gemini-final" {
		t.Fatalf("message item ID = %q, want gemini-final", message.ItemID)
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

func drafts(events []inference.EventDraft) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		draft := event.Draft()
		out = append(out, string(draft.Kind)+":"+string(draft.Phase))
	}
	return out
}

type conformanceCommandRunner struct {
	calls  int
	result workerprocess.CommandResult
	err    error
}

func (r *conformanceCommandRunner) Run(_ context.Context, _ workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.calls++
	return r.result, r.err
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
