// Package validationassert provides shared test helpers for factory validation
// target assertions. Import it only from test code.
package validationassert

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// HasTarget asserts that targets contains a validation target matching code,
// subject type, subject id, and location. want is included in the failure message.
func HasTarget(
	t *testing.T,
	targets []factoryapi.FactoryValidationTarget,
	code string,
	subjectType factoryapi.FactoryValidationSubjectType,
	subjectID string,
	location factoryapi.FactoryValidationSubjectLocation,
	want string,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code {
			continue
		}
		if target.Subject.Type != subjectType || target.Subject.Id != subjectID || target.Subject.Location != location {
			continue
		}
		return
	}
	t.Fatalf("validation targets = %#v, want %s", targets, want)
}

// HasTargetCode asserts that targets contains a validation target with the given
// code. When want is non-empty, the failure message uses the service topology
// label form; otherwise it reports the missing code.
func HasTargetCode(t *testing.T, targets []factoryapi.FactoryValidationTarget, code string, want ...string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	if len(want) > 0 && want[0] != "" {
		t.Fatalf("topology targets = %#v, want %s", targets, want[0])
	}
	t.Fatalf("targets = %#v, want code %q", targets, code)
}
