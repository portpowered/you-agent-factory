package validationassert

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestHasTarget_MatchesCodeSubjectAndLocation(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{{
		Code: "factory.workstation.missingFailureRoute",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}}

	HasTarget(
		t,
		targets,
		"factory.workstation.missingFailureRoute",
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"missing failure route target",
	)
}

func TestHasTargetCode_MatchesAnyTargetWithCode(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{
		{Code: "factory.identifier.duplicate"},
		{Code: "factory.route.danglingWorkerReference"},
	}

	HasTargetCode(t, targets, "factory.route.danglingWorkerReference")
}

func TestHasTargetCode_MatchesWithOptionalTopologyLabel(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{
		{Code: "factory.workstation.missingFailureRoute"},
	}

	HasTargetCode(t, targets, "factory.workstation.missingFailureRoute", "missing failure route target")
}
