package sessionvalidation

import (
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestReasonFromErrorRecognizesStructuredValidationErrors(t *testing.T) {
	err := New(factorysessions.ValidationReasonUnreadable, "folderPath", errors.New("permission denied"))
	reason, field, ok := ReasonFromError(err)
	if !ok || reason != factorysessions.ValidationReasonUnreadable || field != "folderPath" {
		t.Fatalf("ReasonFromError = (%q, %q, %v)", reason, field, ok)
	}

	var validation *Error
	if !errors.As(err, &validation) {
		t.Fatal("expected Error type")
	}
	targets := validation.ErrorTargets()
	if len(targets) != 1 || targets[0].Code == "" || targets[0].Subject.ID != "folderPath" {
		t.Fatalf("ErrorTargets() = %#v, want folderPath validation target", targets)
	}
}

func TestReasonFromErrorIgnoresUnrelatedErrors(t *testing.T) {
	if _, _, ok := ReasonFromError(errors.New("other")); ok {
		t.Fatal("ReasonFromError(other) = ok, want false")
	}
}
