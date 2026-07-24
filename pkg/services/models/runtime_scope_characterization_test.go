package models_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// runtimeScopePeerService is a fake peer implementer of Models root Service
// that validates plain runtime-scope binding inputs and returns a usable bound
// Service view without starting host processes.
type runtimeScopePeerService struct {
	bound bool
}

func (s runtimeScopePeerService) ForRuntime(binding models.RuntimeBinding) (models.Service, error) {
	if err := models.ValidateRuntimeBinding(binding); err != nil {
		return nil, err
	}
	return runtimeScopePeerService{bound: true}, nil
}

func (s runtimeScopePeerService) ListModels(context.Context) (models.List, error) {
	if !s.bound {
		return models.List{}, models.ErrInvalidRuntimeBinding
	}
	return models.List{Results: []models.Summary{}}, nil
}

func (runtimeScopePeerService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrNotFound
}

func (runtimeScopePeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (runtimeScopePeerService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (runtimeScopePeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestRuntimeScope_ValidBindingReturnsUsableServiceViewWithoutHostProcesses(t *testing.T) {
	t.Parallel()

	var service models.Service = runtimeScopePeerService{}
	binding := models.RuntimeBinding{
		CacheDirectory: "cache",
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				FactoryDirectory: "factory",
				Workers: []models.RuntimeWorker{{
					Name:          "writer",
					Type:          models.RuntimeWorkerTypeInference,
					Model:         "local-model",
					ModelLocality: models.RuntimeModelLocalityLocal,
				}},
			}
		},
	}

	bound, err := service.ForRuntime(binding)
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
	}
	if bound == nil {
		t.Fatal("ForRuntime returned nil Service view")
	}

	list, err := bound.ListModels(context.Background())
	if err != nil {
		t.Fatalf("bound ListModels: %v", err)
	}
	if list.Results == nil {
		t.Fatal("bound ListModels Results = nil, want empty Models-owned slice")
	}
}

func TestRuntimeScope_MissingRequiredInputsFailWithTypedBindingError(t *testing.T) {
	t.Parallel()

	var service models.Service = runtimeScopePeerService{}

	t.Run("missing cache directory", func(t *testing.T) {
		t.Parallel()
		_, err := service.ForRuntime(models.RuntimeBinding{
			RuntimeConfig: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		})
		if err == nil {
			t.Fatal("ForRuntime error = nil, want ErrInvalidRuntimeBinding")
		}
		if !errors.Is(err, models.ErrInvalidRuntimeBinding) {
			t.Fatalf("ForRuntime error = %v, want ErrInvalidRuntimeBinding", err)
		}
		if !strings.Contains(err.Error(), "cache directory") {
			t.Fatalf("ForRuntime error = %v, want cache directory detail", err)
		}
	})

	t.Run("missing runtime config loader", func(t *testing.T) {
		t.Parallel()
		_, err := service.ForRuntime(models.RuntimeBinding{CacheDirectory: "cache"})
		if err == nil {
			t.Fatal("ForRuntime error = nil, want ErrInvalidRuntimeBinding")
		}
		if !errors.Is(err, models.ErrInvalidRuntimeBinding) {
			t.Fatalf("ForRuntime error = %v, want ErrInvalidRuntimeBinding", err)
		}
		if !strings.Contains(err.Error(), "runtime configuration") {
			t.Fatalf("ForRuntime error = %v, want runtime configuration detail", err)
		}
	})
}

func TestRuntimeScope_RemainsOnSingularServiceWithoutConstructionPorts(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// bind runtime scope without HostProcessLauncher or local-runtime ports.
	var service models.Service = runtimeScopePeerService{}
	binding := models.RuntimeBinding{
		CacheDirectory: "cache",
		RuntimeConfig:  func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
	}
	bound, err := service.ForRuntime(binding)
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
	}
	if _, ok := bound.(models.Service); !ok {
		t.Fatal("ForRuntime result does not satisfy singular root Service")
	}

	opening := models.RuntimeOpeningRequest{CacheDirectory: binding.CacheDirectory}
	if opening.CacheDirectory != binding.CacheDirectory {
		t.Fatalf("RuntimeOpeningRequest CacheDirectory = %q, want %q", opening.CacheDirectory, binding.CacheDirectory)
	}
}
