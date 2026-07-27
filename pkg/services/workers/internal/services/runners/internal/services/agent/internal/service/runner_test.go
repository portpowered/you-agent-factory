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
	var published int
	runner, err := New(fake, func(workers.ProgressFragment) { published++ })
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.Execute(t.Context(), baseAgentRequest())
	if err == nil {
		t.Fatal("Execute() error = nil, want provider failure")
	}
	if published != 0 {
		t.Fatalf("progress publications = %d, want none without diagnostics", published)
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

func TestExecuteRejectsUnsupportedCapability(t *testing.T) {
	t.Parallel()

	runner, err := New(&providersFake{}, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-capability",
		},
		RunnerID:    string(providers.IDCodex),
		UserMessage: "user",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityImageInput,
		},
	})
	var unsupported *workers.UnsupportedRunnerCapabilityError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Execute() error = %v, want UnsupportedRunnerCapabilityError", err)
	}
}

func baseAgentRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-agent-1",
		},
		RunnerID:    string(providers.IDCodex),
		SystemPrompt: "system",
		UserMessage: "user",
	}
}

type providersFake struct {
	request providers.ExecuteRequest
}

type failingProvidersFake struct {
	failure providers.ExecuteFailure
}

type pointerFailureProvidersFake struct {
	failure *providers.ExecuteFailure
}

func noopPublisher(workers.ProgressFragment) {}

func (fake *providersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.request = request.Clone()
	return providers.ExecuteResult{Content: "ok"}, nil
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
