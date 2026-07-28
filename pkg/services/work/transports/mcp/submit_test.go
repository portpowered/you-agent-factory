package workmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	work "github.com/portpowered/infinite-you/pkg/services/work"
	workmcp "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
)

const testSubmitSessionID = "session-mcp-submit-001"
const testSubmitRequestID = "request-mcp-submit-001"

func TestBind_SubmitSuccessReturnsAcceptedFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	wantRequest := testSubmitWorkRequest()
	fake := fakeWorkRoot{
		invoked: &invoked,
		submitWorkRequestForSession: func(
			_ context.Context,
			sessionID string,
			request work.WorkRequest,
		) (work.WorkRequestSubmitResult, error) {
			if sessionID != testSubmitSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, testSubmitSessionID)
			}
			if request.RequestID != wantRequest.RequestID {
				t.Fatalf("requestId = %q, want %q", request.RequestID, wantRequest.RequestID)
			}
			if len(request.Works) != 1 || request.Works[0].Name != wantRequest.Works[0].Name {
				t.Fatalf("workRequest = %#v, want %#v", request, wantRequest)
			}
			return work.WorkRequestSubmitResult{
				RequestID:    testSubmitRequestID,
				TraceID:      "trace-mcp-submit-001",
				WorkID:       "work-mcp-submit-001",
				Name:         "story-submit",
				WorkTypeName: "story",
				Accepted:     true,
				Works: []work.WorkRequestSubmittedWork{{
					Name:         "story-submit",
					WorkTypeName: "story",
					WorkID:       "work-mcp-submit-001",
				}},
			}, nil
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolSubmit,
		testSubmitInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(submit) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Work root was not invoked")
	}
	var response workmcp.ToolResponse[work.WorkRequestSubmitResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if !response.Result.Accepted || response.Result.RequestID != testSubmitRequestID {
		t.Fatalf("submit result = %#v, want accepted %q", response.Result, testSubmitRequestID)
	}
	if len(response.Result.Works) != 1 || response.Result.Works[0].WorkID != "work-mcp-submit-001" {
		t.Fatalf("submit works = %#v, want work-mcp-submit-001", response.Result.Works)
	}
}

func TestBind_SubmitSuccessEncodesCallToolResultTransport(t *testing.T) {
	t.Parallel()

	fake := fakeWorkRoot{
		submitWorkRequestForSession: func(
			_ context.Context,
			_ string,
			request work.WorkRequest,
		) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{
				RequestID: request.RequestID,
				Accepted:  true,
			}, nil
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolSubmit,
		testSubmitInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(submit) transport error = %v, want typed tool response", err)
	}

	projected, err := workmcp.MarshalSuccessCallToolResultJSON(raw)
	if err != nil {
		t.Fatalf("MarshalSuccessCallToolResultJSON() error = %v", err)
	}
	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError *bool `json:"isError"`
	}
	if err := json.Unmarshal(projected, &envelope); err != nil {
		t.Fatalf("decode CallToolResult envelope: %v", err)
	}
	if len(envelope.Content) != 1 {
		t.Fatalf("content item count = %d, want 1", len(envelope.Content))
	}
	if envelope.Content[0].Type != "text" {
		t.Fatalf("content[0].type = %q, want text", envelope.Content[0].Type)
	}
	if envelope.Content[0].Text != string(raw) {
		t.Fatalf("content[0].text = %q, want serialized tool response %q", envelope.Content[0].Text, raw)
	}
	if envelope.IsError != nil {
		t.Fatalf("isError = %v, want omitted or false for success transport", *envelope.IsError)
	}
}

func TestBind_SubmitAdmissionFailuresReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		rootErr       error
		wantCode      string
		wantMessage   string
		wantRetryable bool
	}{
		{
			name:          "invalid",
			rootErr:       work.ErrInvalidWorkRequest,
			wantCode:      "work.admission.invalid",
			wantMessage:   "invalid Work Request",
			wantRetryable: false,
		},
		{
			name:          "conflict",
			rootErr:       work.ErrWorkRequestConflict,
			wantCode:      "work.admission.conflict",
			wantMessage:   "Work Request admission conflict",
			wantRetryable: false,
		},
		{
			name:          "rejected",
			rootErr:       work.ErrWorkRequestRejected,
			wantCode:      "work.admission.rejected",
			wantMessage:   "Work Request rejected",
			wantRetryable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var invoked bool
			fake := fakeWorkRoot{
				invoked: &invoked,
				submitWorkRequestForSession: func(
					context.Context,
					string,
					work.WorkRequest,
				) (work.WorkRequestSubmitResult, error) {
					return work.WorkRequestSubmitResult{}, tc.rootErr
				},
			}
			operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
			raw, err := operation(
				context.Background(),
				workmcp.ToolSubmit,
				testSubmitInputJSON(),
			)
			if err != nil {
				t.Fatalf("CallTool(submit) transport error = %v, want typed tool response", err)
			}
			if !invoked {
				t.Fatal("fake Work root was not invoked")
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, tc.wantCode, tc.wantRetryable)
			if envelope.Message != tc.wantMessage {
				t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, tc.wantMessage, envelope)
			}
		})
	}
}

func TestBind_SubmitAdmissionFailuresAreDistinct(t *testing.T) {
	t.Parallel()

	invalidRaw := mustCallSubmit(t, fakeWorkRoot{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, work.ErrInvalidWorkRequest
		},
	})
	conflictRaw := mustCallSubmit(t, fakeWorkRoot{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, work.ErrWorkRequestConflict
		},
	})
	rejectedRaw := mustCallSubmit(t, fakeWorkRoot{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, work.ErrWorkRequestRejected
		},
	})

	invalidEnvelope := assertTypedToolErrorEnvelope(t, invalidRaw, "work.admission.invalid", false)
	conflictEnvelope := assertTypedToolErrorEnvelope(t, conflictRaw, "work.admission.conflict", false)
	rejectedEnvelope := assertTypedToolErrorEnvelope(t, rejectedRaw, "work.admission.rejected", false)

	if invalidEnvelope.Code == conflictEnvelope.Code || invalidEnvelope.Code == rejectedEnvelope.Code {
		t.Fatalf("admission error codes should be distinct: %#v %#v %#v", invalidEnvelope, conflictEnvelope, rejectedEnvelope)
	}
}

func TestBind_SubmitMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := workmcp.Bind(workmcp.RootDependencies{
		Work: fakeWorkRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		workmcp.ToolSubmit,
		json.RawMessage(`{"sessionId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(submit) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode submit work input") {
		t.Fatalf("error.message = %q, want decode submit work input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Work root was invoked for malformed JSON")
	}
}

func mustCallSubmit(t *testing.T, fake fakeWorkRoot) json.RawMessage {
	t.Helper()

	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolSubmit,
		testSubmitInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(submit) transport error = %v", err)
	}
	return raw
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
) *workmcp.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage           `json:"result"`
		Error  *workmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}

func testSubmitWorkRequest() work.WorkRequest {
	return work.WorkRequest{
		RequestID: testSubmitRequestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "story-submit",
			WorkTypeID: "story",
			State:      "draft",
		}},
	}
}

func testSubmitInputJSON() json.RawMessage {
	return json.RawMessage(`{"sessionId":"` + testSubmitSessionID +
		`","workRequest":{"requestId":"` + testSubmitRequestID +
		`","type":"FACTORY_REQUEST_BATCH","works":[{"name":"story-submit","workTypeName":"story","state":"draft"}]}}`)
}
