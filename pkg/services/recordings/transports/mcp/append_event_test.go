package recordingmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

const testAppendEventID = "event-mcp-append-001"

func TestBind_AppendEventSuccessReturnsAcceptedFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	wantEvent := testAppendCanonicalEvent()
	fake := fakeRecordingsRoot{
		invoked: &invoked,
		appendEvent: func(request recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
			if request.Event.ID != wantEvent.ID {
				t.Fatalf("event.id = %q, want %q", request.Event.ID, wantEvent.ID)
			}
			if request.Event.Kind != wantEvent.Kind {
				t.Fatalf("event.kind = %q, want %q", request.Event.Kind, wantEvent.Kind)
			}
			return recordings.AppendRecordedEventResult{
				Event: recordings.CanonicalEvent{
					ID:       wantEvent.ID,
					Sequence: 7,
					Kind:     wantEvent.Kind,
					Payload:  wantEvent.Payload,
				},
			}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolAppendEvent,
		testAppendEventInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(append_event) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake recordings root was not invoked")
	}
	var response mcprecording.ToolResponse[recordings.AppendRecordedEventResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Event.Sequence != 7 {
		t.Fatalf("event.sequence = %d, want 7", response.Result.Event.Sequence)
	}
	if response.Result.Event.ID != wantEvent.ID {
		t.Fatalf("event.id = %q, want %q", response.Result.Event.ID, wantEvent.ID)
	}
}

func TestBind_AppendEventSuccessEncodesCallToolResultTransport(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		appendEvent: func(request recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
			return recordings.AppendRecordedEventResult{Event: request.Event}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolAppendEvent,
		testAppendEventInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(append_event) transport error = %v, want typed tool response", err)
	}

	projected, err := mcprecording.MarshalSuccessCallToolResultJSON(raw)
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

func TestBind_AppendEventInvalidAppendReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		appendEvent: func(_ recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
			return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolAppendEvent,
		testAppendEventInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(append_event) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.append.invalid",
		false,
		"",
	)
	if envelope.Message != "invalid canonical event append" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "invalid canonical event append", envelope)
	}
	if envelope.Details == nil || envelope.Details["eventId"] != testAppendEventID {
		t.Fatalf("error.details = %#v, want eventId %q", envelope.Details, testAppendEventID)
	}
}

func TestBind_AppendEventInvalidAppendDistinctFromMissingTarget(t *testing.T) {
	t.Parallel()

	appendRaw := mustCallAppendEvent(t, fakeRecordingsRoot{
		appendEvent: func(_ recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
			return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
		},
	})
	statusRaw := mustCallQueryStatus(t, fakeRecordingsRoot{
		queryStatus: func(_ recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
		},
	})

	appendEnvelope := assertTypedToolErrorEnvelope(t, appendRaw, "recording.append.invalid", false, "")
	statusEnvelope := assertTypedToolErrorEnvelope(
		t,
		statusRaw,
		"recording.target.missing",
		false,
		missingRecordingID,
	)
	if appendEnvelope.Code == statusEnvelope.Code {
		t.Fatalf("append and status error codes should differ: %#v vs %#v", appendEnvelope, statusEnvelope)
	}
}

func TestBind_AppendEventInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolAppendEvent,
		json.RawMessage(`{"event":`),
	)
	if err != nil {
		t.Fatalf("CallTool(append_event) transport error = %v, want typed tool response", err)
	}
	assertAppendBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake recordings root was invoked for invalid JSON decode")
	}
}

func TestBind_AppendEventInvalidRecordedAtReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolAppendEvent,
		json.RawMessage(`{"event":{"id":"`+testAppendEventID+`","recordedAt":"not-a-timestamp","kind":"WORK_REQUEST","payload":"{}"}}`),
	)
	if err != nil {
		t.Fatalf("CallTool(append_event) transport error = %v, want typed tool response", err)
	}
	assertAppendBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake recordings root was invoked for invalid recordedAt decode")
	}
}

func mustCallAppendEvent(t *testing.T, fake fakeRecordingsRoot) json.RawMessage {
	t.Helper()

	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolAppendEvent,
		testAppendEventInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(append_event) transport error = %v", err)
	}
	return raw
}

func mustCallQueryStatus(t *testing.T, fake fakeRecordingsRoot) json.RawMessage {
	t.Helper()

	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+missingRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v", err)
	}
	return raw
}

func testAppendCanonicalEvent() recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID(testAppendEventID),
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "session-mcp-001"},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       recordings.CanonicalEventKind("WORK_REQUEST"),
		Payload:    `{}`,
	}
}

func testAppendEventInputJSON() json.RawMessage {
	return json.RawMessage(`{"event":{"id":"` + testAppendEventID +
		`","scope":{"factorySessionId":"session-mcp-001"},"recordedAt":"` +
		testAppendCanonicalEvent().RecordedAt.Format(time.RFC3339Nano) +
		`","kind":"WORK_REQUEST","payload":"{}"}}`)
}

func assertAppendBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "decode append event input") {
		t.Fatalf("error.message = %q, want decode append event input context", envelope.Message)
	}
}
