package service_test

import (
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestOpenReturnsOpaqueReferenceThatResolvesDetachedBinding(t *testing.T) {
	t.Parallel()

	config := &models.RuntimeConfig{
		FactoryDirectory: "factory",
		BaseDirectory:    "base",
		Workers: []models.RuntimeWorker{{
			Name:      "speaker",
			Args:      []string{"--voice", "original"},
			Resources: []models.RuntimeResource{{ID: "worker-resource", Capacity: 1}},
			Operations: []models.RuntimeOperation{{
				Name: "speak",
				Inputs: []models.RuntimeOperationSlot{{
					Name: "text", ContentTypes: []string{models.RuntimeContentTypeText},
				}},
			}},
		}},
		Resources: []models.RuntimeResource{{ID: "model-resource", Capacity: 2}},
	}
	service := runtimescopeswire.NewService()
	ref, err := service.Open(models.RuntimeBinding{
		CacheDirectory: "cache",
		RuntimeConfig:  func() *models.RuntimeConfig { return config },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ref == "" {
		t.Fatal("Open reference is empty")
	}

	config.FactoryDirectory = "mutated"
	config.Workers[0].Args[1] = "mutated"
	config.Workers[0].Resources[0].Capacity = 99
	config.Workers[0].Operations[0].Inputs[0].ContentTypes[0] = models.RuntimeContentTypeAudio
	config.Resources[0].Capacity = 99

	resolved, err := service.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertOriginalBinding(t, resolved)

	first := resolved.RuntimeConfig()
	first.Workers[0].Args[1] = "changed-after-resolve"
	first.Resources[0].Capacity = 100
	resolvedAgain, err := service.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve again: %v", err)
	}
	assertOriginalBinding(t, resolvedAgain)
}

func TestOpenRejectsInvalidBindingWithoutIssuingReference(t *testing.T) {
	t.Parallel()

	service := runtimescopeswire.NewService()
	ref, err := service.Open(models.RuntimeBinding{CacheDirectory: "cache"})
	if !errors.Is(err, models.ErrInvalidRuntimeBinding) {
		t.Fatalf("Open error = %v, want ErrInvalidRuntimeBinding", err)
	}
	if ref != "" {
		t.Fatalf("Open reference = %q, want empty", ref)
	}

	if _, err := service.Resolve(ref); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
		t.Fatalf("Resolve invalid-open reference error = %v, want ErrScopeUnknown", err)
	}
}

func TestConstructionAndOpenOnlySnapshotTheAcceptedBinding(t *testing.T) {
	t.Parallel()

	loaderCalls := 0
	service := runtimescopeswire.NewService()
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	if loaderCalls != 0 {
		t.Fatalf("construction called runtime loader %d times", loaderCalls)
	}

	ref, err := service.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			loaderCalls++
			return &models.RuntimeConfig{FactoryDirectory: "factory"}
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if loaderCalls != 1 {
		t.Fatalf("Open called runtime loader %d times, want one snapshot", loaderCalls)
	}
	if _, err := service.Resolve(ref); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if loaderCalls != 1 {
		t.Fatalf("Resolve called caller runtime loader; calls = %d", loaderCalls)
	}
}

func TestCloseInvalidatesOnlyTheSelectedScope(t *testing.T) {
	t.Parallel()

	service := runtimescopeswire.NewService()
	firstRef := openBinding(t, service, "first")
	secondRef := openBinding(t, service, "second")

	if err := service.Close(firstRef); err != nil {
		t.Fatalf("Close first scope: %v", err)
	}
	if _, err := service.Resolve(firstRef); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
		t.Fatalf("Resolve closed scope error = %v, want ErrScopeUnknown", err)
	}
	if err := service.Close(firstRef); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
		t.Fatalf("Close closed scope error = %v, want ErrScopeUnknown", err)
	}

	resolved, err := service.Resolve(secondRef)
	if err != nil {
		t.Fatalf("Resolve second scope: %v", err)
	}
	if got := resolved.RuntimeConfig().FactoryDirectory; got != "second" {
		t.Fatalf("second scope FactoryDirectory = %q, want second", got)
	}
}

func TestCloseRejectsMalformedAndUnknownReferencesWithoutChangingLiveScopes(t *testing.T) {
	t.Parallel()

	service := runtimescopeswire.NewService()
	liveRef := openBinding(t, service, "live")

	for _, ref := range []runtimescopes.Reference{
		"",
		"malformed",
		"00000000000000000000000000000000.00000000000000000000000000000000",
	} {
		if _, err := service.Resolve(ref); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
			t.Errorf("Resolve(%q) error = %v, want ErrScopeUnknown", ref, err)
		}
		if err := service.Close(ref); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
			t.Errorf("Close(%q) error = %v, want ErrScopeUnknown", ref, err)
		}
	}

	resolved, err := service.Resolve(liveRef)
	if err != nil {
		t.Fatalf("Resolve live scope: %v", err)
	}
	if got := resolved.RuntimeConfig().FactoryDirectory; got != "live" {
		t.Fatalf("live scope FactoryDirectory = %q, want live", got)
	}
}

func openBinding(t *testing.T, service runtimescopes.Service, factoryDirectory string) runtimescopes.Reference {
	t.Helper()
	ref, err := service.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{FactoryDirectory: factoryDirectory}
		},
	})
	if err != nil {
		t.Fatalf("Open %q scope: %v", factoryDirectory, err)
	}
	return ref
}

func assertOriginalBinding(t *testing.T, binding models.RuntimeBinding) {
	t.Helper()
	if binding.CacheDirectory != "cache" {
		t.Fatalf("CacheDirectory = %q, want cache", binding.CacheDirectory)
	}
	config := binding.RuntimeConfig()
	if config.FactoryDirectory != "factory" ||
		config.Workers[0].Args[1] != "original" ||
		config.Workers[0].Resources[0].Capacity != 1 ||
		config.Workers[0].Operations[0].Inputs[0].ContentTypes[0] != models.RuntimeContentTypeText ||
		config.Resources[0].Capacity != 2 {
		t.Fatalf("resolved binding was not detached: %#v", config)
	}
}
