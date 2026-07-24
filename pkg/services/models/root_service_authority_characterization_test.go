package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// peerModelsService is a fake peer implementer of the singular Models root
// Service. It compiles against only the published root package and does not
// import models/internal.
type peerModelsService struct{}

func (peerModelsService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return peerModelsService{}, nil
}

func (peerModelsService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: nil}, nil
}

func (peerModelsService) GetModel(_ context.Context, name string) (models.Detail, error) {
	return models.Detail{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
}

func (peerModelsService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (peerModelsService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (peerModelsService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestRootServiceAuthority_FakePeerGetModelNotFound(t *testing.T) {
	t.Parallel()

	var service models.Service = peerModelsService{}
	_, err := service.GetModel(context.Background(), "missing-model")
	if err == nil {
		t.Fatal("GetModel error = nil, want ErrNotFound")
	}
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel error = %v, want ErrNotFound", err)
	}
}

func TestRootServiceAuthority_AggregateSurfaceRemainsOnSingularService(t *testing.T) {
	t.Parallel()

	var service models.Service = peerModelsService{}

	bound, err := service.ForRuntime(models.RuntimeBinding{CacheDirectory: "cache"})
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
	}
	if bound == nil {
		t.Fatal("ForRuntime returned nil Service view")
	}

	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if list.Results == nil {
		// Empty catalog is valid; Results must remain a usable Models-owned List.
		list.Results = []models.Summary{}
	}

	if _, err := service.PullModel(context.Background(), "model"); !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("PullModel error = %v, want ErrUnsupportedOperation", err)
	}
	if _, err := service.InspectRuntime(context.Background(), "model"); !errors.Is(err, models.ErrUnsupported) {
		t.Fatalf("InspectRuntime error = %v, want ErrUnsupported", err)
	}

	result, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{})
	if err != nil {
		t.Fatalf("InvokeLocal: %v", err)
	}
	if result.Handled {
		t.Fatal("InvokeLocal Handled = true, want false for unsupported peer path")
	}
}
