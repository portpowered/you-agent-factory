package validationassert

import (
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestHasDomainTargetCode_MatchesAnyTargetWithCode(t *testing.T) {
	targets := []factoryvalidation.Target{
		{Code: factoryvalidation.CodeDuplicateIdentifier},
		{Code: factoryvalidation.CodeDanglingWorkerReference},
	}

	HasDomainTargetCode(t, targets, factoryvalidation.CodeDanglingWorkerReference)
}

func TestHasDomainTargetSubject_MatchesSubject(t *testing.T) {
	want := factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "process",
		Location: factoryvalidation.SubjectLocationReference,
	}
	targets := []factoryvalidation.Target{{
		Code:    factoryvalidation.CodeDanglingPlaceReference,
		Subject: want,
	}}

	HasDomainTargetSubject(t, targets, want)
}
