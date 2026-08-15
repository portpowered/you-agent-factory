package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecuteOpaqueContinuationPreservesKindAndIdentity(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := baseAgentRequest()
	request.RunnerID = ""
	request.Continuation = &workers.ProviderContinuationRef{
		Provider:          string(providers.IDCodex),
		Kind:              "provider-native-thread",
		ProviderSessionID: "opaque-provider-session",
		ExternalRef:       "opaque-provider-token",
	}
	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fake.executeCalls != 0 || fake.continueCalls != 1 {
		t.Fatalf("Providers calls execute=%d continue=%d, want execute=0 continue=1", fake.executeCalls, fake.continueCalls)
	}
	if fake.continuationReference == nil || fake.continuationReference.Kind != "provider-native-thread" ||
		fake.continuationReference.ID != "opaque-provider-session" {
		t.Fatalf("Providers continuation reference = %#v, want exact kind and session id", fake.continuationReference)
	}
	if result.ProviderSession == nil || result.ProviderSession.Kind != "provider-native-thread" ||
		result.ProviderSession.ID != "opaque-provider-session" {
		t.Fatalf("runner result ProviderSession = %#v, want exact continuation identity", result.ProviderSession)
	}
}

func TestExecutePreservesProviderOutcomeAndStructuredDiagnostics(t *testing.T) {
	t.Parallel()

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: "provider-thread", ID: "provider-session"}
	fake := &resultProvidersFake{result: providers.ExecuteResult{
		Content:    "structured provider output",
		Outcome:    providers.ExecuteOutcomeAccepted,
		SessionRef: &reference,
		Diagnostics: &providers.ExecuteDiagnostics{
			Metadata: map[string]string{"phase": "complete"},
			Command: &providers.ExecuteCommandDiagnostics{
				Command: "codex", Args: []string{"--json"}, Env: map[string]string{"KEY": "value"}, DurationMS: 1500,
			},
			Panic: &providers.ExecutePanicDiagnostics{Message: "bounded", Stack: "bounded-stack"},
		},
	}}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := runner.Execute(t.Context(), baseAgentRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertProviderOutcome(t, result)
	assertProviderDiagnostics(t, result.Diagnostics)
}

func assertProviderOutcome(t *testing.T, result workers.RunnerExecutionResult) {
	t.Helper()
	if result.Content != "structured provider output" || result.Outcome != workers.WorkOutcome(providers.ExecuteOutcomeAccepted) || result.ProviderSession == nil || result.ProviderSession.Kind != "provider-thread" || result.ProviderSession.ID != "provider-session" {
		t.Fatalf("runner result = %#v, want outcome and exact provider session", result)
	}
}

func assertProviderDiagnostics(t *testing.T, diagnostics *workers.WorkDiagnostics) {
	t.Helper()
	if diagnostics == nil || diagnostics.Command == nil || diagnostics.Command.Duration != 1500*time.Millisecond || diagnostics.Panic == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata["phase"] != "complete" {
		t.Fatalf("runner diagnostics = %#v, want structured command, panic, and metadata", diagnostics)
	}
}

func TestExecuteNormalizesUnexpectedErrorAfterProviderCancelsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	runner, err := New(&cancelingErrorProvidersFake{cancel: cancel}, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = runner.Execute(ctx, baseAgentRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestExecuteReportsUnsupportedContinuationWithoutFallingBackToExecute(t *testing.T) {
	t.Parallel()

	fake := &noContinuationProvidersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := baseAgentRequest()
	request.SessionID = "session-without-capability"
	_, err = runner.Execute(t.Context(), request)
	assertUnsupportedContinuation(t, err)
	if fake.executeCalls != 0 {
		t.Fatalf("Providers.Execute calls = %d, want 0 after unsupported continuation", fake.executeCalls)
	}
}

func assertUnsupportedContinuation(t *testing.T, err error) {
	t.Helper()
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.ProviderContinuationOutcome != providers.ContinuationOutcomeUnsupported {
		t.Fatalf("ProviderError continuation outcome = %q, want unsupported", providerErr.ProviderContinuationOutcome)
	}
	var unsupported continuationUnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Execute() error = %v, want the unsupported continuation cause", err)
	}
	if unsupported.Error() != "provider does not support resuming this Provider Session" {
		t.Fatalf("unsupported continuation message = %q", unsupported.Error())
	}
}

func TestExecuteFailurePublishesProviderDiagnosticsAndUsesDiagnosticFailureType(t *testing.T) {
	t.Parallel()

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "diagnostic-failure-session"}
	fake := &failingProvidersFake{failure: providers.ExecuteFailure{
		Kind:       providers.ExecuteFailureKindDependency,
		Message:    "dependency unavailable",
		SessionRef: &reference,
		Diagnostics: &providers.ExecuteDiagnostics{
			Progress:       []providers.ExecuteProgress{{Phase: "provider.failed", Detail: "dependency fact"}},
			Metadata:       map[string]string{"work-failure-type": string(workers.WorkFailureTypeMissingExecutable)},
			DurationMillis: 42,
		},
	}}
	var published []workers.ProgressFragment
	runner, err := New(fake, func(fragment workers.ProgressFragment) { published = append(published, fragment) })
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := runner.Execute(t.Context(), baseAgentRequest())
	assertDiagnosticFailure(t, result, err)
	if len(published) != 2 || published[0].Kind != workers.ProgressFragmentKind ||
		published[0].Payload != "dependency fact" || published[1].Kind != workers.FailedFragmentKind {
		t.Fatalf("published failure observations = %#v, want provider diagnostic then terminal failure", published)
	}
}

func assertDiagnosticFailure(t *testing.T, result workers.RunnerExecutionResult, err error) {
	t.Helper()
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypeMissingExecutable {
		t.Fatalf("ProviderError.Type = %q, want diagnostic missing-executable classification", providerErr.Type)
	}
	if result.Diagnostics == nil || result.Diagnostics.Provider == nil ||
		result.Diagnostics.Provider.ResponseMetadata[workers.ProviderResponseMetadataDurationMS] != "42" {
		t.Fatalf("failure diagnostics = %#v, want preserved provider duration", result.Diagnostics)
	}
}
