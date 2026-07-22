package validationentry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// topologyDeferredOutcomeRouteCodes are validate-only structural findings that
// ProfilePrePersist intentionally omits after canonical load / blocking-load.
var topologyDeferredOutcomeRouteCodes = map[string]struct{}{
	"factory.workstation.missingFailureRoute":         {},
	"factory.workstation.missingRejectionRoute":       {},
	"factory.workType.missingCompletionState":         {},
	"factory.workType.missingFailureState":            {},
	"factory.workState.missingTerminalCompletionPath": {},
}

// prePersistDisallowedOutcomeRouteCodes must not appear on the save pre-check path.
var prePersistDisallowedOutcomeRouteCodes = map[string]struct{}{
	"factory.workstation.missingFailureRoute":   {},
	"factory.workstation.missingRejectionRoute": {},
}

// TestCrossPathInvalidFixture_ProfileTopologyDiffersFromPrePersistDocumentsValidateEndpointGap
// guards the intentional product gap: POST /factory-validations uses ProfileTopology
// (structural validation including deferred outcome-route findings), while editable save
// uses ProfilePrePersist (canonical load + blocking-load subset without those routes).
// Operators who validate-only then save should expect extra outcome-route targets from
// validate-only unless the endpoint profile is upgraded to ProfilePrePersist.
func TestCrossPathInvalidFixture_ProfileTopologyDiffersFromPrePersistDocumentsValidateEndpointGap(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	topologyFindings := validationResult(
		codeDuplicateIdentifier,
		"factory.workstation.missingFailureRoute",
		"factory.workType.missingCompletionState",
	)
	prePersistFindings := validationResult(codeDuplicateIdentifier)
	topologyResult := mustValidateFactoryAPI(t, factory, factoryvalidation.ValidationProfileTopology, topologyFindings)
	prePersistResult := mustValidateFactoryAPI(t, factory, factoryvalidation.ValidationProfilePrePersist, prePersistFindings)
	assertCrossPathInvalidProfileGap(t, topologyResult, prePersistResult)
}

func mustValidateFactoryAPI(
	t *testing.T,
	factory factoryapi.Factory,
	profile factoryvalidation.ValidationProfile,
	findings factoryvalidation.ValidationResult,
) factoryvalidation.ValidationResult {
	t.Helper()

	validator := testFactoryDefinitionValidator(findings)
	if profile == factoryvalidation.ValidationProfilePrePersist {
		validator = testFactoryDefinitionValidator(factoryvalidation.ValidationResult{}, findings)
	}
	return invokeDefinitionValidationRole(t, factory, profile, validator)
}

func assertCrossPathInvalidProfileGap(t *testing.T, topologyResult, prePersistResult factoryvalidation.ValidationResult) {
	t.Helper()

	if !topologyResult.HasTargets() || !prePersistResult.HasTargets() {
		t.Fatal("expected both profiles to reject cross-path invalid fixture")
	}

	topologySignatures := targetCodes(topologyResult.Targets)
	prePersistSignatures := targetCodes(prePersistResult.Targets)
	if len(topologySignatures) == len(prePersistSignatures) {
		t.Fatal("expected ProfileTopology and ProfilePrePersist to differ on cross-path invalid fixture")
	}

	if !targetsContainAnyCode(topologyResult.Targets, topologyDeferredOutcomeRouteCodes) {
		t.Fatalf("topology targets = %#v, want deferred outcome-route codes documenting validate-only gap", topologyResult.Targets)
	}
	if targetsContainAnyCode(prePersistResult.Targets, prePersistDisallowedOutcomeRouteCodes) {
		t.Fatalf("pre-persist targets = %#v, want save path without deferred outcome-route codes", prePersistResult.Targets)
	}
	if !canonicalTargetSignaturesSubset(prePersistSignatures, topologySignatures) {
		t.Fatalf("pre-persist signatures = %#v, want subset of topology signatures %#v",
			prePersistSignatures, topologySignatures)
	}
}

func targetsContainAnyCode(targets []factoryvalidation.ValidationTarget, codes map[string]struct{}) bool {
	for _, target := range targets {
		if _, ok := codes[target.Code]; ok {
			return true
		}
	}
	return false
}

func targetCodes(targets []factoryvalidation.ValidationTarget) []string {
	codes := make([]string, 0, len(targets))
	for _, target := range targets {
		codes = append(codes, target.Code)
	}
	return codes
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
