// Package session defines the closed, validated L1 V0 ACP compatibility
// shapes for session lifecycle, configuration, prompting, updates,
// cancellation, load, resume, and permission correlation. Every exported
// Validate function is a pure decode-time gate: malformed required fields,
// non-text prompt or update content, and non-empty client-supplied MCP
// server requests are rejected here, before any downstream dispatch. The
// resulting values carry no provider or service vocabulary, only the P0
// text-first ACP subset. It is internal to pkg/transports/acp; callers use
// the package root's exported operations instead of this package directly.
package session

import (
	"errors"
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"
)

// ErrUnsupportedContent marks a prompt or update content variant this L1 V0
// transport does not implement. Only text content is accepted; image,
// audio, resource, and resource-link variants are documented scope cuts
// rejected before dispatch, not silently accepted or dropped.
var ErrUnsupportedContent = errors.New("acp: unsupported content variant")

// ErrUnsupportedUpdate marks a session/update variant this L1 V0 transport
// does not implement. Tool call, plan, available-commands, and mode updates
// are declared no-output scope cuts (final-proposal.md §6.2); they are
// rejected here rather than silently accepted and later dropped.
var ErrUnsupportedUpdate = errors.New("acp: unsupported session update variant")

// SessionID is the closed L1 V0 session identity type, independent of the
// vendored SDK's acpsdk.SessionId so this package's public shapes carry no
// third-party wire type in their surface.
type SessionID string

// TextContent is the sole P0 text-first content shape accepted at prompt
// and update content boundaries.
type TextContent struct {
	Text string
}

func validateTextBlock(block acpsdk.ContentBlock) (TextContent, error) {
	switch {
	case block.Text != nil:
		if block.Text.Text == "" {
			return TextContent{}, errors.New("acp: text content must not be empty")
		}
		return TextContent{Text: block.Text.Text}, nil
	case block.Image != nil:
		return TextContent{}, fmt.Errorf("%w: image", ErrUnsupportedContent)
	case block.Audio != nil:
		return TextContent{}, fmt.Errorf("%w: audio", ErrUnsupportedContent)
	case block.ResourceLink != nil:
		return TextContent{}, fmt.Errorf("%w: resource_link", ErrUnsupportedContent)
	case block.Resource != nil:
		return TextContent{}, fmt.Errorf("%w: resource", ErrUnsupportedContent)
	default:
		return TextContent{}, fmt.Errorf("%w: empty content block", ErrUnsupportedContent)
	}
}

func isAbsolutePath(p string) bool {
	if p == "" {
		return false
	}
	if p[0] == '/' {
		return true
	}
	// A Windows drive-absolute path: "C:\..." or "C:/...".
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		c := p[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}

// NewSessionParams is the closed L1 V0 shape of a validated session/new
// request. Non-empty client-supplied MCP servers are an explicit rejection,
// not a silently ignored field: L1 V0 implements no MCP passthrough.
type NewSessionParams struct {
	Cwd                   string
	AdditionalDirectories []string
}

// ValidateNewSession validates a session/new request against the L1 V0
// compatibility boundary.
func ValidateNewSession(req acpsdk.NewSessionRequest) (NewSessionParams, error) {
	if req.Cwd == "" {
		return NewSessionParams{}, errors.New("acp: cwd is required")
	}
	if !isAbsolutePath(req.Cwd) {
		return NewSessionParams{}, errors.New("acp: cwd must be an absolute path")
	}
	if len(req.McpServers) > 0 {
		return NewSessionParams{}, errors.New("acp: client-supplied MCP servers are not supported in L1 V0")
	}
	return NewSessionParams{
		Cwd:                   req.Cwd,
		AdditionalDirectories: append([]string(nil), req.AdditionalDirectories...),
	}, nil
}

// LoadSessionParams is the closed L1 V0 shape of a validated session/load
// or session/resume request.
type LoadSessionParams struct {
	SessionID SessionID
	NewSessionParams
}

// ValidateLoadSession validates a session/load request against the L1 V0
// compatibility boundary.
func ValidateLoadSession(req acpsdk.LoadSessionRequest) (LoadSessionParams, error) {
	if req.SessionId == "" {
		return LoadSessionParams{}, errors.New("acp: sessionId is required")
	}
	base, err := ValidateNewSession(acpsdk.NewSessionRequest{
		Cwd:                   req.Cwd,
		AdditionalDirectories: req.AdditionalDirectories,
		McpServers:            req.McpServers,
	})
	if err != nil {
		return LoadSessionParams{}, err
	}
	return LoadSessionParams{SessionID: SessionID(req.SessionId), NewSessionParams: base}, nil
}

// ValidateResumeSession validates a session/resume request against the L1
// V0 compatibility boundary.
func ValidateResumeSession(req acpsdk.ResumeSessionRequest) (LoadSessionParams, error) {
	if req.SessionId == "" {
		return LoadSessionParams{}, errors.New("acp: sessionId is required")
	}
	base, err := ValidateNewSession(acpsdk.NewSessionRequest{
		Cwd:                   req.Cwd,
		AdditionalDirectories: req.AdditionalDirectories,
		McpServers:            req.McpServers,
	})
	if err != nil {
		return LoadSessionParams{}, err
	}
	return LoadSessionParams{SessionID: SessionID(req.SessionId), NewSessionParams: base}, nil
}

// CancelParams is the closed L1 V0 shape of a validated session/cancel
// notification.
type CancelParams struct {
	SessionID SessionID
}

// ValidateCancel validates a session/cancel notification against the L1 V0
// compatibility boundary.
func ValidateCancel(req acpsdk.CancelNotification) (CancelParams, error) {
	if req.SessionId == "" {
		return CancelParams{}, errors.New("acp: sessionId is required")
	}
	return CancelParams{SessionID: SessionID(req.SessionId)}, nil
}

// ConfigOptionValue is the closed L1 V0 shape of a validated
// session/set_config_option request: a session and config-option identity
// plus exactly one of a boolean or select value-id payload.
type ConfigOptionValue struct {
	SessionID SessionID
	ConfigID  string
	Boolean   *bool
	ValueID   *string
}

// ValidateSetConfigOption validates a session/set_config_option request
// against the L1 V0 compatibility boundary.
func ValidateSetConfigOption(req acpsdk.SetSessionConfigOptionRequest) (ConfigOptionValue, error) {
	switch {
	case req.Boolean != nil:
		if req.Boolean.SessionId == "" {
			return ConfigOptionValue{}, errors.New("acp: sessionId is required")
		}
		if req.Boolean.ConfigId == "" {
			return ConfigOptionValue{}, errors.New("acp: configId is required")
		}
		value := req.Boolean.Value
		return ConfigOptionValue{
			SessionID: SessionID(req.Boolean.SessionId),
			ConfigID:  string(req.Boolean.ConfigId),
			Boolean:   &value,
		}, nil
	case req.ValueId != nil:
		if req.ValueId.SessionId == "" {
			return ConfigOptionValue{}, errors.New("acp: sessionId is required")
		}
		if req.ValueId.ConfigId == "" {
			return ConfigOptionValue{}, errors.New("acp: configId is required")
		}
		if req.ValueId.Value == "" {
			return ConfigOptionValue{}, errors.New("acp: value is required")
		}
		value := string(req.ValueId.Value)
		return ConfigOptionValue{
			SessionID: SessionID(req.ValueId.SessionId),
			ConfigID:  string(req.ValueId.ConfigId),
			ValueID:   &value,
		}, nil
	default:
		return ConfigOptionValue{}, errors.New("acp: exactly one of a boolean or value-id payload is required")
	}
}

// PromptTurn is the closed L1 V0 shape of a validated session/prompt
// request: a session identity and an ordered, non-empty sequence of
// text-first content blocks.
type PromptTurn struct {
	SessionID SessionID
	MessageID string
	Content   []TextContent
}

// ValidatePrompt validates a session/prompt request against the L1 V0
// compatibility boundary. Every content block must be text; the first
// non-text block rejects the whole request before dispatch.
func ValidatePrompt(req acpsdk.PromptRequest) (PromptTurn, error) {
	if req.SessionId == "" {
		return PromptTurn{}, errors.New("acp: sessionId is required")
	}
	if len(req.Prompt) == 0 {
		return PromptTurn{}, errors.New("acp: prompt must contain at least one content block")
	}

	content := make([]TextContent, 0, len(req.Prompt))
	for i, block := range req.Prompt {
		text, err := validateTextBlock(block)
		if err != nil {
			return PromptTurn{}, fmt.Errorf("acp: prompt block %d: %w", i, err)
		}
		content = append(content, text)
	}

	turn := PromptTurn{SessionID: SessionID(req.SessionId), Content: content}
	if req.MessageId != nil {
		turn.MessageID = *req.MessageId
	}
	return turn, nil
}

// TextUpdateKind is the closed set of L1 V0 outbound session/update
// variants. Tool call, plan, available-commands, and mode updates are
// declared no-output scope cuts (final-proposal.md §6.2), so they are
// rejected by ValidateSessionUpdate rather than represented here.
type TextUpdateKind string

const (
	TextUpdateUserMessageChunk  TextUpdateKind = "user_message_chunk"
	TextUpdateAgentMessageChunk TextUpdateKind = "agent_message_chunk"
	TextUpdateAgentThoughtChunk TextUpdateKind = "agent_thought_chunk"
	TextUpdateUsage             TextUpdateKind = "usage_update"
	TextUpdateSessionInfo       TextUpdateKind = "session_info_update"
	TextUpdateConfigOption      TextUpdateKind = "config_option_update"
)

// TextUpdate is the closed L1 V0 shape of a validated session/update
// notification. Content and MessageID are populated only for the three
// streamed-message kinds; the usage, session-info, and config-option kinds
// are accepted as bare, content-free markers at this compatibility layer.
type TextUpdate struct {
	Kind      TextUpdateKind
	Content   *TextContent
	MessageID string
}

func chunkUpdate(kind TextUpdateKind, content acpsdk.ContentBlock, messageID *string) (TextUpdate, error) {
	text, err := validateTextBlock(content)
	if err != nil {
		return TextUpdate{}, fmt.Errorf("acp: %s: %w", kind, err)
	}
	update := TextUpdate{Kind: kind, Content: &text}
	if messageID != nil {
		update.MessageID = *messageID
	}
	return update, nil
}

// ValidateSessionUpdate validates a session/update notification's Update
// payload against the L1 V0 compatibility boundary.
func ValidateSessionUpdate(update acpsdk.SessionUpdate) (TextUpdate, error) {
	switch {
	case update.UserMessageChunk != nil:
		return chunkUpdate(TextUpdateUserMessageChunk, update.UserMessageChunk.Content, update.UserMessageChunk.MessageId)
	case update.AgentMessageChunk != nil:
		return chunkUpdate(TextUpdateAgentMessageChunk, update.AgentMessageChunk.Content, update.AgentMessageChunk.MessageId)
	case update.AgentThoughtChunk != nil:
		return chunkUpdate(TextUpdateAgentThoughtChunk, update.AgentThoughtChunk.Content, update.AgentThoughtChunk.MessageId)
	case update.UsageUpdate != nil:
		return TextUpdate{Kind: TextUpdateUsage}, nil
	case update.SessionInfoUpdate != nil:
		return TextUpdate{Kind: TextUpdateSessionInfo}, nil
	case update.ConfigOptionUpdate != nil:
		return TextUpdate{Kind: TextUpdateConfigOption}, nil
	case update.ToolCall != nil:
		return TextUpdate{}, fmt.Errorf("%w: tool_call", ErrUnsupportedUpdate)
	case update.ToolCallUpdate != nil:
		return TextUpdate{}, fmt.Errorf("%w: tool_call_update", ErrUnsupportedUpdate)
	case update.Plan != nil:
		return TextUpdate{}, fmt.Errorf("%w: plan", ErrUnsupportedUpdate)
	case update.PlanUpdate != nil:
		return TextUpdate{}, fmt.Errorf("%w: plan_update", ErrUnsupportedUpdate)
	case update.PlanRemoved != nil:
		return TextUpdate{}, fmt.Errorf("%w: plan_removed", ErrUnsupportedUpdate)
	case update.AvailableCommandsUpdate != nil:
		return TextUpdate{}, fmt.Errorf("%w: available_commands_update", ErrUnsupportedUpdate)
	case update.CurrentModeUpdate != nil:
		return TextUpdate{}, fmt.Errorf("%w: current_mode_update", ErrUnsupportedUpdate)
	default:
		return TextUpdate{}, fmt.Errorf("%w: unrecognized session update", ErrUnsupportedUpdate)
	}
}

// PermissionCorrelation is the closed L1 V0 shape correlating a
// session/request_permission request to the session and tool call it
// concerns. It deliberately excludes the tool call's raw input, raw output,
// content, and locations: those carry arbitrary payload data this
// compatibility layer does not need in order to correlate the request, and
// keeping them out avoids re-exposing unsanitized tool payloads here.
type PermissionCorrelation struct {
	SessionID  SessionID
	ToolCallID string
	OptionIDs  []string
}

// ValidatePermissionCorrelation validates a session/request_permission
// request against the L1 V0 compatibility boundary.
func ValidatePermissionCorrelation(req acpsdk.RequestPermissionRequest) (PermissionCorrelation, error) {
	if req.SessionId == "" {
		return PermissionCorrelation{}, errors.New("acp: sessionId is required")
	}
	if req.ToolCall.ToolCallId == "" {
		return PermissionCorrelation{}, errors.New("acp: toolCall.toolCallId is required")
	}
	if len(req.Options) == 0 {
		return PermissionCorrelation{}, errors.New("acp: options must not be empty")
	}

	optionIDs := make([]string, 0, len(req.Options))
	for i, opt := range req.Options {
		if opt.OptionId == "" {
			return PermissionCorrelation{}, fmt.Errorf("acp: option %d optionId is required", i)
		}
		optionIDs = append(optionIDs, string(opt.OptionId))
	}

	return PermissionCorrelation{
		SessionID:  SessionID(req.SessionId),
		ToolCallID: string(req.ToolCall.ToolCallId),
		OptionIDs:  optionIDs,
	}, nil
}
