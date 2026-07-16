package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestService_ListModels_SummarizesConfiguredModelCapabilities(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
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
	if model.Name != "OMNIVOICE_Q4_K_M" || model.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("model summary = %#v, want OMNIVOICE local model", model)
	}
	if model.Status != factoryapi.ModelStatusREADY || model.LoadState != factoryapi.UNLOADED {
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

	svc, err := modelsservice.NewService(modelsservice.Dependencies{})
	if svc != nil || !errors.Is(err, modelsservice.ErrInvalidDependencies) {
		t.Fatalf("NewService = (%v, %v), want missing runtime construction error", svc, err)
	}
}

func TestService_ListModels_ReturnsInspectFailureFromModelHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
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
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     missingCacheInspectHost{},
	})

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	if models.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING {
		t.Fatalf("managed readiness = %s, want MISSING", models.Results[0].ManagedRuntime.ReadinessState)
	}
}

type failingInspectHost struct{}

func (failingInspectHost) ResolveIdentity(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, errors.New("inspect failed")
}

func (failingInspectHost) InspectReadiness(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, errors.New("inspect failed")
}

func (failingInspectHost) Pull(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("inspect failed")
}

func (failingInspectHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, errors.New("inspect failed")
}

func (failingInspectHost) ReleaseLease(context.Context, string) error {
	return errors.New("inspect failed")
}

func (failingInspectHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return errors.New("inspect failed")
}

func mustLoadedCatalogConfig(t *testing.T, factoryCfg *interfaces.FactoryConfig) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func catalogFactoryConfig(includeResource bool) *interfaces.FactoryConfig {
	worker := interfaces.WorkerConfig{
		Name:          "voice-local",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}
	cfg := &interfaces.FactoryConfig{
		Name:    "factory",
		Workers: []interfaces.WorkerConfig{worker},
	}
	if includeResource {
		worker.Resources = []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}}
		cfg.Resources = []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}}
	}
	return cfg
}
