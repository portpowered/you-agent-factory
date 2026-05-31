package validationentry_test

import (
	"context"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
)

// TestCrossPathInvalidFixture_ProfileTopologyDiffersFromPrePersistDocumentsValidateEndpointGap
// guards the intentional product gap: POST /factory-validations uses ProfileTopology
// (structural validation including deferred outcome-route findings), while editable save
// uses ProfilePrePersist (canonical load + blocking-load subset without those routes).
// Operators who validate-only then save should expect extra outcome-route targets from
// validate-only unless the endpoint profile is upgraded to ProfilePrePersist.
func TestCrossPathInvalidFixture_ProfileTopologyDiffersFromPrePersistDocumentsValidateEndpointGap(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	topologyResult, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI topology: %v", err)
	}
	prePersistResult, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI pre-persist: %v", err)
	}

	if !topologyResult.HasTargets() || !prePersistResult.HasTargets() {
		t.Fatal("expected both profiles to reject cross-path invalid fixture")
	}

	topologySignatures := factoryvalidation.CanonicalTargetSignatures(topologyResult.Targets)
	prePersistSignatures := factoryvalidation.CanonicalTargetSignatures(prePersistResult.Targets)
	if factoryvalidation.EquivalentCanonicalTargetSignatures(topologySignatures, prePersistSignatures) {
		t.Fatal("expected ProfileTopology and ProfilePrePersist to differ on cross-path invalid fixture")
	}

	hasDeferredOutcomeRoute := false
	for _, target := range topologyResult.Targets {
		if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute ||
			target.Code == factoryvalidation.CodeWorkstationMissingRejectionRoute ||
			target.Code == factoryvalidation.CodeWorkTypeMissingCompletionState ||
			target.Code == factoryvalidation.CodeWorkTypeMissingFailureState ||
			target.Code == factoryvalidation.CodeWorkStateMissingTerminalPath {
			hasDeferredOutcomeRoute = true
			break
		}
	}
	if !hasDeferredOutcomeRoute {
		t.Fatalf("topology targets = %#v, want deferred outcome-route codes documenting validate-only gap", topologyResult.Targets)
	}

	for _, target := range prePersistResult.Targets {
		if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute ||
			target.Code == factoryvalidation.CodeWorkstationMissingRejectionRoute {
			t.Fatalf("pre-persist targets = %#v, want save path without deferred outcome-route codes", prePersistResult.Targets)
		}
	}

	if !canonicalTargetSignaturesSubset(prePersistSignatures, topologySignatures) {
		t.Fatalf("pre-persist signatures = %#v, want subset of topology signatures %#v",
			prePersistSignatures, topologySignatures)
	}
}

func canonicalTargetSignaturesSubset(subset, superset []string) bool {
	allowed := make(map[string]struct{}, len(superset))
	for _, signature := range superset {
		allowed[signature] = struct{}{}
	}
	for _, signature := range subset {
		if _, ok := allowed[signature]; !ok {
			return false
		}
	}
	return true
}
