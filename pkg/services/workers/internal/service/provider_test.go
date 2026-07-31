package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
)

func TestExecuteResolvesProviderAliasAndReturnsContinuation(t *testing.T) {
	t.Parallel()

	var gotRequest workers.RunnerExecutionRequest
	runner := &stubRunner{
		execute: func(
			_ context.Context,
			request workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			gotRequest = workers.CloneProviderInferenceRequest(request)
			return workers.RunnerExecutionResult{
				Content: "resumed",
				ProviderSession: &workers.ProviderSessionMetadata{
					Provider: "codex",
					Kind:     providers.SessionIDKind,
					ID:       "session-resume-1",
				},
			}, nil
		},
	}
	service := mustExecuteService(t, runner, nil)
	request := validExecuteRequest("dispatch-resume", "attempt-resume")
	request.Target.Provider = workers.ProviderReference{Alias: "openai"}
	request.Target.RunnerID = ""
	request.Input.Resume = &workers.ProviderContinuationRef{
		Provider:          "openai",
		ProviderSessionID: "session-resume-1",
		ExternalRef:       "session-resume-1",
	}

	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if gotRequest.ExecutorProvider != string(providers.IDCodex) {
		t.Fatalf("ExecutorProvider = %q, want codex", gotRequest.ExecutorProvider)
	}
	if gotRequest.SessionID != "session-resume-1" {
		t.Fatalf("SessionID = %q, want session-resume-1", gotRequest.SessionID)
	}
	if result.Continuation == nil ||
		result.Continuation.Provider != "codex" ||
		result.Continuation.ProviderSessionID != "session-resume-1" {
		t.Fatalf("Continuation = %#v", result.Continuation)
	}
}

func TestExecuteRejectsUnknownProviderAlias(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{content: "unused"}, nil)
	request := validExecuteRequest("dispatch-unknown", "attempt-unknown")
	request.Target.Provider = workers.ProviderReference{Alias: "unknown-provider"}
	request.Target.RunnerID = ""

	_, err := service.Execute(context.Background(), request)
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidExecuteRequest", err)
	}
}

func TestExecuteRejectsUnavailableProvider(t *testing.T) {
	t.Parallel()

	service, err := executeservice.New(executeservice.Dependencies{
		Runners:   &staticRunners{runner: &stubRunner{content: "unused"}},
		Providers: &unavailableCursorProvidersFake{},
		Clock:     func() time.Time { return time.Unix(10, 0) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := validExecuteRequest("dispatch-blocked", "attempt-blocked")
	request.Target.Provider = workers.ProviderReference{ID: string(providers.IDCursor)}
	request.Target.RunnerID = workers.RunnerIDCursorCLI

	_, err = service.Execute(context.Background(), request)
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidExecuteRequest", err)
	}
}

type unavailableCursorProvidersFake struct{}

func (*unavailableCursorProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}

func (*unavailableCursorProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{
		Providers: []providers.Descriptor{{
			ID:           providers.IDCursor,
			DisplayName:  "Cursor",
			Availability: providers.AvailabilitySupportedButUnavailable,
			Readiness:    providers.ReadinessUnavailable,
			Prerequisites: []providers.Prerequisite{{
				Kind:   providers.PrerequisiteDependency,
				Name:   "cursor-agent",
				Status: providers.PrerequisiteMissing,
			}},
		}},
	}, nil
}

func (*unavailableCursorProvidersFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if request.ID != providers.IDCursor {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{}, providers.ErrProviderUnavailable
}
