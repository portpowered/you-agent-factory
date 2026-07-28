package providers_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// TestRootContractInvariants_AllSlicesThroughSingularService seals the
// Providers root-contract packet: every published slice (catalog list/get and
// one-attempt Execute) is reachable through one named providers.Service, a
// peer-shaped fake can exercise success and typed-failure paths using only the
// root package, and no second peer-facing Providers authority is required.
func TestRootContractInvariants_AllSlicesThroughSingularService(t *testing.T) {
	t.Parallel()

	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
	service := newExecutePeerFake("cancel-attempt", codex)
	var root providers.Service = service

	assertSealCatalogSuccess(t, root)
	assertSealCatalogFailures(t, root, codex)
	assertSealExecuteSuccess(t, root)
	assertSealExecuteFailures(t, root)
}

func assertSealCatalogSuccess(t *testing.T, service providers.Service) {
	t.Helper()

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 1 || list.Providers[0].ID != providers.IDCodex {
		t.Fatalf("ListProviders() = %#v, want one codex descriptor", list.Providers)
	}

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.DisplayName != "Codex" ||
		got.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("GetProvider(codex) = %#v", got.Provider)
	}
}

func assertSealCatalogFailures(t *testing.T, service providers.Service, codex providers.Descriptor) {
	t.Helper()

	assertGetErrorIs(t, service, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDClaude}, providers.ErrUnknownProvider)

	unavailable := providers.Descriptor{
		ID:           providers.IDCursor,
		DisplayName:  "Cursor",
		Availability: providers.AvailabilitySupportedButUnavailable,
		Readiness:    providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:   providers.PrerequisiteDependency,
			Name:   "cursor-agent",
			Status: providers.PrerequisiteMissing,
		}},
	}
	blocked := newExecutePeerFake("cancel-attempt", codex, unavailable)
	assertGetErrorIs(
		t,
		blocked,
		providers.GetProviderRequest{ID: providers.IDCursor},
		providers.ErrProviderUnavailable,
	)
}

func assertSealExecuteSuccess(t *testing.T, service providers.Service) {
	t.Helper()

	result, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "seal-attempt",
		UserMessage: "hello seal",
	})
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "hello seal-result" {
		t.Fatalf("Execute() content = %q, want hello seal-result", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "session-seal-attempt" {
		t.Fatalf("Execute() SessionRef = %#v", result.SessionRef)
	}
}

func assertSealExecuteFailures(t *testing.T, service providers.Service) {
	t.Helper()

	_, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "cancel-attempt",
	})
	if !errors.Is(err, providers.ErrExecuteCancelled) {
		t.Fatalf("cancelled Execute() error = %v, want ErrExecuteCancelled", err)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("cancelled Execute() error = %T(%v), want ExecuteFailure", err, err)
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDClaude,
		AttemptID: "seal-unknown",
	})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("unknown provider Execute() error = %v, want ErrUnknownProvider", err)
	}
}

func TestRootContract_ContractValuesStayInertWhenHeld(t *testing.T) {
	t.Parallel()

	descriptor := providers.Descriptor{
		ID:          providers.IDCodex,
		Aliases:     []string{"openai-codex"},
		DisplayName: "Codex",
	}
	clonedDescriptor := descriptor.Clone()
	descriptor.DisplayName = "mutated"
	if clonedDescriptor.DisplayName == "mutated" {
		t.Fatal("Descriptor.Clone() shares mutable display name state")
	}

	request := providers.ExecuteRequest{
		Provider:           providers.IDCodex,
		AttemptID:          "inert-attempt",
		UserMessage:        "hello",
		EnvVars:            map[string]string{"FIXTURE": "original"},
		ProcessEnvironment: []string{"FIXTURE=original"},
	}
	clonedRequest := request.Clone()
	request.UserMessage = "mutated"
	request.EnvVars["FIXTURE"] = "mutated"
	request.ProcessEnvironment[0] = "FIXTURE=mutated"
	if clonedRequest.UserMessage == "mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable user message state")
	}
	if clonedRequest.EnvVars["FIXTURE"] == "mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable env vars state")
	}
	if clonedRequest.ProcessEnvironment[0] == "FIXTURE=mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable process environment state")
	}

	session := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-inert",
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("SessionRef.Validate() = %v", err)
	}
	if cloned := session.Clone(); cloned != session {
		t.Fatalf("SessionRef.Clone() = %#v, want %#v", cloned, session)
	}

	// Holding contract values must not require a Service implementation or
	// perform adapter/registry/conductor/process work.
	var (
		_ providers.Descriptor     = descriptor
		_ providers.ExecuteRequest = request
		_ providers.SessionRef     = session
		_ providers.ListProvidersRequest
		_ providers.GetProviderRequest
		_ providers.ExecuteResult
	)
}

func TestRootContract_FakePeerConstructionIsInert(t *testing.T) {
	t.Parallel()

	fake := newExecutePeerFake("cancel-attempt")
	if fake.providers == nil {
		t.Fatal("fake peer construction returned nil catalog map")
	}
	if len(fake.providers) != 0 {
		t.Fatalf("fake peer construction initialized catalog entries = %d, want 0", len(fake.providers))
	}

	var service providers.Service = fake
	if service == nil {
		t.Fatal("constructed Service is nil")
	}
}
