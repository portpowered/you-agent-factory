// Package service implements the parent-private Agent Client Protocol service.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
)

// Command is one configured stdio ACP launch command.
type Command struct {
	Name string
	Args []string
}

// Service owns configured ACP commands. A later lifecycle step upgrades these
// entries to retained daemons without changing the parent contract.
type Service struct {
	commands   map[providers.ID]Command
	newCommand platformprocess.CommandFactory
	locator    platformprocess.ExecutableLocator
}

var _ acp.Service = (*Service)(nil)

func New(commands map[providers.ID]Command, newCommand platformprocess.CommandFactory, locator platformprocess.ExecutableLocator) (acp.Service, error) {
	detached := make(map[providers.ID]Command, len(commands))
	for id, command := range commands {
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("construct ACP service: %w", err)
		}
		if strings.TrimSpace(command.Name) == "" {
			return nil, fmt.Errorf("construct ACP service %q: command is required", id)
		}
		detached[id] = Command{Name: command.Name, Args: append([]string(nil), command.Args...)}
	}
	return &Service{commands: detached, newCommand: newCommand, locator: locator}, nil
}

func (service *Service) Execute(ctx context.Context, id providers.ID, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	if service.newCommand == nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "ACP command is unavailable"}
	}
	command, ok := service.commands[id]
	if !ok {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: fmt.Sprintf("ACP provider %q is unavailable", id)}
	}
	if service.locator != nil {
		if _, err := service.locator.LookPath(command.Name); err != nil {
			return providers.ExecuteResult{}, dependencyFailure(fmt.Sprintf("ACP executable %q is unavailable", command.Name))
		}
	}
	return execute(ctx, id, command, service.newCommand, request)
}

func (*Service) Close(context.Context) error {
	return nil
}

func execute(ctx context.Context, id providers.ID, command Command, newCommand platformprocess.CommandFactory, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	if err := ctx.Err(); err != nil {
		return providers.ExecuteResult{}, nativeFailure(err)
	}
	cwd, err := absoluteWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return providers.ExecuteResult{}, invalidFailure(err)
	}
	prompt := []acpsdk.ContentBlock{}
	if text := strings.TrimSpace(request.SystemPrompt); text != "" {
		prompt = append(prompt, acpsdk.TextBlock("System instructions:\n"+text))
	}
	prompt = append(prompt, acpsdk.TextBlock(request.UserMessage))
	prompt = append(prompt, resourceLinks(request.InputTokens, request.UserMessage)...)

	cmd := newCommand(command.Name, command.Args...)
	if cmd == nil {
		return providers.ExecuteResult{}, dependencyFailure("ACP command factory returned nil")
	}
	cmd.Dir, cmd.Env = cwd, requestEnvironment(request)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return providers.ExecuteResult{}, dependencyFailure(err.Error())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return providers.ExecuteResult{}, dependencyFailure(err.Error())
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	platformprocess.ConfigureSubprocessTree(cmd)
	if err := cmd.Start(); err != nil {
		return providers.ExecuteResult{}, dependencyFailure(fmt.Sprintf("start ACP provider %q: %v", id, err))
	}
	tree, _ := platformprocess.AttachSubprocessTree(cmd)
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	defer func() {
		//TODO: needs logs.
		_ = stdin.Close()
		select {
		case <-finished:
		case <-time.After(500 * time.Millisecond):
			if cmd.Process != nil {
				_ = platformprocess.TerminateSubprocessTree(cmd, tree)
			}
			<-finished
		}
		platformprocess.CloseSubprocessTree(cmd, tree)
	}()

	client := &client{skipPermissions: request.SkipPermissions}
	connection := acpsdk.NewClientSideConnection(client, stdin, stdout)
	initialized, err := connection.Initialize(ctx, acpsdk.InitializeRequest{ProtocolVersion: acpsdk.ProtocolVersionNumber, ClientCapabilities: acpsdk.ClientCapabilities{}})
	if err != nil {
		return providers.ExecuteResult{}, rpcFailure(ctx, "initialize", id, err, stderr.String(), request)
	}
	if initialized.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindMisconfigured, Message: fmt.Sprintf("ACP provider %q negotiated unsupported protocol version %v", id, initialized.ProtocolVersion)}
	}
	session, err := connection.NewSession(ctx, acpsdk.NewSessionRequest{Cwd: cwd, McpServers: []acpsdk.McpServer{}})
	if err != nil {
		var requestErr *acpsdk.RequestError
		if errors.As(err, &requestErr) && requestErr.Code == -32000 {
			return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: "ACP authentication required" + authenticationMethodHint(initialized.AuthMethods)}
		}
		return providers.ExecuteResult{}, rpcFailure(ctx, "session/new", id, err, stderr.String(), request)
	}
	client.setSessionID(string(session.SessionId))
	modelConfig, err := applyAdvertisedModel(ctx, connection, session, request.Model)
	if err != nil {
		return providers.ExecuteResult{}, rpcFailure(ctx, "session/set_config_option", id, err, stderr.String(), request)
	}
	if _, err := connection.Prompt(ctx, acpsdk.PromptRequest{SessionId: session.SessionId, Prompt: prompt}); err != nil {
		return providers.ExecuteResult{}, withPartial(rpcFailure(ctx, "session/prompt", id, err, stderr.String(), request), client)
	}
	return providers.ExecuteResult{Content: client.content(), SessionRef: &providers.SessionRef{Provider: id, Kind: providers.SessionIDKind, ID: string(session.SessionId)}, Diagnostics: &providers.ExecuteDiagnostics{Progress: client.progressFacts(), Metadata: map[string]string{"execution_kind": "acp", "protocol_version": fmt.Sprint(initialized.ProtocolVersion), "model_config": modelConfig}}}, nil
}

func requestEnvironment(request providers.ExecuteRequest) []string {
	if request.EnvVars == nil {
		return append([]string(nil), request.ProcessEnvironment...)
	}
	values := append([]string(nil), request.ProcessEnvironment...)
	for key, value := range request.EnvVars {
		values = append(values, key+"="+value)
	}
	return values
}

func absoluteWorkingDirectory(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("ACP working directory is required")
	}
	return filepath.Abs(value)
}

func applyAdvertisedModel(ctx context.Context, connection *acpsdk.ClientSideConnection, session acpsdk.NewSessionResponse, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "not_requested", nil
	}
	for _, config := range session.ConfigOptions {
		if config.Select == nil || config.Select.Category == nil || *config.Select.Category != acpsdk.SessionConfigOptionCategoryModel || !selectOptionContains(config.Select.Options, acpsdk.SessionConfigValueId(model)) {
			continue
		}
		_, err := connection.SetSessionConfigOption(ctx, acpsdk.SetSessionConfigOptionRequest{ValueId: &acpsdk.SetSessionConfigOptionValueId{SessionId: session.SessionId, ConfigId: config.Select.Id, Value: acpsdk.SessionConfigValueId(model)}})
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
	labels := []string{}
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

func rpcFailure(ctx context.Context, method string, id providers.ID, err error, stderr string, request providers.ExecuteRequest) error {
	if ctx.Err() != nil {
		return nativeFailure(ctx.Err())
	}
	detail := safeACPStderr(stderr, request.EnvVars)
	message := fmt.Sprintf("ACP provider %q %s failed: %s", id, method, safeRPCMessage(err))
	if detail != "" {
		message += " (stderr: " + detail + ")"
	}
	kind := providers.ExecuteFailureKindUnknown
	if method == "initialize" {
		native := strings.ToLower(err.Error())
		if strings.Contains(native, "protocol version") || strings.Contains(native, "protocolversion") {
			kind = providers.ExecuteFailureKindMisconfigured
		}
	}
	return providers.ExecuteFailure{Kind: kind, Message: message}
}
func safeRPCMessage(err error) string {
	var requestErr *acpsdk.RequestError
	if errors.As(err, &requestErr) && strings.TrimSpace(requestErr.Message) != "" {
		return strings.TrimSpace(requestErr.Message)
	}
	message := strings.TrimSpace(err.Error())
	if message != "" && len(message) <= 256 && !strings.ContainsAny(message, "{}[]\\/") {
		return message
	}
	return "RPC request failed"
}

var renderedURL = regexp.MustCompile(`https?://[^\s\]\)]+`)

func resourceLinks(values []any, renderedPrompt string) []acpsdk.ContentBlock {
	var blocks []acpsdk.ContentBlock
	seen := map[string]struct{}{}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var decoded any
		if json.Unmarshal(encoded, &decoded) != nil {
			continue
		}
		for _, block := range resourceLinksFromJSON(decoded) {
			if _, ok := seen[block.ResourceLink.Uri]; ok {
				continue
			}
			seen[block.ResourceLink.Uri] = struct{}{}
			blocks = append(blocks, block)
		}
	}
	for _, url := range renderedURL.FindAllString(renderedPrompt, -1) {
		if _, ok := seen[url]; ok {
			continue
		}
		block := acpsdk.ResourceLinkBlock(url, url)
		ext := strings.ToLower(filepath.Ext(url))
		mime := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}[ext]
		if mime != "" {
			block.ResourceLink.MimeType = &mime
		}
		seen[url] = struct{}{}
		blocks = append(blocks, block)
	}
	return blocks
}

func resourceLinksFromJSON(value any) []acpsdk.ContentBlock {
	var blocks []acpsdk.ContentBlock
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			blocks = append(blocks, resourceLinksFromJSON(child)...)
		}
	case map[string]any:
		url, _ := typed["url"].(string)
		if strings.TrimSpace(url) != "" {
			name, _ := typed["label"].(string)
			if strings.TrimSpace(name) == "" {
				name = url
			}
			block := acpsdk.ResourceLinkBlock(name, url)
			if mime, ok := typed["contentType"].(string); ok && mime != "" {
				block.ResourceLink.MimeType = &mime
			}
			blocks = append(blocks, block)
		}
		for key, child := range typed {
			if key != "url" {
				blocks = append(blocks, resourceLinksFromJSON(child)...)
			}
		}
	}
	return blocks
}
func invalidFailure(err error) error {
	return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: err.Error()}
}
func dependencyFailure(message string) error {
	return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: message}
}
func nativeFailure(err error) error {
	kind := providers.ExecuteFailureKindUnknown
	if errors.Is(err, context.Canceled) {
		kind = providers.ExecuteFailureKindCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		kind = providers.ExecuteFailureKindTimeout
	}
	return providers.ExecuteFailure{Kind: kind, Message: err.Error()}
}
func withPartial(err error, c *client) error {
	var f providers.ExecuteFailure
	if errors.As(err, &f) {
		f.Diagnostics = &providers.ExecuteDiagnostics{Progress: c.progressFacts(), Metadata: map[string]string{"partial_content": c.content()}}
		return f
	}
	return err
}
func safeACPStderr(value string, env map[string]string) string {
	for name, secret := range env {
		if sensitiveEnvironmentName(name) && len(secret) >= 4 {
			value = strings.ReplaceAll(value, secret, "<redacted>")
		}
	}
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
func sensitiveEnvironmentName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range []string{"API_KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

type client struct {
	mu              sync.Mutex
	skipPermissions bool
	text            strings.Builder
	sessionID       string
	progress        []providers.ExecuteProgress
}

func (c *client) SessionUpdate(_ context.Context, n acpsdk.SessionNotification) error {
	p, text := mapSessionUpdate(n.Update)
	c.mu.Lock()
	defer c.mu.Unlock()
	if text != "" {
		c.text.WriteString(text)
	}
	c.progress = append(c.progress, p...)
	return nil
}
func (c *client) setSessionID(v string) { c.mu.Lock(); c.sessionID = v; c.mu.Unlock() }
func (c *client) content() string       { c.mu.Lock(); defer c.mu.Unlock(); return c.text.String() }
func (c *client) progressFacts() []providers.ExecuteProgress {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]providers.ExecuteProgress, len(c.progress))
	for i := range c.progress {
		out[i] = c.progress[i].Clone()
	}
	return out
}

func mapSessionUpdate(update acpsdk.SessionUpdate) ([]providers.ExecuteProgress, string) {
	phase, detail, kind, itemID := "update", "", "unknown", ""
	metadata := map[string]string{}
	switch {
	case update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil:
		kind = "message"
		metadata["native_type"] = "agent_message_chunk"
		phase = "delta"
		detail = update.AgentMessageChunk.Content.Text.Text
		itemID = optionalItemID(update.AgentMessageChunk.MessageId, "assistant-message")
	case update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil:
		kind = "reasoning"
		metadata["native_type"] = "agent_thought_chunk"
		phase = "delta"
		detail = update.AgentThoughtChunk.Content.Text.Text
		itemID = optionalItemID(update.AgentThoughtChunk.MessageId, "assistant-reasoning")
	case update.ToolCall != nil:
		kind = "tool"
		metadata["native_type"] = "tool_call"
		phase = "started"
		detail = update.ToolCall.Title
		itemID = string(update.ToolCall.ToolCallId)
		metadata["status"] = string(update.ToolCall.Status)
		encodeMetadata(metadata, "raw_input", update.ToolCall.RawInput)
	case update.ToolCallUpdate != nil:
		kind = "tool"
		metadata["native_type"] = "tool_call_update"
		phase = "updated"
		itemID = string(update.ToolCallUpdate.ToolCallId)
		if update.ToolCallUpdate.Title != nil {
			detail = *update.ToolCallUpdate.Title
		}
		if update.ToolCallUpdate.Status != nil {
			metadata["status"] = string(*update.ToolCallUpdate.Status)
		}
		encodeMetadata(metadata, "raw_output", update.ToolCallUpdate.RawOutput)
		progress := []providers.ExecuteProgress{{Phase: phase, Detail: detail, Metadata: metadata}}
		for _, content := range update.ToolCallUpdate.Content {
			if content.Diff == nil {
				continue
			}
			operation := "modified"
			if content.Diff.OldText == nil {
				operation = "created"
			}
			progress = append(progress, providers.ExecuteProgress{
				Phase:  "updated",
				Detail: content.Diff.Path,
				Metadata: map[string]string{
					"kind":                "file_change",
					"item_id":             "file:" + content.Diff.Path,
					"native_type":         "tool_call_update",
					"provider_session_id": "",
					"path":                content.Diff.Path,
					"operation":           operation,
				},
			})
		}
		return progress, ""
	case update.Plan != nil:
		kind = "plan"
		metadata["native_type"] = "plan"
		phase = "updated"
		itemID = "plan"
		encodeMetadata(metadata, "entries", update.Plan.Entries)
	case update.UsageUpdate != nil:
		kind = "usage"
		metadata["native_type"] = "usage_update"
		phase = "updated"
		itemID = "usage"
		metadata["used_tokens"] = fmt.Sprint(update.UsageUpdate.Used)
		metadata["max_tokens"] = fmt.Sprint(update.UsageUpdate.Size)
	case update.SessionInfoUpdate != nil:
		kind = "session"
		metadata["native_type"] = "session_info_update"
		phase = "started"
		itemID = "session"
		if update.SessionInfoUpdate.Title != nil {
			detail = *update.SessionInfoUpdate.Title
		}
	default:
		return nil, ""
	}
	metadata["kind"], metadata["item_id"], metadata["provider_session_id"] = kind, itemID, ""
	return []providers.ExecuteProgress{{Phase: phase, Detail: detail, Metadata: metadata}}, func() string {
		if kind == "message" {
			return detail
		}
		return ""
	}()
}
func encodeMetadata(target map[string]string, key string, value any) {
	if value == nil {
		return
	}
	if data, err := json.Marshal(value); err == nil {
		target[key] = string(data)
	}
}
func optionalItemID(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return strings.TrimSpace(*value)
	}
	return fallback
}

func (c *client) RequestPermission(ctx context.Context, request acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	if ctx.Err() != nil {
		return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeCancelled()}, nil
	}
	want := c.skipPermissions
	for _, option := range request.Options {
		allow := option.Kind == acpsdk.PermissionOptionKindAllowOnce || option.Kind == acpsdk.PermissionOptionKindAllowAlways
		if allow == want {
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
