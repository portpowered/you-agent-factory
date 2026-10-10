package apisurface

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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

func TestWorkerSessionCallerHeaderValidation(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("A", 43)
	for _, tc := range []struct {
		name          string
		ids, auth     []string
		valid, absent bool
	}{
		{name: "absent", valid: true, absent: true},
		{name: "valid", ids: []string{"exact/caller"}, auth: []string{"Bearer " + token}, valid: true},
		{name: "case insensitive scheme", ids: []string{"caller"}, auth: []string{"bearer " + token}, valid: true},
		{name: "missing token", ids: []string{"caller"}},
		{name: "missing identity", auth: []string{"Bearer " + token}},
		{name: "empty identity", ids: []string{""}, auth: []string{"Bearer " + token}},
		{name: "empty authorization", ids: []string{"caller"}, auth: []string{""}},
		{name: "duplicate identity", ids: []string{"caller", "peer"}, auth: []string{"Bearer " + token}},
		{name: "duplicate token", ids: []string{"caller"}, auth: []string{"Bearer " + token, "Bearer " + token}},
		{name: "basic", ids: []string{"caller"}, auth: []string{"Basic " + token}},
		{name: "padded token", ids: []string{"caller"}, auth: []string{"Bearer " + token + "="}},
		{name: "short token", ids: []string{"caller"}, auth: []string{"Bearer short"}},
		{name: "noncanonical token", ids: []string{"caller"}, auth: []string{"Bearer " + strings.Repeat("A", 42) + "B"}},
		{name: "identity whitespace", ids: []string{" caller"}, auth: []string{"Bearer " + token}},
		{name: "token whitespace", ids: []string{"caller"}, auth: []string{"Bearer  " + token}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			headers := http.Header{}
			for _, id := range tc.ids {
				headers.Add("X-You-Worker-Session-Id", id)
			}
			for _, auth := range tc.auth {
				headers.Add("Authorization", auth)
			}
			caller, err := WorkerSessionCallerFromHeaders(headers)
			if tc.valid {
				if err != nil || (caller == nil) != tc.absent {
					t.Fatal("valid credentials were refused or changed")
				}
				if caller != nil && (caller.WorkerSessionID != tc.ids[0] || caller.Token != token) {
					t.Fatal("exact credentials changed")
				}
			} else if caller != nil || !errors.Is(err, workersessions.ErrCallerInvalid) {
				t.Fatal("invalid credentials were accepted")
			}
		})
	}
}
