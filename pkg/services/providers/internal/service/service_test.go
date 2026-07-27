package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestNew_RejectsNilCatalog(t *testing.T) {
	t.Parallel()

	service, err := providerservice.New(nil, &stubExecution{})
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
	service, constructionErr := providerservice.New(catalogService, nilExecution)
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
	root, err := providerservice.New(catalogService, executionService)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 8 {
		t.Fatalf("len(Providers) = %d, want 8", len(list.Providers))
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q, want codex", got.Provider.ID)
	}

	byAlias, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.ID("cursor")})
	if err != nil {
		t.Fatalf("GetProvider(cursor) = %v", err)
	}
	if byAlias.Provider.ID != providers.IDCursor {
		t.Fatalf("GetProvider(cursor).Provider.ID = %q, want agent", byAlias.Provider.ID)
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
			Provider: providers.IDCursor,
			Attempt: func(
				_ context.Context,
				request providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				calls++
				if request.Provider != providers.IDCursor {
					t.Fatalf("adapter provider = %q, want %q", request.Provider, providers.IDCursor)
				}
				return providers.ExecuteResult{Content: "root result"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	result, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.ID("cursor"),
		AttemptID: "attempt-1",
	})
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "root result" || calls != 1 {
		t.Fatalf("Execute() = (%#v, %d calls), want root result and 1 call", result, calls)
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
	root, err := providerservice.New(catalogService, executionService)
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
	agy, ok := indexProviders(list.Providers)[providers.IDAgy]
	if !ok {
		t.Fatal("ListProviders() missing agy provider")
	}
	if agy.Availability != providers.AvailabilitySelectable {
		t.Fatalf("agy availability = %q, want selectable", agy.Availability)
	}
}

func TestRootCatalogProbeFailureMatchesPrivateCatalog(t *testing.T) {
	t.Parallel()

	root, err := providerswire.NewService(catalogwire.WithProbeQuery(func(
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
	}))
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

func TestRootExecuteUsesBoundExecutionWithoutARegisteredAdapter(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)

	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-1",
		UserMessage: "hello",
	})
	if !errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("Execute() error = %v, want ErrProviderUnavailable", err)
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
	root, err := providerswire.NewService(catalogwire.WithProbeQuery(func(
		context.Context,
		providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{}, nil
	}))
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
	root, err := providerservice.New(catalogService, executionService)
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
