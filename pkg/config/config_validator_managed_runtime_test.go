package config

import (
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestCanonicalStructuralFindings_RejectsUnsupportedManagedRuntimeIdentity(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:       "unknown-cache",
		Type:       interfaces.ResourceTypeModel,
		Capacity:   1,
		Model:      "UNKNOWN_RUNTIME",
		Backend:    "LLAMACPP",
		LoadPolicy: "ON_DEMAND",
	}}

	findings := CanonicalStructuralFindings(cfg)
	assertFindingExists(t, findings, factoryvalidation.CodeManagedRuntimeUnsupportedIdentity)
}

func TestCanonicalStructuralFindings_RejectsLocalWorkerWithoutModelResource(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:          "voice-local",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
	}}

	findings := CanonicalStructuralFindings(cfg)
	assertFindingExists(t, findings, factoryvalidation.CodeManagedRuntimeWorkerMissingDep)
}
