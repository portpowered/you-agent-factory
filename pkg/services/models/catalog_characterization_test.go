package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// catalogPeerService is a fake peer implementer of Models root Service that
// exercises plain catalog list/get contracts using only root-package types.
type catalogPeerService struct {
	unavailable bool
	entries     map[string]models.Detail
}

func (catalogPeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return catalogPeerService{}, nil
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

func (s catalogPeerService) GetModel(_ context.Context, name string) (models.Detail, error) {
	if err := models.ValidateGetModelRequest(models.GetModelRequest{Name: name}); err != nil {
		return models.Detail{}, err
	}
	if s.unavailable {
		return models.Detail{}, models.ErrUnavailable
	}
	detail, ok := s.entries[name]
	if !ok {
		return models.Detail{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
	}
	return detail, nil
}

func (catalogPeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (catalogPeerService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (catalogPeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestCatalog_ListAndGetReturnDetachedModelsOwnedShapes(t *testing.T) {
	t.Parallel()

	detail := models.Detail{
		Summary: models.Summary{
			Name:             "local-model",
			ProviderLocality: models.LocalityLocal,
			Status:           models.StatusReady,
			LoadState:        models.LoadStateUnloaded,
			ManagedRuntime: models.Runtime{
				Identity:       "local-model",
				ReadinessState: models.ReadinessStateReady,
				LifecycleState: models.LifecycleStateInstalled,
				Locality:       models.LocalityLocal,
			},
		},
		Capabilities: []models.Capability{{
			Worker:           "writer",
			ProviderLocality: models.LocalityLocal,
		}},
	}
	var service models.Service = catalogPeerService{
		entries: map[string]models.Detail{"local-model": detail},
	}

	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(list.Results) != 1 {
		t.Fatalf("ListModels Results len = %d, want 1", len(list.Results))
	}
	if list.Results[0].Name != "local-model" {
		t.Fatalf("ListModels Results[0].Name = %q, want local-model", list.Results[0].Name)
	}
	if list.Results[0].Status != models.StatusReady {
		t.Fatalf("ListModels Status = %s, want READY", list.Results[0].Status)
	}
	if list.Results[0].ManagedRuntime.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("ListModels readiness = %s, want READY", list.Results[0].ManagedRuntime.ReadinessState)
	}

	got, err := service.GetModel(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if got.Name != detail.Name || len(got.Capabilities) != 1 {
		t.Fatalf("GetModel detail = %#v, want detached Models-owned Detail", got)
	}
}

func TestCatalog_MissingUnsupportedAndUnavailableAreDistinctTypedOutcomes(t *testing.T) {
	t.Parallel()

	var service models.Service = catalogPeerService{entries: map[string]models.Detail{}}

	_, err := service.GetModel(context.Background(), "missing-model")
	if err == nil {
		t.Fatal("GetModel error = nil, want ErrNotFound")
	}
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel error = %v, want ErrNotFound", err)
	}
	if errors.Is(err, models.ErrUnavailable) || errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("GetModel error = %v, must stay distinct from unavailable/unsupported", err)
	}

	_, err = service.PullModel(context.Background(), "local-model")
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("PullModel error = %v, want ErrUnsupportedOperation", err)
	}
	if errors.Is(err, models.ErrNotFound) || errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("PullModel error = %v, must stay distinct from not-found/unavailable", err)
	}

	unavailable := catalogPeerService{unavailable: true}
	var unavailableService models.Service = unavailable
	_, err = unavailableService.ListModels(context.Background())
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListModels error = %v, want ErrUnavailable", err)
	}
	_, err = unavailableService.GetModel(context.Background(), "local-model")
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("GetModel unavailable error = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, models.ErrNotFound) || errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("GetModel unavailable error = %v, must stay distinct from not-found/unsupported", err)
	}
}

func TestCatalog_PeerCompilesWithoutInternalCatalogImports(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// consume list/get without models/internal/catalog or local assemblers.
	req := models.GetModelRequest{Name: "local-model"}
	if err := models.ValidateGetModelRequest(req); err != nil {
		t.Fatalf("ValidateGetModelRequest: %v", err)
	}
	if err := models.ValidateGetModelRequest(models.GetModelRequest{}); err == nil {
		t.Fatal("ValidateGetModelRequest empty name = nil, want error")
	}
	if !errors.Is(models.ValidateGetModelRequest(models.GetModelRequest{}), models.ErrNotFound) {
		t.Fatal("ValidateGetModelRequest empty name must wrap ErrNotFound")
	}

	_ = models.ListModelsRequest{}
	var service models.Service = catalogPeerService{}
	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if list.Results == nil {
		t.Fatal("ListModels Results = nil, want empty Models-owned slice")
	}
}
