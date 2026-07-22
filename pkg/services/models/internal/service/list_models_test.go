package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/catalog"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
)

func TestService_ListModels_SummarizesConfiguredModelCapabilities(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     installedCacheInspectHost{},
	})

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	model := models.Results[0]
	if model.Name != "OMNIVOICE_Q4_K_M" || model.ProviderLocality != managedruntime.LocalityLocal {
		t.Fatalf("model summary = %#v, want OMNIVOICE local model", model)
	}
	if model.Status != modelcatalog.StatusReady || model.LoadState != modelcatalog.LoadStateUnloaded {
		t.Fatalf("model readiness = (%s, %s), want (READY, UNLOADED)", model.Status, model.LoadState)
	}
	if len(model.Operations) != 1 || model.Operations[0].Name != "TTS" {
		t.Fatalf("operations = %#v, want one TTS operation", model.Operations)
	}
	if len(model.Modalities) != 2 {
		t.Fatalf("modalities = %#v, want TEXT and AUDIO", model.Modalities)
	}
	if len(model.Resources) != 1 || model.Resources[0].Name != "omnivoice-cache" {
		t.Fatalf("resources = %#v, want omnivoice-cache summary", model.Resources)
	}
}

func TestService_ListModels_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc, err := modelsservice.NewService(nil, nil, nil, nil, nil, nil)
	if svc != nil || !errors.Is(err, modelsservice.ErrInvalidDependencies) {
		t.Fatalf("NewService = (%v, %v), want missing runtime construction error", svc, err)
	}
}

func TestService_ListModels_ReturnsInspectFailureFromModelHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     failingInspectHost{},
	})

	_, err := svc.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels: nil error, want model host inspect failure")
	}
	if !strings.Contains(err.Error(), "inspect failed") {
		t.Fatalf("ListModels error = %v, want inspect failure", err)
	}
}

func TestService_ListModels_ProjectsManagedRuntimeFromModelHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     missingCacheInspectHost{},
	})

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	if models.Results[0].ManagedRuntime.ReadinessState != managedruntime.ReadinessStateMissing {
		t.Fatalf("managed readiness = %s, want MISSING", models.Results[0].ManagedRuntime.ReadinessState)
	}
}

type failingInspectHost struct{}

func (failingInspectHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, errors.New("inspect failed")
}

func (failingInspectHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, errors.New("inspect failed")
}

func (failingInspectHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("inspect failed")
}

func (failingInspectHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, errors.New("inspect failed")
}

func (failingInspectHost) ReleaseLease(context.Context, string) error {
	return errors.New("inspect failed")
}

func (failingInspectHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return errors.New("inspect failed")
}

func mustLoadedCatalogConfig(t *testing.T, factoryCfg *testFactoryConfig) *modelRuntimeConfig {
	t.Helper()
	return projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
}

func catalogFactoryConfig(includeResource bool) *testFactoryConfig {
	worker := modelRuntimeWorker{
		Name:          "voice-local",
		Type:          models.RuntimeWorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: models.RuntimeModelLocalityLocal,
		Operations: []models.RuntimeOperation{{
			Name: "TTS",
			Inputs: []models.RuntimeOperationSlot{{
				Name:         "text",
				ContentTypes: []string{models.RuntimeContentTypeText},
				Required:     true,
			}},
			Outputs: []models.RuntimeOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{models.RuntimeContentTypeAudio},
			}},
		}},
	}
	cfg := &testFactoryConfig{
		Name:    "factory",
		Workers: []modelRuntimeWorker{worker},
	}
	if includeResource {
		worker.Resources = []modelRuntimeResource{{Name: "omnivoice-cache", Capacity: 1}}
		cfg.Resources = []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       models.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}}
	}
	return cfg
}
