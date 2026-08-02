package session

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func textBlock(text string) acpsdk.ContentBlock {
	block := acpsdk.ContentBlock{}
	if err := json.Unmarshal([]byte(`{"type":"text","text":`+jsonString(text)+`}`), &block); err != nil {
		panic(err)
	}
	return block
}

func imageBlock() acpsdk.ContentBlock {
	block := acpsdk.ContentBlock{}
	if err := json.Unmarshal([]byte(`{"type":"image","data":"aGVsbG8=","mimeType":"image/png"}`), &block); err != nil {
		panic(err)
	}
	return block
}

func resourceLinkBlock() acpsdk.ContentBlock {
	block := acpsdk.ContentBlock{}
	if err := json.Unmarshal([]byte(`{"type":"resource_link","uri":"file:///a.py","name":"a.py"}`), &block); err != nil {
		panic(err)
	}
	return block
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func roundTrip[T any](t *testing.T, value T) T {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded T
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if string(reencoded) != string(encoded) {
		t.Fatalf("round trip drifted: first=%s second=%s", encoded, reencoded)
	}
	return decoded
}

func TestValidateNewSession(t *testing.T) {
	t.Run("accepts an absolute cwd with no mcp servers", func(t *testing.T) {
		got, err := ValidateNewSession(acpsdk.NewSessionRequest{
			Cwd:                   "/home/user/project",
			AdditionalDirectories: []string{"/home/user/other"},
			McpServers:            []acpsdk.McpServer{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := NewSessionParams{Cwd: "/home/user/project", AdditionalDirectories: []string{"/home/user/other"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		roundTrip(t, got)
	})

	t.Run("rejects a relative cwd", func(t *testing.T) {
		_, err := ValidateNewSession(acpsdk.NewSessionRequest{Cwd: "relative/path"})
		if err == nil {
			t.Fatalf("expected an error for a relative cwd")
		}
	})

	t.Run("rejects an empty cwd", func(t *testing.T) {
		_, err := ValidateNewSession(acpsdk.NewSessionRequest{Cwd: ""})
		if err == nil {
			t.Fatalf("expected an error for an empty cwd")
		}
	})

	t.Run("rejects non-empty client-supplied mcp servers", func(t *testing.T) {
		_, err := ValidateNewSession(acpsdk.NewSessionRequest{
			Cwd:        "/home/user/project",
			McpServers: []acpsdk.McpServer{{Stdio: &acpsdk.McpServerStdio{Name: "filesystem", Command: "/path/to/mcp-server"}}},
		})
		if err == nil {
			t.Fatalf("expected non-empty mcpServers to be rejected")
		}
	})

	t.Run("accepts a windows drive-absolute cwd", func(t *testing.T) {
		if _, err := ValidateNewSession(acpsdk.NewSessionRequest{Cwd: `C:\workspace\project`}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateLoadSession(t *testing.T) {
	t.Run("accepts a valid load request", func(t *testing.T) {
		got, err := ValidateLoadSession(acpsdk.LoadSessionRequest{
			SessionId: "sess-1",
			Cwd:       "/home/user/project",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := LoadSessionParams{SessionID: "sess-1", NewSessionParams: NewSessionParams{Cwd: "/home/user/project"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		roundTrip(t, got)
	})

	t.Run("rejects a missing sessionId", func(t *testing.T) {
		_, err := ValidateLoadSession(acpsdk.LoadSessionRequest{Cwd: "/home/user/project"})
		if err == nil {
			t.Fatalf("expected an error for a missing sessionId")
		}
	})

	t.Run("rejects non-empty mcp servers", func(t *testing.T) {
		_, err := ValidateLoadSession(acpsdk.LoadSessionRequest{
			SessionId:  "sess-1",
			Cwd:        "/home/user/project",
			McpServers: []acpsdk.McpServer{{Stdio: &acpsdk.McpServerStdio{Name: "filesystem", Command: "/path/to/mcp-server"}}},
		})
		if err == nil {
			t.Fatalf("expected non-empty mcpServers to be rejected")
		}
	})
}

func TestValidateResumeSession(t *testing.T) {
	t.Run("accepts a valid resume request", func(t *testing.T) {
		got, err := ValidateResumeSession(acpsdk.ResumeSessionRequest{
			SessionId: "sess-1",
			Cwd:       "/home/user/project",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := LoadSessionParams{SessionID: "sess-1", NewSessionParams: NewSessionParams{Cwd: "/home/user/project"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("rejects a missing cwd", func(t *testing.T) {
		_, err := ValidateResumeSession(acpsdk.ResumeSessionRequest{SessionId: "sess-1"})
		if err == nil {
			t.Fatalf("expected an error for a missing cwd")
		}
	})

	t.Run("rejects non-empty mcp servers", func(t *testing.T) {
		_, err := ValidateResumeSession(acpsdk.ResumeSessionRequest{
			SessionId:  "sess-1",
			Cwd:        "/home/user/project",
			McpServers: []acpsdk.McpServer{{Stdio: &acpsdk.McpServerStdio{Name: "filesystem", Command: "/path/to/mcp-server"}}},
		})
		if err == nil {
			t.Fatalf("expected non-empty mcpServers to be rejected")
		}
	})
}

func TestValidateCancel(t *testing.T) {
	got, err := ValidateCancel(acpsdk.CancelNotification{SessionId: "sess-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("got %+v", got)
	}
	roundTrip(t, got)

	if _, err := ValidateCancel(acpsdk.CancelNotification{}); err == nil {
		t.Fatalf("expected an error for a missing sessionId")
	}
}

func TestValidateSetConfigOption(t *testing.T) {
	t.Run("accepts a boolean payload", func(t *testing.T) {
		req := acpsdk.SetSessionConfigOptionRequest{
			Boolean: &acpsdk.SetSessionConfigOptionBoolean{SessionId: "sess-1", ConfigId: "reasoning", Value: true},
		}
		got, err := ValidateSetConfigOption(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.SessionID != "sess-1" || got.ConfigID != "reasoning" || got.Boolean == nil || *got.Boolean != true {
			t.Fatalf("got %+v", got)
		}
		if got.ValueID != nil {
			t.Fatalf("expected no value id for a boolean payload, got %+v", got.ValueID)
		}
		roundTrip(t, got)
	})

	t.Run("accepts a value-id payload", func(t *testing.T) {
		req := acpsdk.SetSessionConfigOptionRequest{
			ValueId: &acpsdk.SetSessionConfigOptionValueId{SessionId: "sess-1", ConfigId: "target", Value: "factory:@you/review"},
		}
		got, err := ValidateSetConfigOption(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ValueID == nil || *got.ValueID != "factory:@you/review" {
			t.Fatalf("got %+v", got)
		}
		if got.Boolean != nil {
			t.Fatalf("expected no boolean for a value-id payload, got %+v", got.Boolean)
		}
	})

	t.Run("rejects a request with neither variant set", func(t *testing.T) {
		if _, err := ValidateSetConfigOption(acpsdk.SetSessionConfigOptionRequest{}); err == nil {
			t.Fatalf("expected an error when neither variant is set")
		}
	})

	t.Run("rejects a missing sessionId", func(t *testing.T) {
		req := acpsdk.SetSessionConfigOptionRequest{
			Boolean: &acpsdk.SetSessionConfigOptionBoolean{ConfigId: "reasoning", Value: true},
		}
		if _, err := ValidateSetConfigOption(req); err == nil {
			t.Fatalf("expected an error for a missing sessionId")
		}
	})

	t.Run("rejects an empty value id", func(t *testing.T) {
		req := acpsdk.SetSessionConfigOptionRequest{
			ValueId: &acpsdk.SetSessionConfigOptionValueId{SessionId: "sess-1", ConfigId: "target", Value: ""},
		}
		if _, err := ValidateSetConfigOption(req); err == nil {
			t.Fatalf("expected an error for an empty value")
		}
	})
}

func TestValidatePrompt(t *testing.T) {
	t.Run("accepts text-only content and preserves order", func(t *testing.T) {
		messageID := "msg-1"
		req := acpsdk.PromptRequest{
			SessionId: "sess-1",
			MessageId: &messageID,
			Prompt:    []acpsdk.ContentBlock{textBlock("first"), textBlock("second")},
		}
		got, err := ValidatePrompt(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := PromptTurn{
			SessionID: "sess-1",
			MessageID: "msg-1",
			Content:   []TextContent{{Text: "first"}, {Text: "second"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		roundTrip(t, got)
	})

	t.Run("rejects a missing sessionId", func(t *testing.T) {
		req := acpsdk.PromptRequest{Prompt: []acpsdk.ContentBlock{textBlock("hi")}}
		if _, err := ValidatePrompt(req); err == nil {
			t.Fatalf("expected an error for a missing sessionId")
		}
	})

	t.Run("rejects an empty prompt", func(t *testing.T) {
		req := acpsdk.PromptRequest{SessionId: "sess-1", Prompt: []acpsdk.ContentBlock{}}
		if _, err := ValidatePrompt(req); err == nil {
			t.Fatalf("expected an error for an empty prompt")
		}
	})

	t.Run("rejects image content before dispatch", func(t *testing.T) {
		req := acpsdk.PromptRequest{SessionId: "sess-1", Prompt: []acpsdk.ContentBlock{imageBlock()}}
		_, err := ValidatePrompt(req)
		if err == nil {
			t.Fatalf("expected an error for image content")
		}
		if !errors.Is(err, ErrUnsupportedContent) {
			t.Fatalf("expected ErrUnsupportedContent, got %v", err)
		}
	})

	t.Run("rejects resource-link content before dispatch", func(t *testing.T) {
		req := acpsdk.PromptRequest{SessionId: "sess-1", Prompt: []acpsdk.ContentBlock{resourceLinkBlock()}}
		_, err := ValidatePrompt(req)
		if !errors.Is(err, ErrUnsupportedContent) {
			t.Fatalf("expected ErrUnsupportedContent, got %v", err)
		}
	})

	t.Run("rejects a mixed text and non-text prompt as a whole", func(t *testing.T) {
		req := acpsdk.PromptRequest{SessionId: "sess-1", Prompt: []acpsdk.ContentBlock{textBlock("ok"), imageBlock()}}
		if _, err := ValidatePrompt(req); !errors.Is(err, ErrUnsupportedContent) {
			t.Fatalf("expected ErrUnsupportedContent, got %v", err)
		}
	})
}

func TestValidateSessionUpdate(t *testing.T) {
	t.Run("accepts text-first update kinds", func(t *testing.T) {
		cases := []struct {
			name   string
			update acpsdk.SessionUpdate
			want   TextUpdate
		}{
			{
				name:   "user message chunk",
				update: acpsdk.SessionUpdate{UserMessageChunk: &acpsdk.SessionUpdateUserMessageChunk{Content: textBlock("hi")}},
				want:   TextUpdate{Kind: TextUpdateUserMessageChunk, Content: &TextContent{Text: "hi"}},
			},
			{
				name:   "agent message chunk",
				update: acpsdk.SessionUpdate{AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{Content: textBlock("hello")}},
				want:   TextUpdate{Kind: TextUpdateAgentMessageChunk, Content: &TextContent{Text: "hello"}},
			},
			{
				name:   "agent thought chunk",
				update: acpsdk.SessionUpdate{AgentThoughtChunk: &acpsdk.SessionUpdateAgentThoughtChunk{Content: textBlock("thinking")}},
				want:   TextUpdate{Kind: TextUpdateAgentThoughtChunk, Content: &TextContent{Text: "thinking"}},
			},
			{
				name:   "usage update",
				update: acpsdk.SessionUpdate{UsageUpdate: &acpsdk.SessionUsageUpdate{Size: 100, Used: 10}},
				want:   TextUpdate{Kind: TextUpdateUsage},
			},
			{
				name:   "session info update",
				update: acpsdk.SessionUpdate{SessionInfoUpdate: &acpsdk.SessionSessionInfoUpdate{}},
				want:   TextUpdate{Kind: TextUpdateSessionInfo},
			},
			{
				name:   "config option update",
				update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{}},
				want:   TextUpdate{Kind: TextUpdateConfigOption},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := ValidateSessionUpdate(tc.update)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got %+v, want %+v", got, tc.want)
				}
				roundTrip(t, got)
			})
		}
	})

	t.Run("rejects declared no-output update kinds", func(t *testing.T) {
		cases := []struct {
			name   string
			update acpsdk.SessionUpdate
		}{
			{"tool call", acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{ToolCallId: "call-1", Title: "x"}}},
			{"tool call update", acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{ToolCallId: "call-1"}}},
			{"plan", acpsdk.SessionUpdate{Plan: &acpsdk.SessionUpdatePlan{}}},
			{"available commands", acpsdk.SessionUpdate{AvailableCommandsUpdate: &acpsdk.SessionAvailableCommandsUpdate{}}},
			{"current mode", acpsdk.SessionUpdate{CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{}}},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := ValidateSessionUpdate(tc.update)
				if !errors.Is(err, ErrUnsupportedUpdate) {
					t.Fatalf("expected ErrUnsupportedUpdate, got %v", err)
				}
			})
		}
	})

	t.Run("rejects a message chunk carrying non-text content", func(t *testing.T) {
		update := acpsdk.SessionUpdate{AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{Content: imageBlock()}}
		if _, err := ValidateSessionUpdate(update); !errors.Is(err, ErrUnsupportedContent) {
			t.Fatalf("expected ErrUnsupportedContent, got %v", err)
		}
	})

	t.Run("rejects an empty update", func(t *testing.T) {
		if _, err := ValidateSessionUpdate(acpsdk.SessionUpdate{}); !errors.Is(err, ErrUnsupportedUpdate) {
			t.Fatalf("expected ErrUnsupportedUpdate for an empty update, got %v", err)
		}
	})
}

func TestValidatePermissionCorrelation(t *testing.T) {
	t.Run("preserves session and tool call correlation identities", func(t *testing.T) {
		req := acpsdk.RequestPermissionRequest{
			SessionId: "sess-1",
			ToolCall:  acpsdk.ToolCallUpdate{ToolCallId: "call-1"},
			Options: []acpsdk.PermissionOption{
				{OptionId: "allow", Name: "Allow", Kind: acpsdk.PermissionOptionKindAllowOnce},
				{OptionId: "deny", Name: "Deny", Kind: acpsdk.PermissionOptionKindRejectOnce},
			},
		}
		got, err := ValidatePermissionCorrelation(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := PermissionCorrelation{SessionID: "sess-1", ToolCallID: "call-1", OptionIDs: []string{"allow", "deny"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		roundTrip(t, got)
	})

	t.Run("rejects a missing tool call id", func(t *testing.T) {
		req := acpsdk.RequestPermissionRequest{
			SessionId: "sess-1",
			Options:   []acpsdk.PermissionOption{{OptionId: "allow", Name: "Allow"}},
		}
		if _, err := ValidatePermissionCorrelation(req); err == nil {
			t.Fatalf("expected an error for a missing tool call id")
		}
	})

	t.Run("rejects empty options", func(t *testing.T) {
		req := acpsdk.RequestPermissionRequest{
			SessionId: "sess-1",
			ToolCall:  acpsdk.ToolCallUpdate{ToolCallId: "call-1"},
		}
		if _, err := ValidatePermissionCorrelation(req); err == nil {
			t.Fatalf("expected an error for empty options")
		}
	})

	t.Run("does not carry raw tool call payload through", func(t *testing.T) {
		req := acpsdk.RequestPermissionRequest{
			SessionId: "sess-1",
			ToolCall: acpsdk.ToolCallUpdate{
				ToolCallId: "call-1",
				RawInput:   map[string]any{"command": "rm -rf /"},
				RawOutput:  "secret output",
			},
			Options: []acpsdk.PermissionOption{{OptionId: "allow", Name: "Allow"}},
		}
		got, err := ValidatePermissionCorrelation(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if strings.Contains(string(encoded), "rm -rf") || strings.Contains(string(encoded), "secret output") {
			t.Fatalf("expected raw tool call payload to be excluded, got %s", encoded)
		}
	})
}
