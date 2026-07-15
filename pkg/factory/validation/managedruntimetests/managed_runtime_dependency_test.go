package validation_test

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestManagedRuntimeDependencyTargets_RejectsUnsupportedIdentity(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "unknown-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "UNKNOWN_RUNTIME",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	}

	targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg)
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeManagedRuntimeUnsupportedIdentity)
}

func TestManagedRuntimeDependencyTargets_RejectsInvalidBackend(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}},
	}

	targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg)
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeManagedRuntimeInvalidBackend)
}

func TestManagedRuntimeDependencyTargets_RejectsLocalWorkerWithoutDependency(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Workers: []interfaces.WorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
		}},
	}

	targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg)
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeManagedRuntimeWorkerMissingDep)
}

func TestManagedRuntimeDependencyTargets_AcceptsValidAuthoredFactory(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []interfaces.WorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Resources:     []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}},
		}},
	}

	if targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg); len(targets) != 0 {
		t.Fatalf("targets = %#v, want none", targets)
	}
}

func TestValidate_IncludesManagedRuntimeDependencyTargets(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeManagedRuntimeInvalidBackend)
}
