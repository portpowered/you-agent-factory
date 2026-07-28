package recordingmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

const missingRecordingID = "recording-mcp-missing-001"

func TestBind_QueryStatusSuccessReturnsStatusFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRecordingsRoot{
		invoked: &invoked,
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != testRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, testRecordingID)
			}
			return recordings.RecordingStatusResult{
				Status: recordings.RecordingStatusFacts{
					RecordingID: testRecordingID,
					State:       recordings.RecordingActive,
				},
			}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake recordings root was not invoked")
	}
	var response mcprecording.ToolResponse[recordings.RecordingStatusResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Status.RecordingID != testRecordingID {
		t.Fatalf("recordingId = %q, want %q", response.Result.Status.RecordingID, testRecordingID)
	}
	if response.Result.Status.State != recordings.RecordingActive {
		t.Fatalf("state = %q, want ACTIVE", response.Result.Status.State)
	}
}

func TestBind_QueryStatusMissingTargetReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != missingRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, missingRecordingID)
			}
			return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+missingRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.target.missing",
		false,
		missingRecordingID,
	)
	if envelope.Message != "recording target not found" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "recording target not found", envelope)
	}
}

func TestBind_QueryStatusInvalidScopeReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		queryStatus: func(_ recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{}, recordings.ErrInvalidRecordingScope
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.target.missing",
		false,
		testRecordingID,
	)
	if envelope.Details == nil || !strings.Contains(envelope.Details["reason"].(string), "invalid recording scope") {
		t.Fatalf("error.details = %#v, want invalid recording scope reason", envelope.Details)
	}
}

func TestBind_QueryStatusInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake recordings root was invoked for invalid JSON decode")
	}
}

func TestBind_QueryStatusNilServiceReturnsUnavailableEnvelope(t *testing.T) {
	t.Parallel()

	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: nil})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.service.unavailable",
		false,
		"",
	)
	if envelope.Message != "recordings service is unavailable" {
		t.Fatalf("error.message = %q, want unavailable message; envelope = %#v", envelope.Message, envelope)
	}
}

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
	wantRecordingID string,
) *mcprecording.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage              `json:"result"`
		Error  *mcprecording.ToolErrorEnvelope `json:"error"`
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
	if wantRecordingID != "" && response.Error.RecordingID != wantRecordingID {
		t.Fatalf("error.recordingId = %q, want %q; envelope = %#v", response.Error.RecordingID, wantRecordingID, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}
