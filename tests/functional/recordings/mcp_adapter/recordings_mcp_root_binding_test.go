package mcp_adapter_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

// TestRecordingsMCPToolsUseTheAcceptedServiceRoot exercises the current
// Recordings MCP catalog through its supported Go binding and canonical root.
// The scenario appends a Factory Event, reads the resulting detached status,
// and verifies the remaining read tools return their typed unavailable errors
// when no durable artifact has been published.
func TestRecordingsMCPToolsUseTheAcceptedServiceRoot(t *testing.T) {
	t.Parallel()

	service := functionalRecordingsRoot(t)
	operation := mcprecording.BindToolOperation(service)
	ctx := context.Background()

	appendRaw, err := operation(ctx, mcprecording.ToolAppendEvent, json.RawMessage(`{
		"event": {
			"id": "mcp-functional-event",
			"sequence": 0,
			"scope": {"factorySessionId": "mcp-functional-session"},
			"recordedAt": "2026-09-27T12:00:00Z",
			"kind": "WORK_REQUEST",
			"payload": "{}"
		}
	}`))
	if err != nil {
		t.Fatalf("CallTool(append_event) error = %v", err)
	}
	var appended mcprecording.ToolResponse[recordings.AppendRecordedEventResult]
	if err := json.Unmarshal(appendRaw, &appended); err != nil {
		t.Fatalf("decode append_event response: %v", err)
	}
	if appended.Error != nil || appended.Result == nil || appended.Result.Event.ID != "mcp-functional-event" {
		t.Fatalf("append_event response = %#v, want appended canonical event", appended)
	}

	statusRaw, err := operation(ctx, mcprecording.ToolQueryStatus, json.RawMessage(`{"recordingId":"mcp-functional-session"}`))
	if err != nil {
		t.Fatalf("CallTool(query_status) error = %v", err)
	}
	var status mcprecording.ToolResponse[recordings.RecordingStatusResult]
	if err := json.Unmarshal(statusRaw, &status); err != nil {
		t.Fatalf("decode query_status response: %v", err)
	}
	if status.Error == nil || status.Error.Code == "" {
		t.Fatalf("query_status response = %#v, want typed missing-recording outcome", status)
	}

	for _, check := range []struct {
		name  string
		input string
	}{
		{mcprecording.ToolQueryHistory, `{"recordingId":"mcp-functional-session"}`},
		{mcprecording.ToolLoadReplay, `{"recordingId":"mcp-functional-session"}`},
		{mcprecording.ToolReadPortableArtifact, `{"reference":"missing.replay.json"}`},
	} {
		t.Run(check.name, func(t *testing.T) {
			raw, callErr := operation(ctx, check.name, json.RawMessage(check.input))
			if callErr != nil {
				t.Fatalf("CallTool(%s) transport error = %v", check.name, callErr)
			}
			var envelope struct {
				Error *mcprecording.ToolErrorEnvelope `json:"error"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("decode %s response: %v", check.name, err)
			}
			if envelope.Error == nil || envelope.Error.Code == "" {
				t.Fatalf("%s response = %s, want typed service outcome", check.name, raw)
			}
		})
	}
}

func functionalRecordingsRoot(t *testing.T) recordings.Service {
	t.Helper()
	ledger := recordingswire.NewRuntimeLedger(nil, time.Now, "functional-recordings-mcp", nil)
	if ledger == nil {
		t.Fatal("NewRuntimeLedger returned nil")
	}
	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		ledger,
		recordingswire.NewProjectionService(),
		nil,
		func(string, []byte) error { return nil },
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewServiceWithProjectionAndEffects: %v", err)
	}
	return service
}
