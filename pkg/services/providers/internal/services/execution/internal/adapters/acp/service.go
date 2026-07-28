package acp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	acpsdk "github.com/coder/acp-go-sdk"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

type Adapter struct {
	newCommand platformprocess.CommandFactory
}

func New(newCommand platformprocess.CommandFactory) (*Adapter, error) {
	if newCommand == nil {
		return nil, errors.New("ACP process command factory is required")
	}
	return &Adapter{newCommand: newCommand}, nil
}

func (a *Adapter) Execute(ctx context.Context, descriptor catalog.Descriptor, request providers.ExecuteRequest) (providers.ExecuteResponse, error) {
	stream, err := a.ExecuteStream(ctx, descriptor, request)
	if err != nil {
		return providers.ExecuteResponse{}, err
	}
	defer stream.Close()
	for range stream.Updates {
	}
	outcome, ok := <-stream.Outcome
	if !ok {
		return providers.ExecuteResponse{}, errors.New("ACP execution stream closed without an outcome")
	}
	return outcome.Response, outcome.Err
}

func (a *Adapter) ExecuteStream(ctx context.Context, descriptor catalog.Descriptor, request providers.ExecuteRequest) (*providers.ExecutionStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	updates := make(chan providers.ExecutionUpdate, 64)
	outcomes := make(chan providers.ExecuteOutcome, 1)
	go func() {
		defer close(updates)
		defer close(outcomes)
		response, err := a.execute(streamCtx, descriptor, request, func(update providers.ExecutionUpdate) error {
			select {
			case updates <- update:
				return nil
			case <-streamCtx.Done():
				return context.Cause(streamCtx)
			}
		})
		outcomes <- providers.ExecuteOutcome{Response: response, Err: err}
	}()
	return &providers.ExecutionStream{Updates: updates, Outcome: outcomes, Close: cancel}, nil
}

func (a *Adapter) execute(
	ctx context.Context,
	descriptor catalog.Descriptor,
	request providers.ExecuteRequest,
	emit func(providers.ExecutionUpdate) error,
) (providers.ExecuteResponse, error) {
	if err := ctx.Err(); err != nil {
		return providers.ExecuteResponse{}, err
	}
	if strings.TrimSpace(descriptor.Provider.Command) == "" {
		return providers.ExecuteResponse{}, fmt.Errorf("%w: ACP provider %q has no command", providers.ErrInvalidRequest, descriptor.Provider.ID)
	}
	cwd, err := absoluteWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return providers.ExecuteResponse{}, err
	}
	prompt, err := promptBlocks(request)
	if err != nil {
		return providers.ExecuteResponse{}, err
	}

	cmd := a.newCommand(descriptor.Provider.Command, descriptor.Provider.Arguments...)
	if cmd == nil {
		return providers.ExecuteResponse{}, fmt.Errorf("ACP command factory returned nil for %q", descriptor.Provider.Command)
	}
	cmd.Dir = cwd
	cmd.Env = environment(request.Environment)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return providers.ExecuteResponse{}, fmt.Errorf("open ACP stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return providers.ExecuteResponse{}, fmt.Errorf("open ACP stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	platformprocess.ConfigureSubprocessTree(cmd)
	if err := cmd.Start(); err != nil {
		return providers.ExecuteResponse{}, fmt.Errorf("start ACP provider %q: %w", descriptor.Provider.ID, err)
	}
	tree, _ := platformprocess.AttachSubprocessTree(cmd)
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = platformprocess.TerminateSubprocessTree(cmd, tree)
		}
		select {
		case <-finished:
		default:
			<-finished
		}
		platformprocess.CloseSubprocessTree(cmd, tree)
	}()

	client := &client{skipPermissions: request.SkipPermissions, emit: emit}
	connection := acpsdk.NewClientSideConnection(client, stdin, stdout)
	initialize, err := connection.Initialize(ctx, acpsdk.InitializeRequest{
		ProtocolVersion:    acpsdk.ProtocolVersionNumber,
		ClientCapabilities: acpsdk.ClientCapabilities{},
	})
	if err != nil {
		if ctx.Err() != nil {
			return providers.ExecuteResponse{}, context.Cause(ctx)
		}
		return providers.ExecuteResponse{}, protocolError("initialize", descriptor.Provider.ID, err, safeACPStderr(stderr.String(), request.Environment))
	}
	if initialize.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		return providers.ExecuteResponse{}, fmt.Errorf("%w: ACP provider %q negotiated unsupported protocol version %v", providers.ErrIncompatibleProtocol, descriptor.Provider.ID, initialize.ProtocolVersion)
	}
	session, err := connection.NewSession(ctx, acpsdk.NewSessionRequest{Cwd: cwd, McpServers: []acpsdk.McpServer{}})
	if err != nil {
		if ctx.Err() != nil {
			return providers.ExecuteResponse{}, context.Cause(ctx)
		}
		var requestErr *acpsdk.RequestError
		if errors.As(err, &requestErr) && requestErr.Code == -32000 {
			return providers.ExecuteResponse{}, fmt.Errorf("%w: ACP provider %q requires authentication%s", providers.ErrAuthenticationRequired, descriptor.Provider.ID, authenticationMethodHint(initialize.AuthMethods))
		}
		return providers.ExecuteResponse{}, protocolError("session/new", descriptor.Provider.ID, err, safeACPStderr(stderr.String(), request.Environment))
	}
	client.setSessionID(string(session.SessionId))
	modelConfig, err := applyAdvertisedModel(ctx, connection, session, request.Model)
	if err != nil {
		return providers.ExecuteResponse{}, protocolError("session/set_config_option", descriptor.Provider.ID, err, safeACPStderr(stderr.String(), request.Environment))
	}
	if _, err := connection.Prompt(ctx, acpsdk.PromptRequest{SessionId: session.SessionId, Prompt: prompt}); err != nil {
		if ctx.Err() != nil {
			if content := client.content(); content != "" {
				_ = client.emitUpdate(providers.ExecutionUpdate{Kind: providers.ExecutionUpdateMessage, NativeType: "session/prompt", ItemID: "assistant-message", Text: content, Final: true, Partial: true})
			}
			return providers.ExecuteResponse{}, context.Cause(ctx)
		}
		promptErr := protocolError("session/prompt", descriptor.Provider.ID, err, safeACPStderr(stderr.String(), request.Environment))
		if content := client.content(); content != "" {
			_ = client.emitUpdate(providers.ExecutionUpdate{Kind: providers.ExecutionUpdateMessage, NativeType: "session/prompt", ItemID: "assistant-message", Text: content, Final: true, Partial: true})
		}
		_ = client.emitUpdate(providers.ExecutionUpdate{Kind: providers.ExecutionUpdateError, NativeType: "session/prompt", ItemID: "acp-prompt-error", Error: &providers.ErrorUpdate{Code: "ACP_PROMPT_FAILED", Message: promptErr.Error()}})
		return providers.ExecuteResponse{}, promptErr
	}
	if content := client.content(); content != "" {
		if err := client.emitUpdate(providers.ExecutionUpdate{Kind: providers.ExecutionUpdateMessage, NativeType: "session/prompt", ItemID: "assistant-message", Text: content, Final: true}); err != nil {
			return providers.ExecuteResponse{}, err
		}
	}
	return providers.ExecuteResponse{
		Content: client.content(),
		Session: &providers.SessionRef{ProviderID: descriptor.Provider.ID, Kind: "session_id", ID: string(session.SessionId)},
		Diagnostics: &providers.SafeDiagnostics{Metadata: map[string]string{
			"execution_kind":   string(providers.ExecutionKindACP),
			"protocol_version": fmt.Sprint(initialize.ProtocolVersion),
			"model_config":     modelConfig,
		}},
	}, nil
}

func applyAdvertisedModel(ctx context.Context, connection *acpsdk.ClientSideConnection, session acpsdk.NewSessionResponse, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "not_requested", nil
	}
	for _, config := range session.ConfigOptions {
		if config.Select == nil || config.Select.Category == nil || *config.Select.Category != acpsdk.SessionConfigOptionCategoryModel {
			continue
		}
		if !selectOptionContains(config.Select.Options, acpsdk.SessionConfigValueId(model)) {
			continue
		}
		_, err := connection.SetSessionConfigOption(ctx, acpsdk.SetSessionConfigOptionRequest{ValueId: &acpsdk.SetSessionConfigOptionValueId{
			SessionId: session.SessionId, ConfigId: config.Select.Id, Value: acpsdk.SessionConfigValueId(model),
		}})
		if err != nil {
			return "failed", err
		}
		return "applied", nil
	}
	return "not_advertised", nil
}

func selectOptionContains(options acpsdk.SessionConfigSelectOptions, model acpsdk.SessionConfigValueId) bool {
	if options.Ungrouped != nil {
		for _, option := range *options.Ungrouped {
			if option.Value == model {
				return true
			}
		}
	}
	if options.Grouped != nil {
		for _, group := range *options.Grouped {
			for _, option := range group.Options {
				if option.Value == model {
					return true
				}
			}
		}
	}
	return false
}

func authenticationMethodHint(methods []acpsdk.AuthMethod) string {
	labels := make([]string, 0, len(methods))
	for _, method := range methods {
		switch {
		case method.Agent != nil:
			labels = append(labels, method.Agent.Name)
		case method.EnvVar != nil:
			labels = append(labels, method.EnvVar.Name)
		case method.Terminal != nil:
			labels = append(labels, method.Terminal.Name)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return "; advertised methods: " + strings.Join(labels, ", ")
}

func absoluteWorkingDirectory(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%w: ACP working directory is required", providers.ErrInvalidRequest)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%w: resolve ACP working directory: %v", providers.ErrInvalidRequest, err)
	}
	return abs, nil
}

func promptBlocks(request providers.ExecuteRequest) ([]acpsdk.ContentBlock, error) {
	blocks := make([]acpsdk.ContentBlock, 0, len(request.Prompt)+1)
	if instructions := strings.TrimSpace(request.Instructions); instructions != "" {
		blocks = append(blocks, acpsdk.TextBlock("System instructions:\n"+instructions))
	}
	for index, part := range request.Prompt {
		switch part.Kind {
		case providers.ContentKindText:
			blocks = append(blocks, acpsdk.TextBlock(part.Text))
		case providers.ContentKindResourceLink:
			if strings.TrimSpace(part.URI) == "" {
				return nil, fmt.Errorf("%w: prompt resource link %d requires a URI", providers.ErrInvalidRequest, index)
			}
			block := acpsdk.ResourceLinkBlock(firstNonBlank(part.Name, part.URI), part.URI)
			if part.MimeType != "" {
				block.ResourceLink.MimeType = stringPointer(part.MimeType)
			}
			blocks = append(blocks, block)
		default:
			return nil, fmt.Errorf("%w: prompt content part %d has unsupported kind %q", providers.ErrInvalidRequest, index, part.Kind)
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, acpsdk.TextBlock(""))
	}
	return blocks, nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "resource"
}

func stringPointer(value string) *string { return &value }

func environment(entries []providers.EnvironmentEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name != "" {
			result = append(result, entry.Name+"="+entry.Value)
		}
	}
	return result
}

func protocolError(method string, id providers.ID, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail != "" {
		return fmt.Errorf("%w: ACP provider %q %s failed: %v (stderr: %s)", providers.ErrProtocol, id, method, err, detail)
	}
	return fmt.Errorf("%w: ACP provider %q %s failed: %v", providers.ErrProtocol, id, method, err)
}

func safeACPStderr(value string, environment []providers.EnvironmentEntry) string {
	redacted := value
	for _, entry := range environment {
		if !sensitiveEnvironmentName(entry.Name) || len(entry.Value) < 4 {
			continue
		}
		redacted = strings.ReplaceAll(redacted, entry.Value, "<redacted>")
	}
	redacted = strings.TrimSpace(redacted)
	if len(redacted) > 1024 {
		redacted = redacted[:1024]
	}
	return redacted
}

func sensitiveEnvironmentName(name string) bool {
	canonical := strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range []string{"API_KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(canonical, marker) {
			return true
		}
	}
	return false
}

type client struct {
	mu              sync.Mutex
	skipPermissions bool
	text            strings.Builder
	sequence        int64
	sessionID       string
	emit            func(providers.ExecutionUpdate) error
}

func (c *client) SessionUpdate(ctx context.Context, notification acpsdk.SessionNotification) error {
	update := notification.Update
	mapped, text := mapSessionUpdate(update)
	if len(mapped) == 0 {
		return nil
	}
	c.mu.Lock()
	if text != "" {
		c.text.WriteString(text)
	}
	c.mu.Unlock()
	for _, item := range mapped {
		if err := c.emitUpdate(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) setSessionID(value string) {
	c.mu.Lock()
	c.sessionID = value
	c.mu.Unlock()
}

func mapSessionUpdate(update acpsdk.SessionUpdate) ([]providers.ExecutionUpdate, string) {
	switch {
	case update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil:
		chunk := update.AgentMessageChunk
		return []providers.ExecutionUpdate{{
			Kind: providers.ExecutionUpdateMessage, NativeType: chunk.SessionUpdate,
			ItemID: optionalItemID(chunk.MessageId, "assistant-message"), Text: chunk.Content.Text.Text,
		}}, chunk.Content.Text.Text
	case update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil:
		chunk := update.AgentThoughtChunk
		return []providers.ExecutionUpdate{{
			Kind: providers.ExecutionUpdateReasoning, NativeType: chunk.SessionUpdate,
			ItemID: optionalItemID(chunk.MessageId, "assistant-reasoning"), Text: chunk.Content.Text.Text,
		}}, ""
	case update.ToolCall != nil:
		tool := update.ToolCall
		mapped := []providers.ExecutionUpdate{{
			Kind: providers.ExecutionUpdateTool, NativeType: tool.SessionUpdate, ItemID: string(tool.ToolCallId),
			Tool: &providers.ToolUpdate{ID: string(tool.ToolCallId), Name: tool.Title, Status: string(tool.Status), RawInput: tool.RawInput, RawOutput: tool.RawOutput},
		}}
		return append(mapped, mapFileChanges(tool.SessionUpdate, string(tool.ToolCallId), tool.Content)...), ""
	case update.ToolCallUpdate != nil:
		tool := update.ToolCallUpdate
		status := ""
		if tool.Status != nil {
			status = string(*tool.Status)
		}
		name := ""
		if tool.Title != nil {
			name = *tool.Title
		}
		mapped := []providers.ExecutionUpdate{{
			Kind: providers.ExecutionUpdateTool, NativeType: tool.SessionUpdate, ItemID: string(tool.ToolCallId),
			Tool: &providers.ToolUpdate{ID: string(tool.ToolCallId), Name: name, Status: status, RawInput: tool.RawInput, RawOutput: tool.RawOutput},
		}}
		return append(mapped, mapFileChanges(tool.SessionUpdate, string(tool.ToolCallId), tool.Content)...), ""
	case update.Plan != nil:
		entries := make([]providers.PlanEntry, len(update.Plan.Entries))
		for index, entry := range update.Plan.Entries {
			entries[index] = providers.PlanEntry{ID: fmt.Sprintf("plan-step-%d", index+1), Description: entry.Content, Status: string(entry.Status)}
		}
		return []providers.ExecutionUpdate{{Kind: providers.ExecutionUpdatePlan, NativeType: update.Plan.SessionUpdate, ItemID: "plan", Plan: entries}}, ""
	case update.UsageUpdate != nil:
		return []providers.ExecutionUpdate{{
			Kind: providers.ExecutionUpdateUsage, NativeType: update.UsageUpdate.SessionUpdate, ItemID: "usage",
			Usage: &providers.UsageUpdate{UsedTokens: int64(update.UsageUpdate.Used), MaxTokens: int64(update.UsageUpdate.Size)},
		}}, ""
	case update.SessionInfoUpdate != nil:
		metadata := map[string]string{}
		if update.SessionInfoUpdate.Title != nil {
			metadata["title"] = *update.SessionInfoUpdate.Title
		}
		if update.SessionInfoUpdate.UpdatedAt != nil {
			metadata["updated_at"] = *update.SessionInfoUpdate.UpdatedAt
		}
		return []providers.ExecutionUpdate{{Kind: providers.ExecutionUpdateSession, NativeType: update.SessionInfoUpdate.SessionUpdate, ItemID: "session", Metadata: metadata}}, ""
	default:
		return nil, ""
	}
}

func mapFileChanges(nativeType, toolCallID string, content []acpsdk.ToolCallContent) []providers.ExecutionUpdate {
	var updates []providers.ExecutionUpdate
	for index, item := range content {
		if item.Diff == nil {
			continue
		}
		operation := "update"
		if item.Diff.OldText == nil {
			operation = "create"
		} else if item.Diff.NewText == "" {
			operation = "delete"
		}
		updates = append(updates, providers.ExecutionUpdate{
			Kind: providers.ExecutionUpdateFileChange, NativeType: nativeType,
			ItemID:     fmt.Sprintf("%s-file-%d", toolCallID, index+1),
			FileChange: &providers.FileChangeUpdate{Path: item.Diff.Path, Operation: operation, Summary: "ACP tool file diff"},
		})
	}
	return updates
}

func (c *client) emitUpdate(update providers.ExecutionUpdate) error {
	c.mu.Lock()
	c.sequence++
	update.Sequence = c.sequence
	update.ProviderSessionID = c.sessionID
	c.mu.Unlock()
	if c.emit == nil {
		return nil
	}
	return c.emit(update)
}

func optionalItemID(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return strings.TrimSpace(*value)
	}
	return fallback
}

func (c *client) content() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text.String()
}

func (c *client) RequestPermission(ctx context.Context, request acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	if ctx.Err() != nil {
		return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeCancelled()}, nil
	}
	wantAllow := c.skipPermissions
	for _, option := range request.Options {
		allow := option.Kind == acpsdk.PermissionOptionKindAllowOnce || option.Kind == acpsdk.PermissionOptionKindAllowAlways
		if allow == wantAllow {
			return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeSelected(option.OptionId)}, nil
		}
	}
	return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeCancelled()}, nil
}

func (*client) ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	return acpsdk.ReadTextFileResponse{}, errors.New("ACP filesystem reads are not supported")
}
func (*client) WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	return acpsdk.WriteTextFileResponse{}, errors.New("ACP filesystem writes are not supported")
}
func (*client) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error) {
	return acpsdk.KillTerminalResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, errors.New("ACP terminals are not supported")
}

var _ acpsdk.Client = (*client)(nil)
