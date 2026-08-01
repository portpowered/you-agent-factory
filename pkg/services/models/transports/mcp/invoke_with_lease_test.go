package modelmcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

var errUnknownInvokeFailure = errors.New("invoke failed for an unmapped internal reason")

func TestBind_InvokeWithLeaseSuccessReturnsInvokeFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	leaseRef, err := (models.ModelLeaseRef{}).Parse(testLeaseRef)
	if err != nil {
		t.Fatalf("parse lease ref: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(testRuntimeScopeRef)
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	invocationRef, err := (models.ModelInvocationRef{}).Parse("model-invocation-test-001")
	if err != nil {
		t.Fatalf("parse invocation ref: %v", err)
	}
	artifactRef, err := (models.InferenceArtifactRef{}).Parse("artifact-test-001")
	if err != nil {
		t.Fatalf("parse artifact ref: %v", err)
	}
	fake := fakeModelsRoot{
		invoked: &invoked,
		invokeModelWithLease: func(_ context.Context, request models.InvokeModelRequest) (models.InvokeModelResult, error) {
			if request.Scope.String() != testRuntimeScopeRef {
				t.Fatalf("scope = %q, want %q", request.Scope.String(), testRuntimeScopeRef)
			}
			if request.Lease.String() != testLeaseRef {
				t.Fatalf("lease = %q, want %q", request.Lease.String(), testLeaseRef)
			}
			if request.Holder != testLeaseHolder {
				t.Fatalf("holder = %q, want %q", request.Holder, testLeaseHolder)
			}
			if request.ModelName != testPrepareModelName {
				t.Fatalf("modelName = %q, want %q", request.ModelName, testPrepareModelName)
			}
			if request.Operation != testInvokeOperation {
				t.Fatalf("operation = %q, want %q", request.Operation, testInvokeOperation)
			}
			if request.Input.ContentType != "text/plain" || request.Input.Content != "hello" {
				t.Fatalf("input = %#v, want text/plain hello", request.Input)
			}
			return models.InvokeModelResult{
				Invocation:       invocationRef,
				Scope:            scope,
				Lease:            leaseRef,
				ModelName:        testPrepareModelName,
				Operation:        testInvokeOperation,
				Status:           models.ModelInvocationStatusCompleted,
				Content:          []models.InferenceContent{{ContentType: "text/plain", Content: "models-owned-output"}},
				Artifacts:        []models.InferenceArtifact{{Artifact: artifactRef, Name: "output.txt", MediaType: "text/plain", SizeBytes: 5}},
				LeaseDisposition: models.InvocationLeaseReleased,
			}, nil
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolInvokeWithLease,
		invokeWithLeaseInputJSON(testRuntimeScopeRef, testLeaseRef, testLeaseHolder, testPrepareModelName, testInvokeOperation),
	)
	if err != nil {
		t.Fatalf("CallTool(invoke_with_lease) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake models root was not invoked")
	}
	var response modelmcp.ToolResponse[modelmcp.InvokeWithLeaseResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Invocation != invocationRef.String() {
		t.Fatalf("invocation = %q, want %q", response.Result.Invocation, invocationRef.String())
	}
	if response.Result.Lease != testLeaseRef {
		t.Fatalf("lease = %q, want %q", response.Result.Lease, testLeaseRef)
	}
	if response.Result.Status != models.ModelInvocationStatusCompleted {
		t.Fatalf("status = %q, want %q", response.Result.Status, models.ModelInvocationStatusCompleted)
	}
	if response.Result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("leaseDisposition = %q, want %q", response.Result.LeaseDisposition, models.InvocationLeaseReleased)
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Content != "models-owned-output" {
		t.Fatalf("content = %#v, want one models-owned-output item", response.Result.Content)
	}
	if len(response.Result.Artifacts) != 1 || response.Result.Artifacts[0].Name != "output.txt" {
		t.Fatalf("artifacts = %#v, want one output.txt artifact", response.Result.Artifacts)
	}
	if response.Result.Artifacts[0].Artifact != artifactRef.String() {
		t.Fatalf("artifact ref = %q, want %q", response.Result.Artifacts[0].Artifact, artifactRef.String())
	}
}

func TestBind_InvokeWithLeaseDomainErrorsReturnDistinctTypedEnvelopes(t *testing.T) {
	t.Parallel()

	for _, test := range invokeWithLeaseDomainErrorCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInvokeWithLeaseDomainErrorEnvelope(t, test)
		})
	}
}

type invokeWithLeaseDomainErrorCase struct {
	name          string
	rootErr       error
	wantCode      string
	wantRetryable bool
	wantMessage   string
	wantReason    string
}

func invokeWithLeaseDomainErrorCases() []invokeWithLeaseDomainErrorCase {
	return []invokeWithLeaseDomainErrorCase{
		{
			name: "runtime scope invalid", rootErr: models.ErrRuntimeScopeInvalid,
			wantCode: "model.runtime_scope.invalid", wantRetryable: false,
			wantMessage: "models runtime scope is invalid", wantReason: models.ErrRuntimeScopeInvalid.Error(),
		},
		{
			name: "capacity exhausted", rootErr: models.ErrHostCapacityExhausted,
			wantCode: "model.lease.capacity_exhausted", wantRetryable: true,
			wantMessage: "models lease capacity is exhausted", wantReason: models.ErrHostCapacityExhausted.Error(),
		},
		{
			name: "capacity contended", rootErr: models.ErrHostCapacityContended,
			wantCode: "model.lease.capacity_contended", wantRetryable: true,
			wantMessage: "models lease capacity is contended", wantReason: models.ErrHostCapacityContended.Error(),
		},
		{
			name: "runtime not ready", rootErr: models.ErrHostRuntimeNotReady,
			wantCode: "model.host.runtime_not_ready", wantRetryable: true,
			wantMessage: "models host runtime is not ready", wantReason: models.ErrHostRuntimeNotReady.Error(),
		},
		{
			name: "lease expired", rootErr: models.ErrHostLeaseExpired,
			wantCode: "model.lease.expired", wantRetryable: false,
			wantMessage: "models lease has expired", wantReason: models.ErrHostLeaseExpired.Error(),
		},
		{
			name: "lease not found", rootErr: models.ErrHostLeaseNotFound,
			wantCode: "model.lease.not_found", wantRetryable: false,
			wantMessage: "models lease was not found", wantReason: models.ErrHostLeaseNotFound.Error(),
		},
		{
			name: "invalid holder", rootErr: models.ErrHostInvalidHolder,
			wantCode: "model.lease.invalid_holder", wantRetryable: false,
			wantMessage: "models lease holder is invalid", wantReason: models.ErrHostInvalidHolder.Error(),
		},
		{
			name: "asset unavailable", rootErr: models.ErrAssetUnavailable,
			wantCode: "model.asset.unavailable", wantRetryable: true,
			wantMessage: "models assets are unavailable", wantReason: models.ErrAssetUnavailable.Error(),
		},
		{
			name: "inference timeout", rootErr: models.ErrInferenceTimeout,
			wantCode: "model.inference.timeout", wantRetryable: true,
			wantMessage: "models inference timed out", wantReason: models.ErrInferenceTimeout.Error(),
		},
		{
			name: "inference failed", rootErr: models.ErrInferenceFailed,
			wantCode: "model.inference.failed", wantRetryable: false,
			wantMessage: "models inference failed", wantReason: models.ErrInferenceFailed.Error(),
		},
		{
			name: "unsupported operation", rootErr: models.ErrUnsupportedModelOperation,
			wantCode: "model.operation.unsupported", wantRetryable: false,
			wantMessage: "models operation is not supported", wantReason: models.ErrUnsupportedModelOperation.Error(),
		},
		{
			name: "unsupported response mode", rootErr: models.ErrUnsupportedResponseMode,
			wantCode: "model.inference.response_mode_unsupported", wantRetryable: false,
			wantMessage: "models inference response mode is not supported", wantReason: models.ErrUnsupportedResponseMode.Error(),
		},
		{
			name: "request canceled", rootErr: context.Canceled,
			wantCode: "model.request.canceled", wantRetryable: false,
			wantMessage: "models request was canceled", wantReason: "CANCELED",
		},
		{
			name: "request timed out", rootErr: context.DeadlineExceeded,
			wantCode: "model.request.timed_out", wantRetryable: true,
			wantMessage: "models request timed out", wantReason: "TIMED_OUT",
		},
		{
			name: "unknown execution failure", rootErr: errUnknownInvokeFailure,
			wantCode: "model.execution.internal", wantRetryable: false,
			wantMessage: errUnknownInvokeFailure.Error(), wantReason: errUnknownInvokeFailure.Error(),
		},
	}
}

func assertInvokeWithLeaseDomainErrorEnvelope(t *testing.T, test invokeWithLeaseDomainErrorCase) {
	t.Helper()

	fake := fakeModelsRoot{
		invokeModelWithLease: func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error) {
			return models.InvokeModelResult{}, test.rootErr
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolInvokeWithLease,
		invokeWithLeaseInputJSON(testRuntimeScopeRef, testLeaseRef, testLeaseHolder, testPrepareModelName, testInvokeOperation),
	)
	if err != nil {
		t.Fatalf("CallTool(invoke_with_lease) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, test.wantCode, test.wantRetryable)
	if envelope.Message != test.wantMessage {
		t.Fatalf("error.message = %q, want %q", envelope.Message, test.wantMessage)
	}
	if envelope.Details == nil || envelope.Details["reason"] != test.wantReason {
		t.Fatalf("error.details = %#v, want reason %q", envelope.Details, test.wantReason)
	}
}

func TestBind_InvokeWithLeaseInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolInvokeWithLease,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`",`),
	)
	if err != nil {
		t.Fatalf("CallTool(invoke_with_lease) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for invalid JSON decode")
	}
}

func TestBind_InvokeWithLeaseMissingOperationReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolInvokeWithLease,
		invokeWithLeaseInputJSON(testRuntimeScopeRef, testLeaseRef, testLeaseHolder, testPrepareModelName, ""),
	)
	if err != nil {
		t.Fatalf("CallTool(invoke_with_lease) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for missing operation")
	}
}

func invokeWithLeaseInputJSON(runtimeScopeRef, leaseRef, holder, modelName, operation string) json.RawMessage {
	payload, err := json.Marshal(map[string]any{
		"runtimeScopeRef": runtimeScopeRef,
		"leaseRef":        leaseRef,
		"holder":          holder,
		"modelName":       modelName,
		"operation":       operation,
		"input": map[string]string{
			"contentType": "text/plain",
			"content":     "hello",
		},
	})
	if err != nil {
		panic(err)
	}
	return payload
}
