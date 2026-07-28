package modelmcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

const (
	testLeaseHolder    = "mcp-worker-1"
	testLeaseRef       = "model-lease-test-001"
	testInvokeOperation = "generate"
)

func TestBind_AcquireLeaseSuccessReturnsLeaseFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	expiresAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	fake := fakeModelsRoot{
		invoked: &invoked,
		acquireModelLease: func(_ context.Context, request models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
			if request.Scope.String() != testRuntimeScopeRef {
				t.Fatalf("scope = %q, want %q", request.Scope.String(), testRuntimeScopeRef)
			}
			if request.Name != testPrepareModelName {
				t.Fatalf("name = %q, want %q", request.Name, testPrepareModelName)
			}
			if request.Holder != testLeaseHolder {
				t.Fatalf("holder = %q, want %q", request.Holder, testLeaseHolder)
			}
			leaseRef, err := (models.ModelLeaseRef{}).Parse(testLeaseRef)
			if err != nil {
				t.Fatalf("parse lease ref: %v", err)
			}
			scope, err := (models.RuntimeScopeRef{}).Parse(testRuntimeScopeRef)
			if err != nil {
				t.Fatalf("parse scope: %v", err)
			}
			return models.AcquireModelLeaseResult{
				Lease: models.ModelLease{
					Lease:         leaseRef,
					Scope:         scope,
					ModelName:     testPrepareModelName,
					Holder:        testLeaseHolder,
					ExpiresAt:     expiresAt,
					Status:        models.ModelLeaseStatusActive,
					HostReadiness: models.ReadinessStateReady,
				},
			}, nil
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolAcquireLease,
		acquireLeaseInputJSON(testRuntimeScopeRef, testPrepareModelName, testLeaseHolder),
	)
	if err != nil {
		t.Fatalf("CallTool(acquire_lease) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake models root was not invoked")
	}
	var response modelmcp.ToolResponse[modelmcp.AcquireLeaseResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Lease.Lease != testLeaseRef {
		t.Fatalf("lease ref = %q, want %q", response.Result.Lease.Lease, testLeaseRef)
	}
	if response.Result.Lease.ModelName != testPrepareModelName {
		t.Fatalf("lease.modelName = %q, want %q", response.Result.Lease.ModelName, testPrepareModelName)
	}
	if response.Result.Lease.Holder != testLeaseHolder {
		t.Fatalf("lease.holder = %q, want %q", response.Result.Lease.Holder, testLeaseHolder)
	}
	if response.Result.Lease.Status != models.ModelLeaseStatusActive {
		t.Fatalf("lease.status = %q, want %q", response.Result.Lease.Status, models.ModelLeaseStatusActive)
	}
}

func TestBind_AcquireLeaseDomainErrorsReturnTypedEnvelopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rootErr       error
		wantCode      string
		wantRetryable bool
	}{
		{
			name:          "capacity exhausted",
			rootErr:       models.ErrHostCapacityExhausted,
			wantCode:      "model.lease.capacity_exhausted",
			wantRetryable: true,
		},
		{
			name:          "runtime not ready",
			rootErr:       models.ErrHostRuntimeNotReady,
			wantCode:      "model.host.runtime_not_ready",
			wantRetryable: true,
		},
		{
			name:          "capacity contended",
			rootErr:       models.ErrHostCapacityContended,
			wantCode:      "model.lease.capacity_contended",
			wantRetryable: true,
		},
		{
			name:          "foreign scope",
			rootErr:       models.ErrRuntimeScopeForeign,
			wantCode:      "model.runtime_scope.foreign",
			wantRetryable: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := fakeModelsRoot{
				acquireModelLease: func(context.Context, models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
					return models.AcquireModelLeaseResult{}, test.rootErr
				},
			}
			operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
			raw, err := operation(
				context.Background(),
				modelmcp.ToolAcquireLease,
				acquireLeaseInputJSON(testRuntimeScopeRef, testPrepareModelName, testLeaseHolder),
			)
			if err != nil {
				t.Fatalf("CallTool(acquire_lease) transport error = %v, want typed tool response", err)
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, test.wantCode, test.wantRetryable)
			if envelope.Details == nil || envelope.Details["reason"] != test.rootErr.Error() {
				t.Fatalf("error.details = %#v, want reason %q", envelope.Details, test.rootErr.Error())
			}
		})
	}
}

func TestBind_AcquireLeaseInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolAcquireLease,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`",`),
	)
	if err != nil {
		t.Fatalf("CallTool(acquire_lease) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for invalid JSON decode")
	}
}

func TestBind_AcquireLeaseMissingHolderReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolAcquireLease,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`","name":"`+testPrepareModelName+`","holder":""}`),
	)
	if err != nil {
		t.Fatalf("CallTool(acquire_lease) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for missing holder")
	}
}

func acquireLeaseInputJSON(runtimeScopeRef, name, holder string) json.RawMessage {
	payload, err := json.Marshal(map[string]string{
		"runtimeScopeRef": runtimeScopeRef,
		"name":            name,
		"holder":          holder,
	})
	if err != nil {
		panic(err)
	}
	return payload
}
