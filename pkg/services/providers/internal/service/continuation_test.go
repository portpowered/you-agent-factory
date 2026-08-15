package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	var received execution.ContinuationRequest
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency}
			},
			Continue: func(_ context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
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

func TestRootContinueRejectsUnsupportedSessionKindBeforeAdapterDispatch(t *testing.T) {
	t.Parallel()

	adapterCalls := 0
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{}, nil
			},
			Continue: func(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
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

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: "external-session-id", ID: "foreign-1"}
	_, err = root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"},
	})
	if !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("Continue(unsupported kind) error = %v, want ErrInvalidContinuationRequest", err)
	}
	var failure providers.ContinuationFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindInvalid {
		t.Fatalf("Continue(unsupported kind) error = %#v, want invalid ContinuationFailure", err)
	}
	if failure.Reference != reference {
		t.Fatalf("ContinuationFailure.Reference = %#v, want %#v", failure.Reference, reference)
	}
	if adapterCalls != 0 {
		t.Fatalf("adapter calls = %d, want 0 for rejected continuation kind", adapterCalls)
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

// TestRootContinueStaleWhenProviderReportsSessionNotFound proves that a
// provider declaring ExecuteFailureKindSessionNotFound for a continuation
// attempt (the vocabulary a real adapter uses when its native "no such
// session" signal is observed - see the codex/claude/ACP adapters) surfaces
// through Continue as the typed stale continuation failure, not the raw
// ExecuteFailure a caller would otherwise have to pattern-match themselves.
// Exactly one adapter call must happen: the resume attempt that discovered
// staleness, and nothing else - no retry, no fresh-session fallback attempt.
func TestRootContinueStaleWhenProviderReportsSessionNotFound(t *testing.T) {
	t.Parallel()

	adapterCalls := 0
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency}
			},
			Continue: func(_ context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
				adapterCalls++
				if request.ResumeSession == nil {
					t.Fatalf("adapter received request with nil ResumeSession, want the continued reference")
				}
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind:    providers.ExecuteFailureKindSessionNotFound,
					Message: "codex: no rollout found for thread id",
				}
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

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-expired"}
	continued, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"},
	})
	if (continued != providers.ContinueResult{}) {
		t.Fatalf("Continue().result = %#v, want zero value on stale failure", continued)
	}
	var failure providers.ContinuationFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindStale {
		t.Fatalf("Continue(stale reference) error = %#v, want ContinuationFailureKindStale", err)
	}
	if !errors.Is(err, providers.ErrContinuationStale) {
		t.Fatalf("Continue(stale reference) error does not unwrap to ErrContinuationStale: %v", err)
	}
	if failure.Reference != reference {
		t.Fatalf("ContinuationFailure.Reference = %#v, want %#v", failure.Reference, reference)
	}
	if adapterCalls != 1 {
		t.Fatalf("adapter calls = %d, want exactly 1 - a stale reference must not retry or fall back to a fresh session", adapterCalls)
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
			Continue: func(_ context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
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

// TestRootContinueConcurrentStaleAndResumedAttemptsStayIndependent runs many
// concurrent Continue calls against distinct attempt identities that
// deliberately interleave a stale reference (session id ending in "-stale")
// with an ordinary resumable one against the same shared provider adapter.
// Attempt-ownership binding is shared registry state guarding every
// concurrent Continue/Execute call; this proves a reference that ends up
// classified stale can never leak its typed outcome onto a concurrently
// running resumable attempt, or vice versa, and that every goroutine
// observes exactly the one outcome its own reference deserves - not a
// duplicate or a neighbor's result.
func TestRootContinueConcurrentStaleAndResumedAttemptsStayIndependent(t *testing.T) {
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
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency}
			},
			Continue: func(_ context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
				if request.ResumeSession != nil && strings.HasSuffix(request.ResumeSession.ID, "-stale") {
					return providers.ExecuteResult{}, providers.ExecuteFailure{
						Kind:    providers.ExecuteFailureKindSessionNotFound,
						Message: "no rollout found for thread id",
					}
				}
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

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for index := range concurrency {
		go func(index int) {
			defer wg.Done()
			stale := index%2 == 0
			sessionID := fmt.Sprintf("session-%d", index)
			if stale {
				sessionID += "-stale"
			}
			reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: sessionID}
			continued, err := root.Continue(context.Background(), providers.ContinueRequest{
				Reference: reference,
				Attempt: providers.ExecuteRequest{
					Provider:  providers.IDCodex,
					AttemptID: fmt.Sprintf("attempt-concurrent-mixed-%d", index),
				},
			})
			if stale {
				var failure providers.ContinuationFailure
				if !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindStale {
					t.Errorf("concurrent Continue(stale) error = %#v, want ContinuationFailureKindStale", err)
					return
				}
				if failure.Reference != reference {
					t.Errorf("concurrent Continue(stale) failure.Reference = %#v, want %#v", failure.Reference, reference)
				}
				return
			}
			if err != nil {
				t.Errorf("concurrent Continue(resumable) error = %v, want nil", err)
				return
			}
			if continued.Outcome != providers.ContinuationOutcomeResumed {
				t.Errorf("concurrent Continue(resumable).Outcome = %q, want resumed", continued.Outcome)
			}
			if continued.Reference != reference {
				t.Errorf("concurrent Continue(resumable).Reference = %#v, want %#v", continued.Reference, reference)
			}
		}(index)
	}
	wg.Wait()
}

func TestContinueReferenceRoutesExactOpaqueIdentity(t *testing.T) {
	t.Parallel()

	var received execution.ContinuationRequest
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency}
			},
			Continue: func(_ context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
				received = request.Clone()
				return providers.ExecuteResult{
					Content: "opaque continuation reply",
					SessionRef: &providers.SessionRef{
						Provider: providers.IDCodex,
						Kind:     providers.SessionIDKind,
						ID:       "provider-session-99",
					},
				}, nil
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

	request := providers.ContinueReferenceRequest{
		Reference: providers.ContinuationRef{
			Provider:          string(providers.IDCodex),
			Kind:              providers.SessionIDKind,
			ProviderSessionID: "provider-session-99",
			ExternalRef:       "provider-opaque-token-99",
		},
		Attempt: providers.ExecuteRequest{
			Provider:    providers.IDCodex,
			AttemptID:   "attempt-opaque-99",
			UserMessage: "continue without selecting a new session",
		},
	}
	continued, err := providers.ContinueReference(context.Background(), root, request)
	if err != nil {
		t.Fatalf("ContinueReference() error = %v", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeResumed ||
		continued.Result.Content != "opaque continuation reply" {
		t.Fatalf("ContinueReference() = %#v, want resumed opaque result", continued)
	}
	if continued.Reference.Provider != string(providers.IDCodex) ||
		continued.Reference.Kind != providers.SessionIDKind ||
		continued.Reference.ProviderSessionID != "provider-session-99" ||
		continued.Reference.ExternalRef != "provider-opaque-token-99" {
		t.Fatalf("ContinueReference().Reference = %#v, want exact detached identity", continued.Reference)
	}
	if received.ResumeSession == nil || *received.ResumeSession != (providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-99",
	}) {
		t.Fatalf("provider continuation reference = %#v, want exact provider session", received.ResumeSession)
	}
}

func TestContinueReferenceUnsupportedNeverExecutes(t *testing.T) {
	t.Parallel()

	adapterCalls := 0
	catalogService := noResumeCapabilityCatalogStub{descriptor: providers.Descriptor{
		ID:           providers.IDCodex,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{Content: "must not execute"}, nil
			},
			Continue: func(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{Content: "must not continue"}, nil
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
	continued, err := providers.ContinueReference(context.Background(), root, providers.ContinueReferenceRequest{
		Reference: providers.ContinuationRef{
			Provider:          string(providers.IDCodex),
			ProviderSessionID: "unsupported-session",
		},
		Attempt: providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-unsupported-opaque",
		},
	})
	if err != nil {
		t.Fatalf("ContinueReference() error = %v", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeUnsupported {
		t.Fatalf("ContinueReference().Outcome = %q, want unsupported", continued.Outcome)
	}
	if adapterCalls != 0 {
		t.Fatalf("provider adapter calls = %d, want 0", adapterCalls)
	}
}

func TestContinueReferenceClassifiesForeignAttempt(t *testing.T) {
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
	_, err = providers.ContinueReference(context.Background(), root, providers.ContinueReferenceRequest{
		Reference: providers.ContinuationRef{
			Provider:          string(providers.IDCodex),
			ProviderSessionID: "session-foreign-attempt",
		},
		Attempt: providers.ExecuteRequest{
			Provider:  providers.IDClaude,
			AttemptID: "attempt-foreign-opaque",
		},
	})
	var failure providers.ContinuationFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindForeign {
		t.Fatalf("ContinueReference() error = %#v, want foreign ContinuationFailure", err)
	}
}
