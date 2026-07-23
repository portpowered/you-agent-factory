package logicaltarget

import (
	"errors"
	"testing"
)

func TestValidationError_RequiredReasonFieldAndUnwrap(t *testing.T) {
	t.Parallel()

	err := requiredFieldError("backendScopeId", "backend scope is required")
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ReasonRequired || field != "backendScopeId" {
		t.Fatalf("ValidationReasonFromError() = (%q, %q, %v), want required/backendScopeId/true", reason, field, ok)
	}

	var ve *validationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As validationError = false, want true")
	}
	if ve.Error() == "" || ve.Unwrap() == nil || ve.Reason() != ReasonRequired || ve.Field() != "backendScopeId" {
		t.Fatalf("validationError accessors = %#v, want populated required error", ve)
	}
}

func TestValidationError_InvalidAndAmbiguousReasons(t *testing.T) {
	t.Parallel()

	invalidErr := invalidTargetError("namedTarget", "")
	if reason, field, ok := ValidationReasonFromError(invalidErr); !ok || reason != ReasonInvalidTarget || field != "namedTarget" {
		t.Fatalf("invalid ValidationReasonFromError() = (%q, %q, %v)", reason, field, ok)
	}

	ambiguousErr := ambiguousTargetError("folderPath", "")
	if reason, field, ok := ValidationReasonFromError(ambiguousErr); !ok || reason != ReasonAmbiguousTarget || field != "folderPath" {
		t.Fatalf("ambiguous ValidationReasonFromError() = (%q, %q, %v)", reason, field, ok)
	}
}

func TestValidationError_NilAndNonValidationErrors(t *testing.T) {
	t.Parallel()

	if got := newValidationError(ReasonRequired, "field", nil); got != nil {
		t.Fatalf("newValidationError(nil err) = %v, want nil", got)
	}

	if reason, field, ok := ValidationReasonFromError(errors.New("other")); ok {
		t.Fatalf("ValidationReasonFromError(other) = (%q, %q, %v), want ok=false", reason, field, ok)
	}

	var nilVE *validationError
	if nilVE.Error() != "" || nilVE.Unwrap() != nil || nilVE.Reason() != "" || nilVE.Field() != "" {
		t.Fatalf("nil validationError accessors = (%q, %v, %q, %q), want empty", nilVE.Error(), nilVE.Unwrap(), nilVE.Reason(), nilVE.Field())
	}
}
