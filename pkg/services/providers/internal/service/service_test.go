package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestNew_RejectsNilCatalog(t *testing.T) {
	t.Parallel()

	service, err := providerservice.New(nil, &stubExecution{}, logging.NoopLogger{})
	if err == nil || service != nil {
		t.Fatalf("New(nil) = (%v, %v), want error", service, err)
	}
}

func TestNewRejectsInvalidExecutionComposition(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	var nilExecution execution.Service
	service, constructionErr := providerservice.New(catalogService, nilExecution, logging.NoopLogger{})
	if constructionErr == nil || service != nil {
		t.Fatalf(
			"New() = (%v, %v), want invalid execution composition error",
			service,
			constructionErr,
		)
	}
}

func TestRootDelegatesListAndGetToCatalog(t *testing.T) {
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

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 23 {
		t.Fatalf("len(Providers) = %d, want 23", len(list.Providers))
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q, want codex", got.Provider.ID)
	}

	_, err = root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.ID("cursor")})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("GetProvider(cursor) error = %v, want ErrUnknownProvider", err)
	}
}

func TestRootDelegatesExecuteToOnePrivateExecutionAttempt(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	calls := 0
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				request providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				calls++
				if request.Provider != providers.IDCodex {
					t.Fatalf("adapter provider = %q, want %q", request.Provider, providers.IDCodex)
				}
				if request.ReasoningEffort != "xhigh" {
					t.Fatalf("adapter reasoning effort = %q, want canonical xhigh", request.ReasoningEffort)
				}
				if !request.SkipPermissions {
					t.Fatal("adapter skip permissions = false, want true")
				}
				return providers.ExecuteResult{Content: "root result"}, nil
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

	result, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "attempt-1",
		ReasoningEffort: " XHIGH ",
		SkipPermissions: true,
	})
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "root result" || calls != 1 {
		t.Fatalf("Execute() = (%#v, %d calls), want root result and 1 call", result, calls)
	}
}

func TestRootFailsClosedForUnsupportedPermissionBypassBeforeAttempt(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService(catalogwire.WithDescriptors(providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}))
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	adapterCalls := 0
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{Content: "unexpected"}, nil
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

	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "unsupported-bypass",
		SkipPermissions: true,
	})
	var failure providers.ExecuteFailure
	if !errors.Is(executeErr, providers.ErrCapabilityMismatch) ||
		!errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindCapabilityMismatch ||
		!strings.Contains(failure.Message, "codex") ||
		!strings.Contains(failure.Message, "permission_bypass") ||
		strings.Contains(failure.Message, "command") ||
		adapterCalls != 0 {
		t.Fatalf(
			"Execute(unsupported bypass) = (%#v, %d calls), want bounded capability failure before attempt",
			executeErr,
			adapterCalls,
		)
	}

	for _, request := range []providers.ExecuteRequest{
		{Provider: providers.IDCodex, AttemptID: "omitted-bypass"},
		{Provider: providers.IDCodex, AttemptID: "false-bypass", SkipPermissions: false},
	} {
		result, executeErr := root.Execute(context.Background(), request)
		if executeErr != nil || result.Content != "unexpected" {
			t.Fatalf("Execute(%q) = (%#v, %v), want default route execution", request.AttemptID, result, executeErr)
		}
	}
	if adapterCalls != 2 {
		t.Fatalf("adapter calls after default bypass requests = %d, want 2", adapterCalls)
	}
}

func TestRootACPRejectsSeparateReasoningEffortAndAcceptsExactModelID(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	acpService := &stubACPService{provider: "cursor-acp"}
	root, err := providerservice.NewWithACP(catalogService, executionService, acpService, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}

	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        "cursor-acp",
		AttemptID:       "attempt-acp-effort",
		Model:           "cursor-grok-4.5-medium-fast",
		ReasoningEffort: "xhigh",
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindInvalidRequest ||
		!strings.Contains(failure.Message, "exact advertised model id") ||
		acpService.executeCalls != 0 {
		t.Fatalf("Execute(ACP with effort) = (%#v, %d calls), want invalid request before ACP execution", executeErr, acpService.executeCalls)
	}

	result, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  "cursor-acp",
		AttemptID: "attempt-acp-model",
		Model:     "cursor-grok-4.5-medium-fast",
	})
	if executeErr != nil || result.Content != "acp result" || acpService.executeCalls != 1 {
		t.Fatalf("Execute(ACP exact model) = (%#v, %v, %d calls), want delegated success", result, executeErr, acpService.executeCalls)
	}
}

func TestRootRejectsSeparateReasoningEffortForAgy(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)
	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.ID(" ANTIGRAVITY "),
		AttemptID:       "attempt-agy-effort",
		ReasoningEffort: "xhigh",
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindInvalidRequest ||
		!strings.Contains(failure.Message, "does not support a separate reasoning effort") {
		t.Fatalf("Execute(Agy with effort) error = %#v, want invalid request", executeErr)
	}
}

func TestCatalogAdvertisedAgyEffortsAreNotRejectedByExecutionPolicy(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDAntigravity,
			Attempt: func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{Content: "accepted"}, nil
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

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	agy := indexProviders(list.Providers)[providers.IDAntigravity]
	for _, model := range agy.Models {
		for _, effort := range model.Efforts {
			_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
				Provider:        providers.IDAntigravity,
				AttemptID:       "catalog-policy-" + model.ID,
				Model:           model.ID,
				ReasoningEffort: string(effort),
			})
			var failure providers.ExecuteFailure
			if errors.As(executeErr, &failure) &&
				failure.Kind == providers.ExecuteFailureKindInvalidRequest &&
				strings.Contains(failure.Message, "does not support a separate reasoning effort") {
				t.Fatalf("catalog advertises AGY model %q effort %q rejected by canonical execution policy", model.ID, effort)
			}
			if executeErr != nil {
				t.Fatalf("Execute(%s, %s) = %v, want accepted advertised combination", model.ID, effort, executeErr)
			}
		}
	}
}

func TestRootRejectsMinimalReasoningEffortForClaude(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)
	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.ID(" CLAUDE "),
		AttemptID:       "claude-minimal",
		ReasoningEffort: " MINIMAL ",
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %T(%v), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("failure.Kind = %q, want invalid_request", failure.Kind)
	}
}

var _ acp.Service = (*stubACPService)(nil)

type stubACPService struct {
	provider        providers.ID
	executeCalls    int
	skipPermissions bool
}

func (service *stubACPService) Close(context.Context) error { return nil }

func (service *stubACPService) Configure(context.Context, []providers.ACPIntegration) error {
	return nil
}

func (service *stubACPService) Integrations() []providers.ACPIntegration {
	return []providers.ACPIntegration{{Name: service.provider}}
}

func (service *stubACPService) Resolve(id providers.ID) (providers.ID, bool) {
	return service.provider, id == service.provider
}

func (service *stubACPService) NegotiatedCapabilities(providers.ID) (acpsdk.AgentCapabilities, bool) {
	return acpsdk.AgentCapabilities{}, false
}

func (service *stubACPService) Execute(
	_ context.Context,
	_ providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	service.executeCalls++
	service.skipPermissions = request.SkipPermissions
	return providers.ExecuteResult{Content: "acp result"}, nil
}

func (service *stubACPService) Claim(providers.ID, string) (acp.Generation, bool) { return nil, false }

func (service *stubACPService) TryCancel(context.Context, acp.Generation) (bool, error) {
	return false, nil
}

func TestRootACPUsesAdvertisedPermissionBypass(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService(catalogwire.WithDescriptors(providers.Descriptor{
		ID:           "cursor-acp",
		DisplayName:  "Cursor ACP",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessUnverified,
		Capabilities: []providers.Capability{
			providers.CapabilityPromptSubmission,
			providers.CapabilityPermissionBypass,
		},
	}))
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	acpService := &stubACPService{provider: "cursor-acp"}
	root, err := providerservice.NewWithACP(catalogService, executionService, acpService, nil, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewWithACP() = %v", err)
	}

	result, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        acpService.provider,
		AttemptID:       "acp-bypass",
		Model:           "cursor-grok-4.5-medium-fast",
		SkipPermissions: true,
	})
	if executeErr != nil || result.Content != "acp result" || !acpService.skipPermissions || acpService.executeCalls != 1 {
		t.Fatalf(
			"Execute(ACP bypass) = (%#v, %v, skip=%v, calls=%d), want delegated bypass request",
			result,
			executeErr,
			acpService.skipPermissions,
			acpService.executeCalls,
		)
	}
}

func TestRootDelegatesTypedExecutionFailure(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind: providers.ExecuteFailureKindAuthentication,
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

	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-auth",
	})
	var failure providers.ExecuteFailure
	if !errors.Is(executeErr, providers.ErrExecuteFailed) ||
		!errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindAuthentication {
		t.Fatalf("Execute() error = %#v, want authentication ExecuteFailure", executeErr)
	}
}

func TestRootCatalogTypedFailuresMatchPrivateCatalog(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)

	assertGetErrorIs(t, root, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, root, providers.GetProviderRequest{ID: providers.IDClaude + "-stale"}, providers.ErrUnknownProvider)

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	agy, ok := indexProviders(list.Providers)[providers.IDAntigravity]
	if !ok {
		t.Fatal("ListProviders() missing agy provider")
	}
	if agy.Availability != providers.AvailabilitySelectable {
		t.Fatalf("agy availability = %q, want selectable", agy.Availability)
	}
}

func TestRootCatalogProbeFailureMatchesPrivateCatalog(t *testing.T) {
	t.Parallel()

	root, err := providerswire.NewService(providerswire.CatalogOption(catalogwire.WithProbeQuery(func(
		_ context.Context,
		descriptor providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		if descriptor.ID == providers.IDCodex {
			return catalog.ProbeFacts{}, errors.New("native probe stderr: /Users/customer/.codex/output")
		}
		return catalog.ProbeFacts{
			Readiness:     descriptor.Readiness,
			Prerequisites: descriptor.Prerequisites,
		}, nil
	})))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	codex := indexProviders(list.Providers)[providers.IDCodex]
	if codex.Readiness != providers.ReadinessUnavailable {
		t.Fatalf("codex readiness = %q, want unavailable after probe failure", codex.Readiness)
	}
	if strings.Contains(codex.Prerequisites[0].Description, "/Users/") {
		t.Fatalf("probe failure description leaked native output: %q", codex.Prerequisites[0].Description)
	}

	assertGetErrorIs(t, root, providers.GetProviderRequest{ID: providers.IDCodex}, providers.ErrProviderUnavailable)
}

func TestRootExecuteUsesRegisteredCodexAdapter(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)

	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-1",
		UserMessage: "hello",
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency ||
		!strings.Contains(failure.Message, "Codex") {
		t.Fatalf("Execute() error = %#v, want Codex adapter dependency failure", err)
	}

	_, err = root.Execute(context.Background(), providers.ExecuteRequest{
		Provider: providers.IDCodex,
	})
	if !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("invalid Execute() error = %v, want ErrExecuteFailed", err)
	}
}

func TestRootConstructionIsInert(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	root, err := providerswire.NewService(providerswire.CatalogOption(catalogwire.WithProbeQuery(func(
		context.Context,
		providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{}, nil
	})))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if root == nil {
		t.Fatal("NewService() returned nil")
	}
	var _ providers.Service = root
	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want 0", probeCalls)
	}
}

func TestRegisteredCompositionIsInert(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	adapterCalls := 0
	catalogService, err := catalogwire.NewService(catalogwire.WithProbeQuery(func(
		context.Context,
		providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{}, nil
	}))
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
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
	if root == nil || probeCalls != 0 || adapterCalls != 0 {
		t.Fatalf(
			"registered construction = (%v, %d probes, %d attempts), want inert root",
			root,
			probeCalls,
			adapterCalls,
		)
	}
}

func mustRootService(t *testing.T) providers.Service {
	t.Helper()

	root, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	return root
}

func assertGetErrorIs(
	t *testing.T,
	service providers.Service,
	request providers.GetProviderRequest,
	want error,
) {
	t.Helper()

	_, err := service.GetProvider(context.Background(), request)
	if !errors.Is(err, want) {
		t.Fatalf("GetProvider(%#v) error = %v, want %v", request, err, want)
	}
}

func indexProviders(descriptors []providers.Descriptor) map[providers.ID]providers.Descriptor {
	byID := make(map[providers.ID]providers.Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
	}
	return byID
}

type stubExecution struct{}

func (*stubExecution) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}
