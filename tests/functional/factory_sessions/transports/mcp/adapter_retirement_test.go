package mcp_adapter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

// TestRetiredFactorySessionHandlersKeepTheirAdapterErrors exercises the
// retained adapter bindings without publishing the retired tools through MCP.
// The public server catalog and its removal guarantee are covered separately
// by the transport smoke test.
func TestRetiredFactorySessionHandlersKeepTheirAdapterErrors(t *testing.T) {
	t.Parallel()
	operation := factorysessionmcp.BindToolOperation(nil, nil, nil, nil, nil, "", nil)
	tests := []struct {
		name  string
		input string
	}{
		{name: factorysessionmcp.ToolListSessions, input: `{}`},
		{name: factorysessionmcp.ToolValidateSource, input: `{}`},
		{name: factorysessionmcp.ToolStartSync, input: `{}`},
		{name: factorysessionmcp.ToolStartAsync, input: `{}`},
		{name: factorysessionmcp.ToolGetSession, input: `{"sessionId":"session-1"}`},
		{name: factorysessionmcp.ToolGetResult, input: `{"sessionId":"session-1"}`},
		{name: factorysessionmcp.ToolListDispatches, input: `{"sessionId":"session-1"}`},
		{name: factorysessionmcp.ToolListArtifacts, input: `{"sessionId":"session-1"}`},
		{name: factorysessionmcp.ToolControl, input: `{"sessionId":"session-1","operation":"pause"}`},
		{name: factorysessionmcp.ToolReadEvents, input: `{"sessionId":"session-1"}`},
		{name: factorysessionmcp.ToolSubagent, input: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			raw, err := operation(context.Background(), test.name, json.RawMessage(test.input))
			if err != nil {
				t.Fatalf("adapter call returned transport error: %v", err)
			}
			var response struct {
				Error *json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatalf("decode adapter response %s: %v", raw, err)
			}
			if response.Error == nil {
				t.Fatalf("adapter response = %s, want structured unavailable or validation error", raw)
			}
		})
	}
}

func TestRetiredFactorySessionCallToolRejectsMissingContextAndUnknownName(t *testing.T) {
	t.Parallel()
	_, err := factorysessionmcp.CallTool(nil, nil, nil, nil, factorysessionmcp.ToolGetSession, json.RawMessage(`{}`), nil, nil, "", nil)
	if err == nil {
		t.Fatal("CallTool with nil context succeeded")
	}
	_, err = factorysessionmcp.CallTool(context.Background(), nil, nil, nil, "you.factory_session.retired_unknown", json.RawMessage(`{}`), nil, nil, "", nil)
	if err == nil {
		t.Fatal("CallTool with unknown tool name succeeded")
	}
}

func TestRetiredFactorySessionGetMapsSuccessfulRootRead(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-js-run-n-001"
	operation := factorysessionmcp.BindToolOperation(
		functionalSessionRoot{sessionID: sessionID}, nil, nil, nil, nil, "", nil,
	)
	raw, err := operation(
		context.Background(),
		factorysessionmcp.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+sessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("adapter call returned transport error: %v", err)
	}
	var response struct {
		Result struct {
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		} `json:"result"`
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode adapter response %s: %v", raw, err)
	}
	if response.Error != nil {
		t.Fatalf("adapter response = %s, want successful session read", raw)
	}
	if response.Result.SessionID != sessionID || response.Result.Status != string(factorysessions.LifecycleStatusRunning) {
		t.Fatalf("mapped session = %#v, want id %q and RUNNING status", response.Result, sessionID)
	}
}

type functionalSessionRoot struct {
	factorysessionmcp.DurableExecution
	sessionID string
}

func (root functionalSessionRoot) GetSession(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	if sessionID != root.sessionID {
		return factorysessions.SessionReadResult{}, fmt.Errorf("unexpected session ID %q", sessionID)
	}
	return factorysessions.SessionReadResult{
		SessionID: root.sessionID,
		Status:    factorysessions.LifecycleStatusRunning,
	}, nil
}
