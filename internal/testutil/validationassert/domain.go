package validationassert

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// HasDomainTargetCode asserts that targets contains a validation target with the
// given code.
func HasDomainTargetCode(t *testing.T, targets []factorydefinitions.ValidationTarget, code string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("targets = %#v, want code %q", targets, code)
}

// HasDomainTargetSubject asserts that targets contains a validation target whose
// subject equals want.
func HasDomainTargetSubject(t *testing.T, targets []factorydefinitions.ValidationTarget, want factorydefinitions.ValidationSubject) {
	t.Helper()
	for _, target := range targets {
		if target.Subject == want {
			return
		}
	}
	t.Fatalf("targets = %#v, want subject %#v", targets, want)
}
