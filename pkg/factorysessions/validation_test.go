package factorysessions

import (
	"errors"
	"testing"
)

func TestValidationReasonFromError_RecognizesStructuredValidationErrors(t *testing.T) {
	err := NewValidationError(ValidationReasonUnreadable, "folderPath", errors.New("permission denied"))
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonUnreadable || field != "folderPath" {
		t.Fatalf("ValidationReasonFromError = (%q, %q, %v)", reason, field, ok)
	}

	var ve *validationError
	if !errors.As(err, &ve) {
		t.Fatal("expected validationError type")
	}
	targets := ve.ErrorTargets()
	if len(targets) != 1 || targets[0].Code == "" || targets[0].Subject.Id != "folderPath" {
		t.Fatalf("ErrorTargets() = %#v, want folderPath validation target", targets)
	}
}

func TestValidationReasonFromError_IgnoresUnrelatedErrors(t *testing.T) {
	if _, _, ok := ValidationReasonFromError(errors.New("other")); ok {
		t.Fatal("ValidationReasonFromError(other) = ok, want false")
	}
}
