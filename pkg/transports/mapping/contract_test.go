package apisurface

import (
	"errors"
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestInvocationResponseRoundTripPreservesTerminalContract(t *testing.T) {
	t.Parallel()

	want := FactoryInvocationResult{
		RequestID: "request-1", TraceID: "trace-1",
		Status:    interfaces.InvocationTerminalStatusFailed,
		ErrorCode: "INVOCATION_RUNTIME_FAILURE", Message: "failed",
		SessionID: "session-1", WorkID: "work-1", WorkName: "customer work", WorkState: "task:failed",
		ApprovalID: "approval-1", DispatchID: "dispatch-1",
		WorkstationID: "review", WorkstationName: "Review", Decisions: []string{"APPROVE", "REJECT"},
	}
	got := FactoryInvocationResultFromResponse(InvocationResponseFromResult(want))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal invocation round trip = %#v, want %#v", got, want)
	}
}

func TestTopologyValidationErrorPreservesPublicContract(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{{}}
	validationErr := NewTopologyValidationError("", targets)
	targets[0] = factoryapi.FactoryValidationTarget{}

	if got := validationErr.Error(); got != "factory topology validation failed" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(validationErr, ErrInvalidNamedFactory) {
		t.Fatal("TopologyValidationError does not match ErrInvalidNamedFactory")
	}
	validationErr.Message = "invalid edge"
	if got := validationErr.Error(); got != "invalid edge" {
		t.Fatalf("custom Error() = %q", got)
	}
	var nilErr *TopologyValidationError
	if got := nilErr.Error(); got != "" {
		t.Fatalf("nil Error() = %q", got)
	}
}
