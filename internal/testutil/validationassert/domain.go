package validationassert

import (
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

// HasDomainTargetCode asserts that targets contains a validation target with the
// given code.
func HasDomainTargetCode(t *testing.T, targets []factoryvalidation.Target, code string) {
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
func HasDomainTargetSubject(t *testing.T, targets []factoryvalidation.Target, want factoryvalidation.Subject) {
	t.Helper()
	for _, target := range targets {
		if target.Subject == want {
			return
		}
	}
	t.Fatalf("targets = %#v, want subject %#v", targets, want)
}
