package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSetResourceCapacitySendsStableIDAndRendersJSON(t *testing.T) {
	var gotRequest factoryapi.FactorySessionResourceCapacityRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/factory-sessions/session-beta/resources/reviewers/capacity" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-You-Source") != "cli" {
			t.Fatalf("source header = %q, want cli", r.Header.Get("X-You-Source"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResourceCapacityResponse{
			SessionId: "session-beta", ResourceId: "reviewers", PreviousCapacity: 1,
			RequestedCapacity: 8, EffectiveCapacity: 8, InUseCount: 1, AvailableCount: 7,
			MinimumCapacity: 1, Outcome: factoryapi.FactorySessionResourceCapacityOutcome("APPLIED"),
			Revision: 3, RequestId: "raise-1", ChangeId: "live-change/raise-1",
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewSetResourceCapacity(testHTTPProtocol(t))(ResourceCapacityConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-beta", ResourceID: "reviewers",
		Capacity: 8, ExpectedRevision: 2, RequestID: "raise-1", Reason: "raise throughput", JSON: true, Output: &output,
	})
	if err != nil {
		t.Fatalf("SetResourceCapacity: %v", err)
	}
	if gotRequest.RequestId != "raise-1" || gotRequest.ExpectedRevision != 2 || gotRequest.Capacity != 8 || gotRequest.Reason == nil || *gotRequest.Reason != "raise throughput" {
		t.Fatalf("request = %#v", gotRequest)
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if response.ResourceId != "reviewers" || response.EffectiveCapacity != 8 || response.Revision != 3 {
		t.Fatalf("response = %#v", response)
	}
}

func TestSetResourceCapacityHumanOutputIncludesAccountingAndLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResourceCapacityResponse{
			SessionId: "~default", ResourceId: "reviewers", PreviousCapacity: 1,
			RequestedCapacity: 8, EffectiveCapacity: 8, InUseCount: 1, AvailableCount: 7,
			MinimumCapacity: 1, Outcome: factoryapi.FactorySessionResourceCapacityOutcome("APPLIED"),
			Revision: 3, RequestId: "raise-1", ChangeId: "live-change/raise-1",
			Links: &factoryapi.FactorySessionResourceCapacityLinks{
				Session: stringPointer("/factory-sessions/~default"), Events: stringPointer("/factory-sessions/~default/events"), Status: stringPointer("/factory-sessions/~default/status"),
			},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := SetResourceCapacity(ResourceCapacityConfig{
		Context: context.Background(), Server: server.URL, ResourceID: "reviewers", Capacity: 8,
		ExpectedRevision: 2, RequestID: "raise-1", Output: &output, HTTP: testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("SetResourceCapacity: %v", err)
	}
	for _, want := range []string{"Resource ID:\treviewers", "In-use count:\t1", "Available count:\t7", "Outcome:\tAPPLIED", "Request ID:\traise-1", "Events link:\t/factory-sessions/~default/events"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, missing %q", output.String(), want)
		}
	}
}

func TestSetResourceCapacityReturnsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "reviewers has units in use", Code: factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE})
	}))
	defer server.Close()

	err := SetResourceCapacity(ResourceCapacityConfig{
		Context: context.Background(), Server: server.URL, ResourceID: "reviewers", Capacity: 0,
		ExpectedRevision: 2, RequestID: "lower-1", Output: &bytes.Buffer{}, HTTP: testHTTPProtocol(t),
	})
	var rejected *ResourceCapacityRejectedError
	if !errors.As(err, &rejected) || rejected.Response.Code != factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE {
		t.Fatalf("error = %v, want typed capacity-in-use error", err)
	}
}

func TestResourceCapacityRejectedErrorPreservesCLIContract(t *testing.T) {
	t.Parallel()

	rejected := &ResourceCapacityRejectedError{
		StatusCode: http.StatusConflict,
		Response: factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE,
			Message: "reviewers has units in use",
		},
	}
	if got, want := rejected.CLIErrorCode(), string(factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE); got != want {
		t.Fatalf("CLIErrorCode() = %q, want %q", got, want)
	}
	if got, want := rejected.CLIErrorMessage(), rejected.Error(); got != want {
		t.Fatalf("CLIErrorMessage() = %q, want %q", got, want)
	}
	if !strings.Contains(rejected.Error(), "reviewers has units in use") {
		t.Fatalf("Error() = %q, want API message", rejected.Error())
	}

	var nilRejected *ResourceCapacityRejectedError
	if got, want := nilRejected.CLIErrorCode(), "RESOURCE_CAPACITY_REQUEST_FAILED"; got != want {
		t.Fatalf("nil CLIErrorCode() = %q, want %q", got, want)
	}
	if got, want := nilRejected.CLIErrorMessage(), "resource capacity request rejected"; got != want {
		t.Fatalf("nil CLIErrorMessage() = %q, want %q", got, want)
	}
	if got := nilRejected.Error(); got != "" {
		t.Fatalf("nil Error() = %q, want empty", got)
	}
}

func stringPointer(value string) *string { return &value }
