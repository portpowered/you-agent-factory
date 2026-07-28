package gemini_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	geminipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

func TestBuiltInRegistrySelectsGeminiThroughAuthoritativeManifestIdentity(t *testing.T) {
	t.Parallel()

	providers := newProductionGeminiRegistry(t)
	entry, err := providers.Lookup(" GEMINI ")
	if err != nil {
		t.Fatalf("Lookup(gemini) error = %v", err)
	}
	if entry.Identity() != "gemini" {
		t.Fatalf("Lookup identity = %q, want gemini", entry.Identity())
	}
	integration, err := providers.Integration("gemini")
	if err != nil {
		t.Fatalf("Integration(gemini) error = %v", err)
	}
	if integration.Identity() != "gemini" {
		t.Fatalf("Integration identity = %q, want gemini", integration.Identity())
	}
	maximum := integration.MaximumCapabilities()
	if !maximum.Has(inference.CapabilityPromptSubmission) || !maximum.Has(inference.CapabilityMessageSnapshots) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt_submission and message_snapshots", maximum.Values())
	}
	if providers.UsesNativeRunner("gemini") {
		t.Fatal("UsesNativeRunner(gemini) = true, want conductor route for migrated Gemini")
	}
	if providers.UsesNativeRunner(workers.RunnerIDCodex) || providers.UsesNativeRunner("claude") {
		t.Fatal("UsesNativeRunner(codex/claude) = true, want conductor route for migrated built-ins")
	}
}

func TestConductorInvokesGeminiWithoutConcreteProviderSwitch(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{result: workerprocess.CommandResult{Stdout: []byte("gemini conductor answer")}}
	providers := newGeminiRegistryWithRunner(t, runner)
	destination := &recordingDestination{}

	err := conductor.New(providers).Invoke(
		context.Background(),
		"gemini",
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-gemini-conductor",
			Model:        "gemini-2.5-pro",
			UserMessage:  "say hello",
			Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		}),
		destination,
	)
	if err != nil {
		t.Fatalf("conductor.Invoke(gemini) error = %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want successful response", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "gemini conductor answer" {
		t.Fatalf("response content = %q, want gemini conductor answer", got)
	}
	if runner.calls != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.calls)
	}
	if runner.request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", runner.request.Command)
	}
	if !containsArgPair(runner.request.Args, "--prompt", "say hello") {
		t.Fatalf("args = %#v, want --prompt say hello", runner.request.Args)
	}
	if !containsArgPair(runner.request.Args, "--model", "gemini-2.5-pro") {
		t.Fatalf("args = %#v, want --model gemini-2.5-pro", runner.request.Args)
	}
}

func TestConductorRejectsGeminiCapabilityEscalationBeforeProviderIO(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{result: workerprocess.CommandResult{Stdout: []byte("should-not-run")}}
	providers := newGeminiRegistryWithRunner(t, runner)
	destination := &recordingDestination{}

	err := conductor.New(providers).Invoke(
		context.Background(),
		"gemini",
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-gemini-escalate",
			UserMessage:  "hello",
			Required: inference.NewCapabilitySet(
				inference.CapabilityPromptSubmission,
				inference.CapabilitySessionResume,
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
	if rejection.Capability() != string(inference.CapabilitySessionResume) {
		t.Fatalf("Capability() = %q, want session_resume", rejection.Capability())
	}
	if runner.calls != 0 {
		t.Fatalf("provider I/O occurred: command runner calls = %d", runner.calls)
	}
	if destination.completion != nil {
		t.Fatalf("destination received completion %#v despite preflight rejection", destination.completion)
	}
}

func TestConductorClassifiesGeminiNativeFailureSafely(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{result: workerprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota path /tmp/secret-key leaked"}}`),
	}}
	providers := newGeminiRegistryWithRunner(t, runner)
	destination := &recordingDestination{}

	err := conductor.New(providers).Invoke(
		context.Background(),
		"gemini",
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-gemini-failure",
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
	if failure.Kind() != inference.FailureThrottled {
		t.Fatalf("failure kind = %q, want throttled", failure.Kind())
	}
	if strings.Contains(failure.Message(), "/tmp/") ||
		strings.Contains(failure.Message(), "secret-key") ||
		strings.Contains(failure.Message(), "private prompt") {
		t.Fatalf("failure message leaked unsafe detail: %q", failure.Message())
	}
}

func TestConductorPublishesOnlyCanonicalGeminiFailureMessages(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		stderr      string
		wantKind    inference.FailureKind
		wantMessage string
		rejected    []string
	}{
		{
			name:        "UnmarkedPromptInStructuredFailure",
			stderr:      `{"error":{"status":"INVALID_ARGUMENT","message":"Retell the private launch roadmap verbatim"}}`,
			wantKind:    inference.FailureInvalidRequest,
			wantMessage: "Gemini rejected the request.",
			rejected:    []string{"launch roadmap", "verbatim"},
		},
		{
			name:        "MachineLocalHomePathsInUnknownFailure",
			stderr:      `Error: failed reading C:\Users\alice\private\project and /home/alice/private/project`,
			wantKind:    inference.FailureUnknown,
			wantMessage: "Gemini invocation failed.",
			rejected:    []string{`C:\Users\alice`, "/home/alice", "private/project"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingCommandRunner{result: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(tc.stderr)}}
			destination := &recordingDestination{}
			err := conductor.New(newGeminiRegistryWithRunner(t, runner)).Invoke(
				context.Background(),
				"gemini",
				inference.NewInvocationRequest(inference.InvocationInput{
					InvocationID: "inv-gemini-safe-" + tc.name,
					UserMessage:  "customer request without a marker",
					Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
				}),
				destination,
			)
			if err != nil {
				t.Fatalf("conductor.Invoke() error = %v", err)
			}
			failure := destination.completion.Failure()
			if failure == nil || failure.Kind() != tc.wantKind || failure.Message() != tc.wantMessage {
				t.Fatalf("failure = %#v, want kind=%q message=%q", failure, tc.wantKind, tc.wantMessage)
			}
			for _, rejected := range tc.rejected {
				if strings.Contains(failure.Message(), rejected) {
					t.Fatalf("failure message leaked %q: %q", rejected, failure.Message())
				}
			}
		})
	}
}

func TestConductorNormalizesGeminiCommandCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	runner := &cancelingCommandRunner{cancel: cancel}
	destination := &recordingDestination{}
	err := conductor.New(newGeminiRegistryWithRunner(t, runner)).Invoke(
		ctx,
		"gemini",
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-gemini-canceled",
			UserMessage:  "cancel this invocation",
			Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		}),
		destination,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("conductor.Invoke() error = %v, want context.Canceled", err)
	}
	if runner.calls != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.calls)
	}
	failure := destination.completion.Failure()
	if failure == nil ||
		failure.Kind() != inference.FailureCanceled ||
		failure.Message() != "provider invocation was canceled" ||
		failure.Retryable() {
		t.Fatalf("failure = %#v, want canonical non-retryable cancellation", failure)
	}
}

func newProductionGeminiRegistry(t *testing.T) *registry.Registry {
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

func newGeminiRegistryWithRunner(t *testing.T, runner workerprocess.CommandRunner) *registry.Registry {
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
	return providers
}

type recordingCommandRunner struct {
	calls   int
	request workerprocess.CommandRequest
	result  workerprocess.CommandResult
	err     error
}

type cancelingCommandRunner struct {
	cancel context.CancelFunc
	calls  int
}

func (r *cancelingCommandRunner) Run(ctx context.Context, _ workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.calls++
	r.cancel()
	return workerprocess.CommandResult{}, ctx.Err()
}

func (r *recordingCommandRunner) Run(_ context.Context, request workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.calls++
	r.request = request
	return r.result, r.err
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

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
