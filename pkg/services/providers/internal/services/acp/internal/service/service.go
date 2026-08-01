// Package service implements the parent-private Agent Client Protocol service.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/mattn/go-shellwords"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
)

// Command is one configured stdio ACP launch command.
type Command struct {
	Name string
	Args []string
}

// Service owns one retained daemon per configured ACP integration.
type Service struct {
	mu           sync.RWMutex
	daemons      map[providers.ID]*daemon
	integrations map[providers.ID]providers.ACPIntegration
	aliases      map[string]providers.ID
	newCommand   platformprocess.CommandFactory
	locator      platformprocess.ExecutableLocator
}

var _ acp.Service = (*Service)(nil)

func New(integrations []providers.ACPIntegration, newCommand platformprocess.CommandFactory, locator platformprocess.ExecutableLocator) (acp.Service, error) {
	service := &Service{newCommand: newCommand, locator: locator}
	if err := service.Configure(context.Background(), integrations); err != nil {
		return nil, err
	}
	return service, nil
}

func (service *Service) Execute(ctx context.Context, id providers.ID, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	service.mu.RLock()
	canonical, ok := service.resolveLocked(id)
	daemon := service.daemons[canonical]
	service.mu.RUnlock()
	if !ok {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: fmt.Sprintf("ACP provider %q is unavailable", id)}
	}
	return daemon.execute(ctx, canonical, request)
}

func (service *Service) Close(ctx context.Context) error {
	service.mu.Lock()
	daemons := service.daemons
	service.daemons = map[providers.ID]*daemon{}
	service.integrations = map[providers.ID]providers.ACPIntegration{}
	service.aliases = map[string]providers.ID{}
	service.mu.Unlock()
	var first error
	for id, daemon := range daemons {
		if err := daemon.close(ctx); err != nil && first == nil {
			first = fmt.Errorf("close ACP provider %q: %w", id, err)
		}
	}
	return first
}

func (service *Service) Configure(ctx context.Context, integrations []providers.ACPIntegration) error {
	commands := make(map[providers.ID]Command, len(integrations))
	values := make(map[providers.ID]providers.ACPIntegration, len(integrations))
	aliases := make(map[string]providers.ID, len(integrations))
	for _, integration := range integrations {
		integration = integration.Clone()
		if err := integration.Name.Validate(); err != nil {
			return fmt.Errorf("configure ACP service: %w", err)
		}
		if integration.Transport != "stdio" {
			return fmt.Errorf("configure ACP provider %q: unsupported transport %q", integration.Name, integration.Transport)
		}
		parts, err := shellwords.Parse(integration.Command)
		if err != nil || len(parts) == 0 {
			return fmt.Errorf("configure ACP provider %q: invalid command", integration.Name)
		}
		if _, exists := values[integration.Name]; exists {
			return fmt.Errorf("configure ACP provider %q: duplicate identity", integration.Name)
		}
		commands[integration.Name] = Command{Name: parts[0], Args: append([]string(nil), parts[1:]...)}
		values[integration.Name] = integration
		aliases[strings.ToLower(integration.Name.String())] = integration.Name
		for _, alias := range integration.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				return fmt.Errorf("configure ACP provider %q: invalid blank alias", integration.Name)
			}
			if existing, exists := aliases[alias]; exists && existing != integration.Name {
				return fmt.Errorf("configure ACP provider %q: alias %q collides with %q", integration.Name, alias, existing)
			}
			aliases[alias] = integration.Name
		}
	}

	service.mu.Lock()
	next := make(map[providers.ID]*daemon, len(commands))
	retired := make(map[providers.ID]*daemon)
	for id, command := range commands {
		if current, ok := service.daemons[id]; ok && current.matches(command) {
			next[id] = current
			continue
		}
		next[id] = newDaemon(command, service.newCommand, service.locator)
		if current := service.daemons[id]; current != nil {
			retired[id] = current
		}
	}
	for id, current := range service.daemons {
		if _, kept := next[id]; !kept {
			retired[id] = current
		}
	}
	service.daemons, service.integrations, service.aliases = next, values, aliases
	service.mu.Unlock()

	var first error
	for id, daemon := range retired {
		if err := daemon.close(ctx); err != nil && first == nil {
			first = fmt.Errorf("drain ACP provider %q: %w", id, err)
		}
	}
	return first
}

func (service *Service) Integrations() []providers.ACPIntegration {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := make([]providers.ACPIntegration, 0, len(service.integrations))
	for _, integration := range service.integrations {
		result = append(result, integration.Clone())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (service *Service) Resolve(id providers.ID) (providers.ID, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.resolveLocked(id)
}

func (service *Service) resolveLocked(id providers.ID) (providers.ID, bool) {
	canonical, ok := service.aliases[strings.ToLower(strings.TrimSpace(id.String()))]
	return canonical, ok
}

func newDaemon(command Command, newCommand platformprocess.CommandFactory, locator platformprocess.ExecutableLocator) *daemon {
	lifecycle, cancelLifecycle := context.WithCancel(context.Background())
	daemon := &daemon{command: Command{Name: command.Name, Args: append([]string(nil), command.Args...)}, newCommand: newCommand, locator: locator, gate: make(chan struct{}, 1), lifecycle: lifecycle, cancelLifecycle: cancelLifecycle}
	daemon.gate <- struct{}{}
	return daemon
}

func (daemon *daemon) matches(command Command) bool {
	return daemon.command.Name == command.Name && slices.Equal(daemon.command.Args, command.Args)
}

type daemon struct {
	gate            chan struct{}
	lifecycle       context.Context
	cancelLifecycle context.CancelFunc
	command         Command
	newCommand      platformprocess.CommandFactory
	locator         platformprocess.ExecutableLocator

	cmd         *exec.Cmd
	stdin       io.WriteCloser
	connection  *acpsdk.ClientSideConnection
	client      *client
	initialized acpsdk.InitializeResponse
	finished    chan error
	tree        platformprocess.SubprocessTree
	stderr      bytes.Buffer
}

func (daemon *daemon) execute(ctx context.Context, id providers.ID, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	executionCtx, cancelExecution := context.WithCancel(ctx)
	stopLifecycleWatch := context.AfterFunc(daemon.lifecycle, cancelExecution)
	defer func() {
		stopLifecycleWatch()
		cancelExecution()
	}()
	ctx = executionCtx
	if err := ctx.Err(); err != nil {
		return providers.ExecuteResult{}, nativeFailure(err)
	}
	select {
	case <-ctx.Done():
		return providers.ExecuteResult{}, nativeFailure(ctx.Err())
	case <-daemon.gate:
	}
	defer func() { daemon.gate <- struct{}{} }()

	cwd, err := absoluteWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return providers.ExecuteResult{}, invalidFailure(err)
	}
	prompt := []acpsdk.ContentBlock{}
	if text := strings.TrimSpace(request.SystemPrompt); text != "" {
		prompt = append(prompt, acpsdk.TextBlock("System instructions:\n"+text))
	}
	prompt = append(prompt, acpsdk.TextBlock(request.UserMessage))
	prompt = append(prompt, inputWorkBlocks(request.InputTokens, request.UserMessage)...)
	prompt = append(prompt, resourceLinks(request.InputTokens, request.UserMessage)...)

	if err := daemon.ensureStarted(ctx, id, cwd, requestEnvironment(request), request); err != nil {
		return providers.ExecuteResult{}, err
	}
	daemon.client.reset(request.SkipPermissions)
	client := daemon.client
	connection := daemon.connection
	initialized := daemon.initialized

	session, err := connection.NewSession(ctx, acpsdk.NewSessionRequest{Cwd: cwd, McpServers: []acpsdk.McpServer{}})
	if err != nil {
		var requestErr *acpsdk.RequestError
		if errors.As(err, &requestErr) && requestErr.Code == -32000 {
			// The current connection cannot serve work until the operator
			// authenticates. No login operation is exposed through this service,
			// so retaining the process only leaves an unusable daemon (and its
			// working directory) alive. Close it now and let a later execution
			// establish a fresh authenticated connection.
			_ = daemon.stopLocked(context.Background())
			return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: "ACP authentication required" + authenticationMethodHint(initialized.AuthMethods)}
		}
		daemon.invalidateDisconnected(ctx)
		return providers.ExecuteResult{}, rpcFailure(ctx, "session/new", id, err, daemon.stderr.String(), request)
	}
	daemon.client.setSessionID(string(session.SessionId))
	modelConfig, err := applyAdvertisedModel(ctx, connection, session, request.Model)
	if err != nil {
		daemon.invalidateDisconnected(ctx)
		return providers.ExecuteResult{}, withSessionRef(
			rpcFailure(ctx, "session/set_config_option", id, err, daemon.stderr.String(), request),
			id,
			string(session.SessionId),
		)
	}
	if _, err := connection.Prompt(ctx, acpsdk.PromptRequest{SessionId: session.SessionId, Prompt: prompt}); err != nil {
		if ctx.Err() != nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_ = connection.Cancel(cancelCtx, acpsdk.CancelNotification{SessionId: session.SessionId})
			cancel()
		}
		daemon.invalidateDisconnected(context.Background())
		return providers.ExecuteResult{}, withPartial(rpcFailure(ctx, "session/prompt", id, err, daemon.stderr.String(), request), client, id)
	}
	return providers.ExecuteResult{Content: client.content(), SessionRef: &providers.SessionRef{Provider: id, Kind: providers.SessionIDKind, ID: string(session.SessionId)}, Diagnostics: &providers.ExecuteDiagnostics{Progress: completedPromptProgress(client.progressFacts()), Metadata: map[string]string{"execution_kind": "acp", "protocol_version": fmt.Sprint(initialized.ProtocolVersion), "model_config": modelConfig}}}, nil
}

func completedPromptProgress(updates []providers.ExecuteProgress) []providers.ExecuteProgress {
	result := []providers.ExecuteProgress{{Phase: "started", Metadata: map[string]string{"kind": "run", "native_type": "session/prompt"}}}
	started := map[string]bool{}
	items := map[string]map[string]bool{}
	content := map[string]map[string]string{}
	for _, update := range updates {
		kind, itemID := update.Metadata["kind"], update.Metadata["item_id"]
		if (kind == "message" || kind == "reasoning") && !started[kind] {
			result = append(result, providers.ExecuteProgress{Phase: "started", Metadata: map[string]string{
				"kind": kind, "item_id": itemID, "native_type": "acp/synthetic_start",
			}})
			started[kind] = true
		}
		if kind != "" && itemID != "" {
			if items[kind] == nil {
				items[kind] = map[string]bool{}
			}
			if content[kind] == nil {
				content[kind] = map[string]string{}
			}
			items[kind][itemID] = update.Phase == "completed"
			content[kind][itemID] += update.Detail
		}
		result = append(result, update)
	}
	for _, kind := range []string{"reasoning", "tool", "session", "message"} {
		ids := make([]string, 0, len(items[kind]))
		for itemID := range items[kind] {
			ids = append(ids, itemID)
		}
		sort.Strings(ids)
		for _, itemID := range ids {
			completed := items[kind][itemID]
			if completed {
				continue
			}
			result = append(result, providers.ExecuteProgress{Phase: "completed", Detail: content[kind][itemID], Metadata: map[string]string{
				"kind": kind, "item_id": itemID, "native_type": "session/prompt",
			}})
		}
	}
	return append(result, providers.ExecuteProgress{Phase: "completed", Metadata: map[string]string{"kind": "run", "native_type": "session/prompt"}})
}

func (daemon *daemon) ensureStarted(ctx context.Context, id providers.ID, cwd string, environment []string, request providers.ExecuteRequest) error {
	if daemon.connection != nil {
		select {
		case <-daemon.finished:
			daemon.clearProcess()
		default:
			return nil
		}
	}
	if daemon.newCommand == nil {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "ACP command is unavailable"}
	}
	if daemon.locator != nil {
		if _, err := daemon.locator.LookPath(daemon.command.Name); err != nil {
			return providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindDependency,
				Message: fmt.Sprintf("ACP executable %q is unavailable", daemon.command.Name),
				Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
					"work-failure-type": "missing_executable",
				}},
			}
		}
	}
	cmd := daemon.newCommand(daemon.command.Name, daemon.command.Args...)
	if cmd == nil {
		return dependencyFailure("ACP command factory returned nil")
	}
	cmd.Dir, cmd.Env = cwd, append([]string(nil), environment...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return dependencyFailure(err.Error())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return dependencyFailure(err.Error())
	}
	daemon.stderr.Reset()
	cmd.Stderr = &daemon.stderr
	platformprocess.ConfigureSubprocessTree(cmd)
	if err := cmd.Start(); err != nil {
		return dependencyFailure(fmt.Sprintf("start ACP provider %q: %v", id, err))
	}
	tree, _ := platformprocess.AttachSubprocessTree(cmd)
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	client := &client{}
	connection := acpsdk.NewClientSideConnection(client, stdin, stdout)
	initialized, err := connection.Initialize(ctx, acpsdk.InitializeRequest{ProtocolVersion: acpsdk.ProtocolVersionNumber, ClientCapabilities: acpsdk.ClientCapabilities{}})
	if err != nil {
		daemon.cmd, daemon.stdin, daemon.finished, daemon.tree = cmd, stdin, finished, tree
		_ = daemon.stopLocked(context.Background())
		return rpcFailure(ctx, "initialize", id, err, daemon.stderr.String(), request)
	}
	if initialized.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		daemon.cmd, daemon.stdin, daemon.finished, daemon.tree = cmd, stdin, finished, tree
		_ = daemon.stopLocked(context.Background())
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindMisconfigured, Message: fmt.Sprintf("ACP provider %q negotiated unsupported protocol version %v", id, initialized.ProtocolVersion)}
	}
	daemon.cmd = cmd
	daemon.stdin = stdin
	daemon.connection = connection
	daemon.client = client
	daemon.initialized = initialized
	daemon.finished = finished
	daemon.tree = tree
	return nil
}

func (daemon *daemon) invalidateDisconnected(ctx context.Context) {
	if daemon.connection == nil {
		return
	}
	select {
	case <-daemon.connection.Done():
		_ = daemon.stopLocked(ctx)
	default:
	}
}

func (daemon *daemon) close(ctx context.Context) error {
	daemon.cancelLifecycle()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-daemon.gate:
	}
	defer func() { daemon.gate <- struct{}{} }()
	return daemon.stopLocked(ctx)
}

func (daemon *daemon) stopLocked(ctx context.Context) error {
	if daemon.cmd == nil {
		return nil
	}
	cmd, tree, finished := daemon.cmd, daemon.tree, daemon.finished
	_ = daemon.stdin.Close()
	var stopErr error
	exited := false
	select {
	case <-finished:
		exited = true
	case <-ctx.Done():
		stopErr = ctx.Err()
		_ = platformprocess.TerminateSubprocessTree(cmd, tree)
	case <-time.After(500 * time.Millisecond):
		_ = platformprocess.TerminateSubprocessTree(cmd, tree)
	}
	if !exited {
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			if stopErr == nil {
				stopErr = errors.New("ACP process did not exit after termination")
			}
		}
	}
	daemon.clearProcess()
	return stopErr
}

func (daemon *daemon) clearProcess() {
	if daemon.cmd != nil {
		platformprocess.CloseSubprocessTree(daemon.cmd, daemon.tree)
	}
	daemon.cmd = nil
	daemon.stdin = nil
	daemon.connection = nil
	daemon.client = nil
	daemon.finished = nil
	daemon.tree = platformprocess.SubprocessTree{}
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
	return providers.ExecuteFailure{Kind: kind, Message: message, Diagnostics: &providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{
		Phase: "failed", Detail: message, Metadata: map[string]string{
			"kind": "error", "native_type": method,
			"error_code": "ACP_" + strings.ToUpper(strings.ReplaceAll(method, "session/", "")) + "_FAILED",
		},
	}}}}
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

func inputWorkBlocks(values []any, renderedPrompt string) []acpsdk.ContentBlock {
	blocks := make([]acpsdk.ContentBlock, 0)
	seen := map[string]struct{}{}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var token struct {
			Color struct {
				Name    string `json:"name"`
				WorkID  string `json:"work_id"`
				Payload []byte `json:"payload"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"color"`
		}
		if json.Unmarshal(encoded, &token) != nil {
			continue
		}
		name := strings.TrimSpace(token.Color.Name)
		if name == "" {
			name = strings.TrimSpace(token.Color.WorkID)
		}
		for _, content := range token.Color.Content {
			text := strings.TrimSpace(content.Text)
			if !strings.EqualFold(content.Type, "text") || text == "" || strings.Contains(renderedPrompt, text) {
				continue
			}
			key := name + "\x00" + text
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if name != "" {
				text = "Input work " + name + ":\n" + text
			} else {
				text = "Input work:\n" + text
			}
			blocks = append(blocks, acpsdk.TextBlock(text))
		}
		payload := strings.TrimSpace(string(token.Color.Payload))
		if payload != "" && !strings.Contains(renderedPrompt, payload) {
			key := name + "\x00" + payload
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				if name != "" {
					payload = "Input work " + name + ":\n" + payload
				} else {
					payload = "Input work:\n" + payload
				}
				blocks = append(blocks, acpsdk.TextBlock(payload))
			}
		}
	}
	return blocks
}

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
func withSessionRef(err error, provider providers.ID, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return err
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		return err
	}
	failure = failure.Clone()
	if failure.SessionRef == nil {
		failure.SessionRef = &providers.SessionRef{
			Provider: provider,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return failure
}

func withPartial(err error, c *client, provider providers.ID) error {
	var f providers.ExecuteFailure
	if errors.As(err, &f) {
		f = f.Clone()
		var failureProgress []providers.ExecuteProgress
		if f.Diagnostics != nil {
			for _, progress := range f.Diagnostics.Progress {
				failureProgress = append(failureProgress, progress.Clone())
			}
		}
		progress := failedPromptProgress(c.progressFacts())
		progress = append(progress, failureProgress...)
		f.Diagnostics = &providers.ExecuteDiagnostics{Progress: progress, Metadata: map[string]string{"partial_content": c.content()}}
		if f.SessionRef == nil {
			f.SessionRef = c.sessionRef(provider)
		}
		return f
	}
	return err
}

func failedPromptProgress(updates []providers.ExecuteProgress) []providers.ExecuteProgress {
	progress := completedPromptProgress(updates)
	if len(progress) > 0 {
		progress = progress[:len(progress)-1]
	}
	for index := range progress {
		if progress[index].Phase != "completed" || progress[index].Metadata["kind"] != "message" {
			continue
		}
		progress[index].Metadata["partial"] = "true"
	}
	return progress
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

func (c *client) reset(skipPermissions bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skipPermissions = skipPermissions
	c.text.Reset()
	c.sessionID = ""
	c.progress = nil
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
func (c *client) sessionRef(provider providers.ID) *providers.SessionRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(c.sessionID) == "" {
		return nil
	}
	return &providers.SessionRef{Provider: provider, Kind: providers.SessionIDKind, ID: c.sessionID}
}
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
			if *update.ToolCallUpdate.Status == acpsdk.ToolCallStatusCompleted {
				phase = "completed"
			}
		}
		encodeMetadata(metadata, "raw_output", update.ToolCallUpdate.RawOutput)
		progress := []providers.ExecuteProgress{}
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
