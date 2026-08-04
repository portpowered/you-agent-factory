package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestRootContinueResumesExactSessionThroughNativeAdapter(t *testing.T) {
	t.Parallel()

	var received providers.ExecuteRequest
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
				received = request.Clone()
				return providers.ExecuteResult{Content: "resumed reply"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-99"}
	continued, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt: providers.ExecuteRequest{
			Provider:    providers.IDCodex,
			AttemptID:   "attempt-continue-1",
			UserMessage: "continue the prior turn",
		},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeResumed {
		t.Fatalf("Continue().Outcome = %q, want resumed", continued.Outcome)
	}
	if continued.Result.Content != "resumed reply" {
		t.Fatalf("Continue().Result.Content = %q, want resumed reply", continued.Result.Content)
	}
	if continued.Reference != reference {
		t.Fatalf("Continue().Reference = %#v, want %#v unchanged", continued.Reference, reference)
	}

	if received.Provider != providers.IDCodex {
		t.Fatalf("adapter received Provider = %q, want codex", received.Provider)
	}
	if received.ResumeSession == nil || *received.ResumeSession != reference {
		t.Fatalf("adapter received ResumeSession = %#v, want %#v", received.ResumeSession, reference)
	}
}

func TestRootContinueRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	_, err = root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDCodex + "-stale", Kind: providers.SessionIDKind, ID: "session-1"},
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex + "-stale", AttemptID: "attempt-1"},
	})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("Continue(unknown provider) error = %v, want ErrUnknownProvider", err)
	}
}

func TestRootExecuteRejectsRequestCarryingResumeSession(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	session := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"}
	_, err = root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:      providers.IDCodex,
		AttemptID:     "attempt-1",
		ResumeSession: &session,
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("Execute(with ResumeSession) error = %#v, want ExecuteFailureKindInvalidRequest", err)
	}
}

// noResumeCapabilityCatalogStub reports one provider descriptor that omits
// CapabilitySessionResume, so Continue must answer with the closed
// unsupported outcome instead of guessing the provider can resume.
type noResumeCapabilityCatalogStub struct {
	descriptor providers.Descriptor
}

func (stub noResumeCapabilityCatalogStub) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{Providers: []providers.Descriptor{stub.descriptor.Clone()}}, nil
}

func (stub noResumeCapabilityCatalogStub) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if request.ID != stub.descriptor.ID {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: stub.descriptor.Clone()}, nil
}

func (stub noResumeCapabilityCatalogStub) ResolveProviderID(id providers.ID) (providers.ID, error) {
	if id != stub.descriptor.ID {
		return "", providers.ErrUnknownProvider
	}
	return id, nil
}

func (stub noResumeCapabilityCatalogStub) RegistrationProvider(
	id providers.ID,
) (providers.Descriptor, error) {
	if id != stub.descriptor.ID {
		return providers.Descriptor{}, providers.ErrUnknownProvider
	}
	return stub.descriptor.Clone(), nil
}

var _ catalog.Service = noResumeCapabilityCatalogStub{}

func TestRootContinueUnsupportedWhenProviderCannotContinue(t *testing.T) {
	t.Parallel()

	adapterCalls := 0
	catalogService := noResumeCapabilityCatalogStub{descriptor: providers.Descriptor{
		ID:           providers.IDCodex,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"}
	continued, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeUnsupported {
		t.Fatalf("Continue().Outcome = %q, want unsupported", continued.Outcome)
	}
	if (continued.Result != providers.ExecuteResult{}) {
		t.Fatalf("Continue().Result = %#v, want zero value when unsupported", continued.Result)
	}
	if adapterCalls != 0 {
		t.Fatalf("adapter calls = %d, want 0 - unsupported must not start a fresh provider process", adapterCalls)
	}
}

func TestRootContinueConcurrentAttemptsAreIndependent(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{Content: request.AttemptID}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	const concurrency = 16
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for index := range concurrency {
		go func(index int) {
			defer wg.Done()
			attemptID := "attempt-concurrent"
			reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-shared"}
			continued, err := root.Continue(context.Background(), providers.ContinueRequest{
				Reference: reference,
				Attempt: providers.ExecuteRequest{
					Provider:  providers.IDCodex,
					AttemptID: attemptID + string(rune('a'+index%26)),
				},
			})
			if err != nil {
				t.Errorf("concurrent Continue() error = %v, want nil", err)
				return
			}
			if continued.Outcome != providers.ContinuationOutcomeResumed {
				t.Errorf("concurrent Continue().Outcome = %q, want resumed", continued.Outcome)
			}
		}(index)
	}
	wg.Wait()
}
