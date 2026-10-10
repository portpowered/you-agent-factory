package apisurface

import (
	"encoding/json"
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
		ErrorCode: "INVOCATION_RUNTIME_FAILURE", Message: "failed", FailureReason: "throttled",
		SessionID: "session-1", WorkID: "work-1", WorkName: "customer work", WorkState: "task:failed",
		ApprovalID: "approval-1", DispatchID: "dispatch-1",
		WorkstationID: "review", WorkstationName: "Review", Decisions: []string{"APPROVE", "REJECT"},
	}
	got := FactoryInvocationResultFromResponse(InvocationResponseFromResult(want))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal invocation round trip = %#v, want %#v", got, want)
	}
}

func TestInvocationResponseFailureReasonPrivacy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, reason, message, want string
		status                      interfaces.InvocationTerminalStatus
	}{
		{name: "success", status: interfaces.InvocationTerminalStatusCompleted, reason: "throttled"},
		{name: "recognized", status: interfaces.InvocationTerminalStatusFailed, reason: "throttled", message: "failed", want: "throttled"},
		{name: "empty message", status: interfaces.InvocationTerminalStatusFailed, reason: "auth_failure", want: "auth_failure"},
		{name: "legacy absent", status: interfaces.InvocationTerminalStatusFailed},
		{name: "unrecognized", status: interfaces.InvocationTerminalStatusFailed, reason: "planted-private-reason"},
		{name: "diagnostic only", status: interfaces.InvocationTerminalStatusFailed, message: "throttled"},
		{name: "inexact category", status: interfaces.InvocationTerminalStatusFailed, reason: " throttled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			response := InvocationResponseFromResult(FactoryInvocationResult{
				RequestID: "request", TraceID: "trace", Status: tc.status, FailureReason: tc.reason, Message: tc.message,
			})
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			var decoded factoryapi.InvocationResponse
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			got := FactoryInvocationResultFromResponse(decoded)
			if got.FailureReason != tc.want || got.Message != tc.message || got.Status != tc.status {
				t.Fatalf("public terminal result = %#v", got)
			}
			if tc.want == "" && (response.FailureReason != nil || strings.Contains(string(encoded), "failureReason")) {
				t.Fatal("unrecognized or successful failure category was published")
			}
			if strings.Contains(string(encoded), "planted-private-reason") {
				t.Fatal("raw failure diagnostic escaped the closed category boundary")
			}
			// A peer may decode an unknown enum string without schema validation.
			injected := factoryapi.WorkFailureType(tc.reason)
			decoded.FailureReason = &injected
			if got := FactoryInvocationResultFromResponse(decoded); got.FailureReason != tc.want {
				t.Fatal("peer response acquired an unrecognized failure category")
			}
		})
	}
}

func TestInvocationResponsePreservesRecognizedFailureCategories(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{
		"auth_failure", "permanent_bad_request", "throttled", "internal_server_error", "timeout", "unknown",
		"misconfigured", "missing_executable", "command_line_too_long", "structured_output_schema_violation",
		"EXPECTED_ARTIFACTS_UNSATISFIED", "worker_declared_failure",
	} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			result := FactoryInvocationResult{Status: interfaces.InvocationTerminalStatusFailed, FailureReason: reason}
			if got := FactoryInvocationResultFromResponse(InvocationResponseFromResult(result)); got.FailureReason != reason {
				t.Fatalf("owner-selected category = %q, want %q", got.FailureReason, reason)
			}
		})
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
