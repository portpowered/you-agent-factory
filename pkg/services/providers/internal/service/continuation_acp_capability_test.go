package service_test

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// negotiatedCapabilityACPService is a fake acp.Service reporting one
// configured ACP integration whose static descriptor always claims
// CapabilitySessionResume (mirroring the real config-time acpDescriptor),
// but whose live NegotiatedCapabilities answer is armed independently by
// each test. It proves Continue prefers a daemon's real negotiated
// LoadSession truth once known, instead of trusting the static claim
// forever, without ever calling Execute to find that out.
type negotiatedCapabilityACPService struct {
	provider     providers.ID
	known        bool
	loadSession  bool
	executeCalls int
}

func (s *negotiatedCapabilityACPService) Close(context.Context) error { return nil }

func (s *negotiatedCapabilityACPService) Configure(context.Context, []providers.ACPIntegration) error {
	return nil
}

func (s *negotiatedCapabilityACPService) Integrations() []providers.ACPIntegration {
	return []providers.ACPIntegration{{Name: s.provider}}
}

func (s *negotiatedCapabilityACPService) Resolve(id providers.ID) (providers.ID, bool) {
	return s.provider, id == s.provider
}

func (s *negotiatedCapabilityACPService) Execute(
	context.Context,
	providers.ID,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	s.executeCalls++
	return providers.ExecuteResult{Content: "acp resumed reply"}, nil
}

func (s *negotiatedCapabilityACPService) Continue(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
	_ providers.SessionRef,
) (providers.ExecuteResult, error) {
	return s.Execute(ctx, id, request)
}

func (s *negotiatedCapabilityACPService) Claim(providers.ID, string) (acp.Generation, bool) {
	return nil, false
}

func (s *negotiatedCapabilityACPService) TryCancel(context.Context, acp.Generation) (bool, error) {
	return false, nil
}

func (s *negotiatedCapabilityACPService) NegotiatedCapabilities(
	providers.ID,
) (acpsdk.AgentCapabilities, bool) {
	return acpsdk.AgentCapabilities{LoadSession: s.loadSession}, s.known
}

var _ acp.ContinuationService = (*negotiatedCapabilityACPService)(nil)

func mustNegotiatedCapabilityRootService(
	t *testing.T,
	acpService *negotiatedCapabilityACPService,
) providers.Service {
	t.Helper()
	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.NewWithACP(catalogService, executionService, acpService, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}
	return root
}

func TestRootContinueFallsBackToConfiguredClaimBeforeFirstHandshake(t *testing.T) {
	t.Parallel()

	fake := &negotiatedCapabilityACPService{provider: "cursor-acp", known: false}
	root := mustNegotiatedCapabilityRootService(t, fake)

	reference := providers.SessionRef{Provider: "cursor-acp", Kind: providers.SessionIDKind, ID: "session-1"}
	continued, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt:   providers.ExecuteRequest{Provider: "cursor-acp", AttemptID: "attempt-1"},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeResumed {
		t.Fatalf("Continue().Outcome = %q, want resumed (static claim, never yet contradicted)", continued.Outcome)
	}
	if fake.executeCalls != 1 {
		t.Fatalf("executeCalls = %d, want 1", fake.executeCalls)
	}
}

func TestRootContinueUnsupportedWhenDaemonNegotiatesNoLoadSession(t *testing.T) {
	t.Parallel()

	fake := &negotiatedCapabilityACPService{provider: "cursor-acp", known: true, loadSession: false}
	root := mustNegotiatedCapabilityRootService(t, fake)

	reference := providers.SessionRef{Provider: "cursor-acp", Kind: providers.SessionIDKind, ID: "session-1"}
	continued, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt:   providers.ExecuteRequest{Provider: "cursor-acp", AttemptID: "attempt-1"},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeUnsupported {
		t.Fatalf(
			"Continue().Outcome = %q, want unsupported -- a daemon that negotiated LoadSession=false must not be "+
				"reported as resumable just because the static config-time descriptor unconditionally claims it can",
			continued.Outcome,
		)
	}
	if fake.executeCalls != 0 {
		t.Fatalf("executeCalls = %d, want 0 - unsupported must not start a fresh provider process", fake.executeCalls)
	}
}

func TestRootContinueResumesWhenDaemonNegotiatesLoadSession(t *testing.T) {
	t.Parallel()

	fake := &negotiatedCapabilityACPService{provider: "cursor-acp", known: true, loadSession: true}
	root := mustNegotiatedCapabilityRootService(t, fake)

	reference := providers.SessionRef{Provider: "cursor-acp", Kind: providers.SessionIDKind, ID: "session-1"}
	continued, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: reference,
		Attempt:   providers.ExecuteRequest{Provider: "cursor-acp", AttemptID: "attempt-1"},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeResumed {
		t.Fatalf("Continue().Outcome = %q, want resumed", continued.Outcome)
	}
	if continued.Result.Content != "acp resumed reply" {
		t.Fatalf("Continue().Result.Content = %q, want acp resumed reply", continued.Result.Content)
	}
	if fake.executeCalls != 1 {
		t.Fatalf("executeCalls = %d, want 1", fake.executeCalls)
	}
}
