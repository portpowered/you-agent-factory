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
	"bytes"
	"encoding/json"
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
	// A Windows UNC absolute path: "\\server\share\..." naming a network
	// host and share rather than a drive letter. Both leading separators
	// must be backslashes -- a single leading "\" is not a recognized
	// absolute form on its own.
	if len(p) >= 2 && p[0] == '\\' && p[1] == '\\' {
		return true
	}
	return false
}

// hasRawField reports whether raw -- a JSON object -- contains a non-null
// value for key. encoding/json collapses an omitted field, an explicit
// null, and an explicit empty list onto the same Go zero value once decoded
// into a typed struct, so a required-field check that only inspects the
// decoded struct can never tell "the client sent nothing" apart from "the
// client sent an empty list." hasRawField answers that question directly
// from the wire bytes instead: a required list field must be present and
// non-null, though it may still be empty.
func hasRawField(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	value, ok := m[key]
	if !ok {
		return false
	}
	return string(bytes.TrimSpace(value)) != "null"
}

// validateCwdAndDirectories validates the cwd and additionalDirectories
// fields shared by session/new, session/load, and session/resume: cwd is
// required and must be absolute, and every additionalDirectories entry --
// each of which the ACP wire contract itself documents as "must be an
// absolute path" -- is rejected if it is not.
func validateCwdAndDirectories(cwd string, additionalDirectories []string) (string, []string, error) {
	if cwd == "" {
		return "", nil, errors.New("acp: cwd is required")
	}
	if !isAbsolutePath(cwd) {
		return "", nil, errors.New("acp: cwd must be an absolute path")
	}
	for i, dir := range additionalDirectories {
		if !isAbsolutePath(dir) {
			return "", nil, fmt.Errorf("acp: additionalDirectories[%d] must be an absolute path", i)
		}
	}
	return cwd, append([]string(nil), additionalDirectories...), nil
}

// rejectNonEmptyMcpServers rejects a non-empty client-supplied MCP server
// list: L1 V0 implements no MCP passthrough.
func rejectNonEmptyMcpServers(mcpServers []acpsdk.McpServer) error {
	if len(mcpServers) > 0 {
		return errors.New("acp: client-supplied MCP servers are not supported in L1 V0")
	}
	return nil
}

// NewSessionParams is the closed L1 V0 shape of a validated session/new
// request. Non-empty client-supplied MCP servers are an explicit rejection,
// not a silently ignored field: L1 V0 implements no MCP passthrough.
type NewSessionParams struct {
	Cwd                   string
	AdditionalDirectories []string
}

// ValidateNewSession validates a raw session/new request against the L1 V0
// compatibility boundary. It accepts the raw JSON-RPC params rather than an
// already-decoded acpsdk.NewSessionRequest because the pinned SDK marks
// mcpServers a required field with no "omitempty" tag: a decoded Go struct
// cannot distinguish an omitted mcpServers from an explicit empty list, so
// the required-field check must run against the wire bytes.
func ValidateNewSession(raw json.RawMessage) (NewSessionParams, error) {
	var req acpsdk.NewSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return NewSessionParams{}, fmt.Errorf("acp: malformed session/new request: %w", err)
	}
	if !hasRawField(raw, "mcpServers") {
		return NewSessionParams{}, errors.New("acp: mcpServers is required")
	}
	if err := rejectNonEmptyMcpServers(req.McpServers); err != nil {
		return NewSessionParams{}, err
	}
	cwd, dirs, err := validateCwdAndDirectories(req.Cwd, req.AdditionalDirectories)
	if err != nil {
		return NewSessionParams{}, err
	}
	return NewSessionParams{Cwd: cwd, AdditionalDirectories: dirs}, nil
}

// LoadSessionParams is the closed L1 V0 shape of a validated session/load
// or session/resume request.
type LoadSessionParams struct {
	SessionID SessionID
	NewSessionParams
}

// ValidateLoadSession validates a raw session/load request against the L1
// V0 compatibility boundary. Like session/new, the pinned SDK marks
// mcpServers required for session/load, so this validates against the raw
// JSON-RPC params rather than an already-decoded request.
func ValidateLoadSession(raw json.RawMessage) (LoadSessionParams, error) {
	var req acpsdk.LoadSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return LoadSessionParams{}, fmt.Errorf("acp: malformed session/load request: %w", err)
	}
	if req.SessionId == "" {
		return LoadSessionParams{}, errors.New("acp: sessionId is required")
	}
	if !hasRawField(raw, "mcpServers") {
		return LoadSessionParams{}, errors.New("acp: mcpServers is required")
	}
	if err := rejectNonEmptyMcpServers(req.McpServers); err != nil {
		return LoadSessionParams{}, err
	}
	cwd, dirs, err := validateCwdAndDirectories(req.Cwd, req.AdditionalDirectories)
	if err != nil {
		return LoadSessionParams{}, err
	}
	return LoadSessionParams{SessionID: SessionID(req.SessionId), NewSessionParams: NewSessionParams{Cwd: cwd, AdditionalDirectories: dirs}}, nil
}

// ValidateResumeSession validates a raw session/resume request against the
// L1 V0 compatibility boundary. Unlike session/new and session/load, the
// pinned SDK marks session/resume's mcpServers "omitempty": it is optional,
// but a non-empty value is still rejected the same way.
func ValidateResumeSession(raw json.RawMessage) (LoadSessionParams, error) {
	var req acpsdk.ResumeSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return LoadSessionParams{}, fmt.Errorf("acp: malformed session/resume request: %w", err)
	}
	if req.SessionId == "" {
		return LoadSessionParams{}, errors.New("acp: sessionId is required")
	}
	if err := rejectNonEmptyMcpServers(req.McpServers); err != nil {
		return LoadSessionParams{}, err
	}
	cwd, dirs, err := validateCwdAndDirectories(req.Cwd, req.AdditionalDirectories)
	if err != nil {
		return LoadSessionParams{}, err
	}
	return LoadSessionParams{SessionID: SessionID(req.SessionId), NewSessionParams: NewSessionParams{Cwd: cwd, AdditionalDirectories: dirs}}, nil
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

// supportedConfigOptionType is the only session/set_config_option "type"
// discriminator value this compatibility layer recognizes on the wire; the
// value-id variant has no discriminator literal of its own and is only ever
// reached by omitting "type" entirely. The pinned SDK's
// SetSessionConfigOptionRequest.UnmarshalJSON does not actually reject an
// unrecognized "type": depending on which other fields are present, it can
// silently fall through to decoding the payload as the boolean variant (with
// a zero-value Value) or fail with an opaque "invalid variant payload"
// error, neither of which is a clear, deterministic rejection of the
// unsupported discriminator itself. ValidateSetConfigOption re-checks the
// raw "type" field before ever asking the SDK to decode the payload, so an
// unrecognized discriminator is rejected on its own terms.
const supportedConfigOptionType = "boolean"

// ValidateSetConfigOption validates a raw session/set_config_option request
// against the L1 V0 compatibility boundary.
func ValidateSetConfigOption(raw json.RawMessage) (ConfigOptionValue, error) {
	var discriminator struct {
		Type *string `json:"type"`
	}
	if err := json.Unmarshal(raw, &discriminator); err != nil {
		return ConfigOptionValue{}, fmt.Errorf("acp: malformed session/set_config_option request: %w", err)
	}
	if discriminator.Type != nil && *discriminator.Type != supportedConfigOptionType {
		return ConfigOptionValue{}, fmt.Errorf("acp: unsupported session/set_config_option type %q", *discriminator.Type)
	}
	// The pinned SDK's fallback decode path treats a request with neither a
	// recognized "type" nor a "value" field as a valid boolean payload
	// defaulting Value to false, rather than failing to decode. Requiring
	// "value" up front closes that silent-acceptance gap.
	if !hasRawField(raw, "value") {
		return ConfigOptionValue{}, errors.New("acp: value is required")
	}

	var req acpsdk.SetSessionConfigOptionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return ConfigOptionValue{}, fmt.Errorf("acp: malformed session/set_config_option request: %w", err)
	}

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

// UsageInfo is the closed L1 V0 shape of a usage_update's context-window
// facts: total size and tokens currently used. Cost is deliberately not
// carried: it is an unstable, optional SDK field this compatibility layer
// does not yet advertise support for.
type UsageInfo struct {
	Size int
	Used int
}

// SessionInfo is the closed L1 V0 shape of a session_info_update's session
// metadata facts.
type SessionInfo struct {
	Title     *string
	UpdatedAt *string
}

// TextUpdate is the closed L1 V0 shape of a validated session/update
// notification. SessionID is always populated: every session/update
// notification pertains to exactly one session, and this compatibility
// layer must be able to correlate an update to the session it belongs to.
// Content and MessageID are populated only for the three streamed-message
// kinds; Usage, SessionInfo, and ConfigOptions are populated only for their
// respective kind and losslessly carry that update's supported semantic
// fields rather than reducing them to a bare marker.
type TextUpdate struct {
	SessionID     SessionID
	Kind          TextUpdateKind
	Content       *TextContent
	MessageID     string
	Usage         *UsageInfo
	SessionInfo   *SessionInfo
	ConfigOptions []acpsdk.SessionConfigOption
}

func chunkUpdate(sessionID SessionID, kind TextUpdateKind, content acpsdk.ContentBlock, messageID *string) (TextUpdate, error) {
	text, err := validateTextBlock(content)
	if err != nil {
		return TextUpdate{}, fmt.Errorf("acp: %s: %w", kind, err)
	}
	update := TextUpdate{SessionID: sessionID, Kind: kind, Content: &text}
	if messageID != nil {
		update.MessageID = *messageID
	}
	return update, nil
}

// ValidateSessionUpdate validates a session/update notification against the
// L1 V0 compatibility boundary. The notification's sessionId is required:
// the wire value is acpsdk.SessionNotification{SessionId, Update}, and a
// notification with no session identity can never be correlated to the
// session it pertains to.
func ValidateSessionUpdate(notification acpsdk.SessionNotification) (TextUpdate, error) {
	if notification.SessionId == "" {
		return TextUpdate{}, errors.New("acp: sessionId is required")
	}
	sessionID := SessionID(notification.SessionId)
	update := notification.Update

	switch {
	case update.UserMessageChunk != nil:
		return chunkUpdate(sessionID, TextUpdateUserMessageChunk, update.UserMessageChunk.Content, update.UserMessageChunk.MessageId)
	case update.AgentMessageChunk != nil:
		return chunkUpdate(sessionID, TextUpdateAgentMessageChunk, update.AgentMessageChunk.Content, update.AgentMessageChunk.MessageId)
	case update.AgentThoughtChunk != nil:
		return chunkUpdate(sessionID, TextUpdateAgentThoughtChunk, update.AgentThoughtChunk.Content, update.AgentThoughtChunk.MessageId)
	case update.UsageUpdate != nil:
		return TextUpdate{SessionID: sessionID, Kind: TextUpdateUsage, Usage: &UsageInfo{
			Size: update.UsageUpdate.Size,
			Used: update.UsageUpdate.Used,
		}}, nil
	case update.SessionInfoUpdate != nil:
		return TextUpdate{SessionID: sessionID, Kind: TextUpdateSessionInfo, SessionInfo: &SessionInfo{
			Title:     update.SessionInfoUpdate.Title,
			UpdatedAt: update.SessionInfoUpdate.UpdatedAt,
		}}, nil
	case update.ConfigOptionUpdate != nil:
		return TextUpdate{SessionID: sessionID, Kind: TextUpdateConfigOption,
			ConfigOptions: append([]acpsdk.SessionConfigOption(nil), update.ConfigOptionUpdate.ConfigOptions...),
		}, nil
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
