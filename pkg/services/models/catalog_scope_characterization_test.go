package models_test

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (unsupportedRuntimeScopePeer) ListCatalog(
	context.Context,
	models.ListModelsRequest,
) (models.ListModelsResult, error) {
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) GetCatalogModel(
	context.Context,
	models.GetModelRequest,
) (models.GetModelResult, error) {
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}

// catalogPeerService is a fake peer implementer of Models root Service that
// exercises scoped catalog and readiness contracts using only root types.
type catalogPeerService struct {
	*runtimeScopePeerService
	unavailable bool
	entries     map[string]models.Detail
}

func (catalogPeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return catalogPeerService{runtimeScopePeerService: newRuntimeScopePeerService("legacy")}, nil
}

func (s catalogPeerService) ListModels(context.Context) (models.List, error) {
	if s.unavailable {
		return models.List{}, models.ErrUnavailable
	}
	results := make([]models.Summary, 0, len(s.entries))
	for _, detail := range s.entries {
		results = append(results, detail.Summary)
	}
	return models.List{Results: results}, nil
}

func (s catalogPeerService) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.ListModelsResult{}, err
	}
	list, err := s.ListModels(ctx)
	if err != nil {
		return models.ListModelsResult{}, err
	}
	result := models.ListModelsResult{Models: make([]models.Summary, len(list.Results))}
	for i := range list.Results {
		result.Models[i] = list.Results[i].Clone()
	}
	return result, nil
}

func (s catalogPeerService) GetCatalogModel(
	ctx context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.GetModelResult{}, err
	}
	detail, err := s.GetModel(ctx, request.Name)
	if err != nil {
		return models.GetModelResult{}, err
	}
	if request.Operation != "" && !detailSupportsOperation(detail, request.Operation) {
		return models.GetModelResult{}, models.ErrUnsupportedOperation
	}
	return models.GetModelResult{Model: detail.Clone()}, nil
}

func (s catalogPeerService) GetModelReadiness(
	ctx context.Context,
	request models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelReadinessResult{}, err
	}
	found, err := s.GetCatalogModel(ctx, models.GetModelRequest{
		Scope:     request.Scope,
		Name:      request.Name,
		Operation: request.Operation,
	})
	if err != nil {
		return models.GetModelReadinessResult{}, err
	}
	return models.GetModelReadinessResult{
		ModelName: found.Model.Name,
		Readiness: found.Model.ManagedRuntime,
	}, nil
}

func detailSupportsOperation(detail models.Detail, operation string) bool {
	for _, configured := range detail.Operations {
		if configured.Name == operation {
			return true
		}
	}
	for _, capability := range detail.Capabilities {
		for _, configured := range capability.Operations {
			if configured.Name == operation {
				return true
			}
		}
	}
	return false
}

func TestCatalogDiscovery_ScopedListGetAndReadinessReturnDetachedRootValues(t *testing.T) {
	t.Parallel()

	fake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-a"),
		entries: map[string]models.Detail{
			"local-model": {
				Summary: models.Summary{
					Name:             "local-model",
					ProviderLocality: models.LocalityLocal,
					Status:           models.StatusReady,
					Operations:       []models.Operation{{Name: "generate"}},
					ManagedRuntime: models.Runtime{
						Identity:            "local-model",
						ReadinessState:      models.ReadinessStateReady,
						LifecycleState:      models.LifecycleStateInstalled,
						SupportedOperations: []models.Operation{{Name: "generate"}},
					},
				},
				Capabilities: []models.Capability{{
					Worker:           "writer",
					ProviderLocality: models.LocalityLocal,
					Operations:       []models.Operation{{Name: "generate"}},
				}},
				Sources: []models.SourceMetadata{{
					Provider:  "hugging-face",
					Reference: "org/local-model",
					Revision:  "sha256:abc",
				}},
			},
		},
	}
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	list, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(list.Models) != 1 || list.Models[0].Name != "local-model" {
		t.Fatalf("ListCatalog = %#v, want detached local-model summary", list)
	}

	got, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	assertCatalogDetail(t, got.Model)

	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "local-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("GetModelReadiness: %v", err)
	}
	if readiness.ModelName != "local-model" || readiness.Readiness.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("GetModelReadiness = %#v, want ready local-model", readiness)
	}
}

func assertCatalogDetail(t *testing.T, got models.Detail) {
	t.Helper()
	if len(got.Sources) != 1 || got.Sources[0].Reference != "org/local-model" {
		t.Fatalf("GetCatalogModel = %#v, want detached source metadata", got)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Worker != "writer" {
		t.Fatalf("GetCatalogModel = %#v, want configured binding facts", got)
	}
}

func TestCatalogDiscovery_ResultsAreDetachedFromPeerState(t *testing.T) {
	t.Parallel()

	fake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-detached"),
		entries: map[string]models.Detail{
			"local-model": {
				Summary:      models.Summary{Name: "local-model", Operations: []models.Operation{{Name: "generate"}}},
				Capabilities: []models.Capability{{Worker: "writer"}},
				Sources:      []models.SourceMetadata{{Reference: "org/local-model"}},
			},
		},
	}
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	got, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	got.Model.Sources[0].Reference = "mutated"
	got.Model.Capabilities[0].Worker = "mutated"
	got.Model.Operations[0].Name = "mutated"

	again, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel after mutation: %v", err)
	}
	if again.Model.Sources[0].Reference != "org/local-model" ||
		again.Model.Capabilities[0].Worker != "writer" ||
		again.Model.Operations[0].Name != "generate" {
		t.Fatalf("GetCatalogModel retained caller mutation: %#v", again)
	}
}

func TestCatalogDiscovery_NormalizedFailuresStayDistinct(t *testing.T) {
	t.Parallel()

	fake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-a"),
		entries: map[string]models.Detail{
			"local-model": {Summary: models.Summary{Name: "local-model"}},
		},
	}
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	stale, _ := (models.RuntimeScopeRef{}).Parse("factory-session-a:stale")
	foreign, _ := (models.RuntimeScopeRef{}).Parse("factory-session-b:1")

	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: models.RuntimeScopeRef{}, Name: "local-model"}, models.ErrRuntimeScopeInvalid)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: stale, Name: "local-model"}, models.ErrRuntimeScopeStale)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: foreign, Name: "local-model"}, models.ErrRuntimeScopeForeign)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: opened.Scope, Name: "missing"}, models.ErrNotFound)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model", Operation: "unsupported",
	}, models.ErrUnsupportedOperation)

	if _, err := service.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{Scope: opened.Scope}); err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: opened.Scope, Name: "local-model"}, models.ErrRuntimeScopeClosed)

	unavailableFake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-c"),
		unavailable:             true,
	}
	var unavailable models.Service = unavailableFake
	unavailableScope, err := unavailable.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope unavailable fake: %v", err)
	}
	_, err = unavailable.ListCatalog(context.Background(), models.ListModelsRequest{Scope: unavailableScope.Scope})
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListCatalog unavailable = %v, want ErrUnavailable", err)
	}
}

func assertCatalogGetErrorIs(
	t *testing.T,
	service models.Service,
	request models.GetModelRequest,
	want error,
) {
	t.Helper()
	_, err := service.GetCatalogModel(context.Background(), request)
	if !errors.Is(err, want) {
		t.Fatalf("GetCatalogModel(%+v) = %v, want %v", request, err, want)
	}
}
