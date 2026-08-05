package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, noopPublisher); err == nil {
		t.Fatal("New(nil publisher) error = nil, want missing Providers service")
	}
	if _, err := New(&providersFake{}, nil); err == nil {
		t.Fatal("New(nil publish) error = nil, want missing progress publisher")
	}
}

func TestExecuteForwardsEnvThroughProviderRequest(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-env-1",
			WorkerType:      "goal-executor",
			WorkstationName: "execute-goal",
		},
		RunnerID:           string(providers.IDCodex),
		WorkerType:         "goal-executor",
		WorkstationType:    "execute-goal",
		SystemPrompt:       "system",
		UserMessage:        "user",
		EnvVars:            map[string]string{"FIXTURE": "configured"},
		ProcessEnvironment: []string{"FIXTURE=configured"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := providers.ExecuteRequest{
		Provider:           providers.IDCodex,
		AttemptID:          "dispatch-env-1",
		WorkerType:         "goal-executor",
		WorkstationName:    "execute-goal",
		SystemPrompt:       "system",
		UserMessage:        "user",
		EnvVars:            map[string]string{"FIXTURE": "configured"},
		ProcessEnvironment: []string{"FIXTURE=configured"},
	}
	if !reflect.DeepEqual(fake.request, want) {
		t.Fatalf("Providers.Execute request = %#v, want %#v", fake.request, want)
	}
}

func TestExecuteCanonicalizesTimeoutAndUnknownFailureMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		failure providers.ExecuteFailure
		wantMsg string
	}{
		{
			name: "timeout execution timeout message",
			failure: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindTimeout,
				Message: "execution timeout",
			},
			wantMsg: agentTimeoutFailureMessage,
		},
		{
			name: "timeout empty message",
			failure: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindTimeout,
			},
			wantMsg: agentTimeoutFailureMessage,
		},
		{
			name: "timeout provider-specific message",
			failure: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindTimeout,
				Message: "Cursor request timed out.",
			},
			wantMsg: agentTimeoutFailureMessage,
		},
		{
			name: "unknown empty message",
			failure: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindUnknown,
			},
			wantMsg: "provider invocation failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &failingProvidersFake{failure: test.failure}
			runner, err := New(fake, noopPublisher)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			_, err = runner.Execute(t.Context(), baseAgentRequest())
			var providerErr *workers.ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
			}
			if providerErr.Message != test.wantMsg {
				t.Fatalf("ProviderError.Message = %q, want %q", providerErr.Message, test.wantMsg)
			}
		})
	}
}

func TestExecuteFailurePreservesSessionRefAndBoundsMessages(t *testing.T) {
	t.Parallel()

	longMessage := strings.Repeat("x", failureMessageRuneLimit+32)
	failure := providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: longMessage,
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "failure-session-1",
		},
	}
	fake := &failingProvidersFake{failure: failure}
	var published []workers.ProgressFragment
	runner, err := New(fake, func(fragment workers.ProgressFragment) { published = append(published, fragment) })
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.SessionID = "resume-session-1"
	result, err := runner.Execute(t.Context(), request)
	if err == nil {
		t.Fatal("Execute() error = nil, want provider failure")
	}
	if len(published) != 1 {
		t.Fatalf("progress publications = %d, want terminal failure", len(published))
	}
	if published[0].Kind != workers.FailedFragmentKind || published[0].Payload != longMessage[:failureMessageRuneLimit] {
		t.Fatalf("terminal failure publication = %#v", published[0])
	}
	wantSession := &workers.ProviderSessionMetadata{
		Provider: string(providers.IDCodex),
		Kind:     providers.SessionIDKind,
		ID:       "failure-session-1",
	}
	if !reflect.DeepEqual(result.ProviderSession, wantSession) {
		t.Fatalf("ProviderSession = %#v, want %#v", result.ProviderSession, wantSession)
	}
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if len([]rune(providerErr.Message)) != failureMessageRuneLimit {
		t.Fatalf("ProviderError.Message length = %d, want %d runes", len([]rune(providerErr.Message)), failureMessageRuneLimit)
	}
	if !reflect.DeepEqual(providerErr.ProviderSession, wantSession) {
		t.Fatalf("ProviderError.ProviderSession = %#v, want failure session %#v", providerErr.ProviderSession, wantSession)
	}
}

func TestExecuteFailureAcceptsPointerExecuteFailure(t *testing.T) {
	t.Parallel()

	failure := providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindThrottled,
		Message: "rate limited",
	}
	fake := &pointerFailureProvidersFake{failure: &failure}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), baseAgentRequest())
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypeThrottled {
		t.Fatalf("ProviderError.Type = %q, want throttled", providerErr.Type)
	}
}

func TestExecuteResumesThroughContinueWhenSessionIDPresent(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.SessionID = "resume-session-2"
	if _, err := runner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantReference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "resume-session-2",
	}
	if fake.continuationReference == nil || *fake.continuationReference != wantReference {
		t.Fatalf("Providers.Continue Reference = %#v, want %#v", fake.continuationReference, wantReference)
	}
}

func TestExecuteResumesThroughExactWorkerSessionReferenceWithoutLegacyReconstruction(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	exact := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     "provider-native-thread",
		ID:       "opaque-provider-session",
	}
	wantReference := exact.Clone()
	request := baseAgentRequest()
	request.SessionID = "legacy-session-that-must-not-be-used"
	request.ResumeSession = &exact
	if _, err := runner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if fake.continuationReference == nil || *fake.continuationReference != wantReference {
		t.Fatalf("Providers.Continue Reference = %#v, want exact %#v", fake.continuationReference, wantReference)
	}
	request.ResumeSession.ID = "caller-mutated"
	if fake.continuationReference.ID != wantReference.ID {
		t.Fatalf("Providers.Continue retained caller ResumeSession mutation: %#v", fake.continuationReference)
	}
	if fake.executeCalls != 0 {
		t.Fatalf("Providers.Execute calls = %d, want 0 for exact continuation", fake.executeCalls)
	}
}

func TestExecuteUnsupportedContinuationReturnsProviderError(t *testing.T) {
	t.Parallel()

	fake := &unsupportedContinuationProvidersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.SessionID = "resume-session-unsupported"
	_, err = runner.Execute(t.Context(), request)
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypePermanentBadRequest {
		t.Fatalf("ProviderError.Type = %q, want %q", providerErr.Type, workers.WorkFailureTypePermanentBadRequest)
	}
	if fake.executeCalls != 0 {
		t.Fatalf("Providers.Execute calls = %d, want 0 (unsupported continuation must not fall back to a fresh attempt)", fake.executeCalls)
	}
}

func TestExecuteForwardsInputTokensToProviders(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tokens := []any{"image-token"}
	_, err = runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-capability",
		},
		RunnerID:    string(providers.IDCodex),
		UserMessage: "user",
		InputTokens: tokens,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityImageInput,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(fake.request.InputTokens) != 1 || fake.request.InputTokens[0] != "image-token" {
		t.Fatalf("Providers.ExecuteRequest.InputTokens = %#v", fake.request.InputTokens)
	}
}

func baseAgentRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-agent-1",
		},
		RunnerID:     string(providers.IDCodex),
		SystemPrompt: "system",
		UserMessage:  "user",
	}
}

type providersFake struct {
	providers.Service
	request               providers.ExecuteRequest
	continuationReference *providers.SessionRef
	executeCalls          int
}

type failingProvidersFake struct {
	providers.Service
	failure providers.ExecuteFailure
}

type pointerFailureProvidersFake struct {
	providers.Service
	failure *providers.ExecuteFailure
}

type unsupportedContinuationProvidersFake struct {
	providers.Service
	executeCalls int
}

func noopPublisher(workers.ProgressFragment) {}

func (fake *providersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeCalls++
	fake.request = request.Clone()
	return providers.ExecuteResult{Content: "ok"}, nil
}

func (fake *providersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	fake.request = request.Attempt.Clone()
	reference := request.Reference.Clone()
	fake.continuationReference = &reference
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    providers.ExecuteResult{Content: "ok"},
	}, nil
}

func (*providersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*providersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *failingProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, fake.failure
}

func (fake *failingProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{}, fake.failure
}

func (fake *failingProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *failingProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *pointerFailureProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, fake.failure
}

func (fake *pointerFailureProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *pointerFailureProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *unsupportedContinuationProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeCalls++
	return providers.ExecuteResult{Content: "ok"}, nil
}

func (fake *unsupportedContinuationProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

func (*unsupportedContinuationProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*unsupportedContinuationProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}
