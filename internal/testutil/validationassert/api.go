// Package validationassert provides shared test helpers for factory validation
// target assertions. Import it only from test code.
package validationassert

import (
	"fmt"
	"sort"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// CanonicalAPITargetSignatures returns sorted stable signatures for generated
// API validation targets. It belongs in test support because production domain
// validation must not depend on generated transport contracts.
func CanonicalAPITargetSignatures(targets []factoryapi.FactoryValidationTarget) []string {
	signatures := make([]string, 0, len(targets))
	for _, target := range targets {
		signatures = append(signatures, fmt.Sprintf(
			"%s|%s|%s|%s",
			target.Code,
			target.Subject.Type,
			target.Subject.Id,
			target.Subject.Location,
		))
	}
	sort.Strings(signatures)
	return signatures
}

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
