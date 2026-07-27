package script_pollers_test

import (
	"errors"
	"strings"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

func TestSubmitFailedError_WrapsAdmissionFailure(t *testing.T) {
	t.Parallel()

	cause := errors.New("ingress unavailable")
	err := scriptpollers.SubmitFailedError(cause)
	if err == nil {
		t.Fatal("expected typed submit failure")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeFailed {
		t.Fatalf("error code = %q, want %q", typed.Code, automations.ErrorCodeFailed)
	}
	if !strings.Contains(err.Error(), "submit failed") {
		t.Fatalf("error = %v, want submit failed message", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped cause %v", err, cause)
	}
}

func TestSubmitFailedError_NilReturnsNil(t *testing.T) {
	t.Parallel()

	if err := scriptpollers.SubmitFailedError(nil); err != nil {
		t.Fatalf("SubmitFailedError(nil) = %v, want nil", err)
	}
}
