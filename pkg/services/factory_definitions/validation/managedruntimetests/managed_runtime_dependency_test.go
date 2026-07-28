package validation_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestManagedRuntimeDependencyTargets_RejectsUnsupportedIdentity(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "unknown-cache",
			Type:       factoryresource.TypeModel,
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

	cfg := &factorydefinitions.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
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

	cfg := &factorydefinitions.FactoryConfig{
		Workers: []workerconfig.Config{{
			Name:          "voice-local",
			Type:          factorydefinitions.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: workerconfig.ModelLocalityLocal,
		}},
	}

	targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg)
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeManagedRuntimeWorkerMissingDep)
}

func TestManagedRuntimeDependencyTargets_AcceptsValidAuthoredFactory(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []workerconfig.Config{{
			Name:          "voice-local",
			Type:          factorydefinitions.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: workerconfig.ModelLocalityLocal,
			Resources:     []factoryresource.Config{{Name: "omnivoice-cache", Capacity: 1}},
		}},
	}

	if targets := factoryvalidation.ManagedRuntimeDependencyTargets(cfg); len(targets) != 0 {
		t.Fatalf("targets = %#v, want none", targets)
	}
}

func TestValidate_IncludesManagedRuntimeDependencyTargets(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeManagedRuntimeInvalidBackend)
}
