package validationassert

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestHasDomainTargetCode_MatchesAnyTargetWithCode(t *testing.T) {
	targets := []factorydefinitions.ValidationTarget{
		{Code: "factory.duplicateIdentifier"},
		{Code: "factory.worker.danglingReference"},
	}

	HasDomainTargetCode(t, targets, "factory.worker.danglingReference")
}

func TestHasDomainTargetSubject_MatchesSubject(t *testing.T) {
	want := factorydefinitions.ValidationSubject{
		Type:     factorydefinitions.ValidationSubjectTypeWorkstation,
		ID:       "process",
		Location: factorydefinitions.ValidationSubjectLocationReference,
	}
	targets := []factorydefinitions.ValidationTarget{{
		Code:    "factory.route.danglingPlaceReference",
		Subject: want,
	}}

	HasDomainTargetSubject(t, targets, want)
}
