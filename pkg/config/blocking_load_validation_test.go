package config

import (
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestBlockingLoadValidation_EquivalentCanonicalTargetsForCrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	cfg, err := FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	configFindings := CanonicalStructuralFindings(&cfg)
	blockingLoad := factoryvalidation.ValidateBlockingLoad(&cfg)

	if len(blockingLoad.Targets) == 0 {
		t.Fatal("expected blocking load validation targets for cross-path invalid fixture")
	}
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, explicit targets = %d, want equivalent coverage",
			len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("finding[%d].Rule = %q, want explicit target code %q",
				index, configFindings[index].Rule, target.Code)
		}
	}

	packageSignatures := factoryvalidation.CanonicalTargetSignatures(blockingLoad.Targets)
	for _, code := range []string{
		factoryvalidation.CodeDuplicateIdentifier,
		factoryvalidation.CodeDanglingWorkerReference,
		factoryvalidation.CodeDanglingPlaceReference,
	} {
		assertBlockingLoadHasCode(t, blockingLoad.Targets, code)
	}
	if len(packageSignatures) == 0 {
		t.Fatal("expected non-empty blocking load signatures")
	}
}

func assertBlockingLoadHasCode(t *testing.T, targets []factoryvalidation.Target, code string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("blocking load targets = %#v, want code %q", targets, code)
}
