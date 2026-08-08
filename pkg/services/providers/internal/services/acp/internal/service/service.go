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
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/internal/service/cancelwindow"
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

var _ acp.ContinuationService = (*Service)(nil)

func New(integrations []providers.ACPIntegration, newCommand platformprocess.CommandFactory, locator platformprocess.ExecutableLocator) (acp.ContinuationService, error) {
	service := &Service{newCommand: newCommand, locator: locator}
	if err := service.Configure(context.Background(), integrations); err != nil {
		return nil, err
	}
	return service, nil
}

// resolveDaemon resolves id (including an accepted alias) to its canonical
// identity and owning daemon under a single read lock. ok mirrors
// resolveLocked's own result; it does not additionally guarantee daemon is
// non-nil, matching Execute's pre-existing behavior. Claim additionally
// checks for a nil daemon itself.
func (service *Service) resolveDaemon(id providers.ID) (target *daemon, canonical providers.ID, ok bool) {
	service.mu.RLock()
	canonical, ok = service.resolveLocked(id)
	target = service.daemons[canonical]
	service.mu.RUnlock()
	return target, canonical, ok
}

func (service *Service) Execute(ctx context.Context, id providers.ID, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	daemon, canonical, ok := service.resolveDaemon(id)
	if !ok {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: fmt.Sprintf("ACP provider %q is unavailable", id)}
	}
	return daemon.execute(ctx, canonical, request, nil)
}

// Continue resumes the exact private Providers session reference without
// permitting an ordinary Execute caller to select a prior session.
func (service *Service) Continue(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
	reference providers.SessionRef,
) (providers.ExecuteResult, error) {
	daemon, canonical, ok := service.resolveDaemon(id)
	if !ok {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: fmt.Sprintf("ACP provider %q is unavailable", id)}
	}
	return daemon.execute(ctx, canonical, request, &reference)
}

// NegotiatedCapabilities returns id's daemon's last successfully negotiated
// AgentCapabilities without starting a connection or touching the daemon's
// execute gate.
func (service *Service) NegotiatedCapabilities(id providers.ID) (acpsdk.AgentCapabilities, bool) {
	target, _, ok := service.resolveDaemon(id)
	if !ok || target == nil {
		return acpsdk.AgentCapabilities{}, false
	}
	return target.negotiatedCapabilities()
}

// Claim atomically captures the exact live cancel-window generation named by
// id/attemptID, if id's daemon currently has an established session/prompt
// turn in flight for it.
func (service *Service) Claim(id providers.ID, attemptID string) (acp.Generation, bool) {
	target, _, ok := service.resolveDaemon(id)
	if !ok || target == nil {
		return nil, false
	}
	return target.window.Claim(attemptID)
}

// TryCancel delivers a session/cancel notification to the exact generation
// captured by a prior Claim call and blocks (bounded by ctx) until it
// observes that generation's real recorded terminal outcome. It never
// re-resolves generation by identity strings, so it cannot be redirected to
// a different generation that later reuses the same provider/attemptID.
func (service *Service) TryCancel(ctx context.Context, generation acp.Generation) (bool, error) {
	session, ok := generation.(*cancelwindow.Session)
	if !ok || session == nil {
		return false, nil
	}
	return session.TryCancel(ctx)
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
	stderr      syncBuffer

	window cancelwindow.Window

	// negotiatedMu guards negotiated/negotiatedKnown separately from gate so
	// a capability read never blocks on (or is blocked by) an in-flight
	// execute call holding the gate for the duration of a live attempt.
	negotiatedMu    sync.RWMutex
	negotiated      acpsdk.AgentCapabilities
	negotiatedKnown bool
}

// negotiatedCapabilities returns the AgentCapabilities from the daemon's
// last successful initialize handshake, without touching gate or causing any
// connection side effect. ok is false until the first successful handshake.
func (daemon *daemon) negotiatedCapabilities() (acpsdk.AgentCapabilities, bool) {
	daemon.negotiatedMu.RLock()
	defer daemon.negotiatedMu.RUnlock()
	return daemon.negotiated, daemon.negotiatedKnown
}

func (daemon *daemon) recordNegotiated(capabilities acpsdk.AgentCapabilities) {
	daemon.negotiatedMu.Lock()
	daemon.negotiated = capabilities
	daemon.negotiatedKnown = true
	daemon.negotiatedMu.Unlock()
}

func (daemon *daemon) execute(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
	resume *providers.SessionRef,
) (providers.ExecuteResult, error) {
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
	prompt := promptBlocks(request)

	if err := daemon.ensureStarted(ctx, id, cwd, requestEnvironment(request), request); err != nil {
		return providers.ExecuteResult{}, err
	}
	daemon.client.reset(request.SkipPermissions, request.ProgressObserver)
	client := daemon.client
	connection := daemon.connection
	initialized := daemon.initialized

	session, err := daemon.openSession(ctx, id, cwd, initialized, request, resume)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	daemon.client.setSessionID(string(session.SessionId))
	request.ObserveSession(providers.SessionRef{
		Provider: id,
		Kind:     providers.SessionIDKind,
		ID:       string(session.SessionId),
	})
	modelConfig, err := applyAdvertisedModel(ctx, connection, session, request.Model)
	if err != nil {
		daemon.invalidateDisconnected(ctx)
		return providers.ExecuteResult{}, withSessionRef(
			rpcFailure(ctx, "session/set_config_option", id, err, daemon.stderr.String(), request),
			id,
			string(session.SessionId),
		)
	}
	response, err := daemon.promptWithWindow(request.AttemptID, session.SessionId, connection, func() (acpsdk.PromptResponse, error) {
		return connection.Prompt(ctx, acpsdk.PromptRequest{SessionId: session.SessionId, Prompt: prompt})
	})
	if err != nil {
		if ctx.Err() != nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_ = connection.Cancel(cancelCtx, acpsdk.CancelNotification{SessionId: session.SessionId})
			cancel()
		}
		daemon.invalidateDisconnected(context.Background())
		return providers.ExecuteResult{}, withPartial(rpcFailure(ctx, "session/prompt", id, err, daemon.stderr.String(), request), client, id)
	}
	if response.StopReason == acpsdk.StopReasonCancelled {
		return providers.ExecuteResult{}, withPartial(acpControlCanceledFailure(id), client, id)
	}
	return providers.ExecuteResult{Content: client.content(), SessionRef: &providers.SessionRef{Provider: id, Kind: providers.SessionIDKind, ID: string(session.SessionId)}, Diagnostics: &providers.ExecuteDiagnostics{Progress: client.completeProgress(), ProgressAlreadyObserved: request.ProgressObserver != nil, Metadata: map[string]string{"execution_kind": "acp", "protocol_version": fmt.Sprint(initialized.ProtocolVersion), "model_config": modelConfig, "completion_evidence": "provider_response"}}}, nil
}

// openSession resumes the exact private Provider Session reference through the native ACP
// session/load method with its exact opaque id forwarded unchanged, and its
// absence starts an ordinary fresh session/new. openSession never falls back
// from a requested resume to a fresh session - a session/load failure is
// returned as-is so a continuation can never silently adopt a different
// Provider Session.
func (daemon *daemon) openSession(
	ctx context.Context,
	id providers.ID,
	cwd string,
	initialized acpsdk.InitializeResponse,
	request providers.ExecuteRequest,
	resume *providers.SessionRef,
) (acpsdk.NewSessionResponse, error) {
	connection := daemon.connection
	if resume != nil {
		if resumeID := strings.TrimSpace(resume.ID); resumeID != "" {
			loaded, err := connection.LoadSession(ctx, acpsdk.LoadSessionRequest{SessionId: acpsdk.SessionId(resumeID), Cwd: cwd, McpServers: []acpsdk.McpServer{}})
			if err != nil {
				return acpsdk.NewSessionResponse{}, daemon.sessionOpenFailure(ctx, id, "session/load", err, initialized, request)
			}
			return acpsdk.NewSessionResponse{Meta: loaded.Meta, ConfigOptions: loaded.ConfigOptions, Modes: loaded.Modes, SessionId: acpsdk.SessionId(resumeID)}, nil
		}
	}
	session, err := connection.NewSession(ctx, acpsdk.NewSessionRequest{Cwd: cwd, McpServers: []acpsdk.McpServer{}})
	if err != nil {
		return acpsdk.NewSessionResponse{}, daemon.sessionOpenFailure(ctx, id, "session/new", err, initialized, request)
	}
	return session, nil
}

const (
	// acpErrorCodeResourceNotFound is the ACP-reserved JSON-RPC error code
	// (schema ErrorCodeResourceNotFound) an agent returns when a referenced
	// resource, including a session/load id, is not recognized.
	acpErrorCodeResourceNotFound = -32002

	// JSON-RPC reserves this inclusive range for server errors. ACP agents use
	// it for provider-side failures that are distinct from the standard JSON-RPC
	// protocol errors such as -32603.
	acpServerErrorMinimum = -32099
	acpServerErrorMaximum = -32000
)

// sessionOpenFailure normalizes a session/new or session/load failure. An
// authentication-required failure closes the unusable daemon so a later
// execution establishes a fresh authenticated connection. A session/load
// failure reporting ResourceNotFound means the daemon itself is healthy but
// does not recognize the exact requested Provider Session id, so it is
// reported as ExecuteFailureKindSessionNotFound instead of the generic RPC
// failure Continue would otherwise be unable to distinguish from any other
// dependency failure. Every other failure is reported through the same
// rpcFailure normalization every other ACP RPC failure in this service uses.
func (daemon *daemon) sessionOpenFailure(
	ctx context.Context,
	id providers.ID,
	method string,
	err error,
	initialized acpsdk.InitializeResponse,
	request providers.ExecuteRequest,
) error {
	var requestErr *acpsdk.RequestError
	if errors.As(err, &requestErr) && requestErr.Code == -32000 {
		// The current connection cannot serve work until the operator
		// authenticates. No login operation is exposed through this service,
		// so retaining the process only leaves an unusable daemon (and its
		// working directory) alive. Close it now and let a later execution
		// establish a fresh authenticated connection.
		_ = daemon.stopLocked(context.Background())
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: "ACP authentication required" + authenticationMethodHint(initialized.AuthMethods)}
	}
	if method == "session/load" && requestErr != nil && requestErr.Code == acpErrorCodeResourceNotFound {
		daemon.invalidateDisconnected(ctx)
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindSessionNotFound,
			Message: fmt.Sprintf("ACP provider %q does not recognize the referenced Provider Session as live", id),
		}
	}
	daemon.invalidateDisconnected(ctx)
	return rpcFailure(ctx, method, id, err, daemon.stderr.String(), request)
}

// promptWithWindow opens the cancelable window for attemptID, runs prompt,
// and closes the window with the real recorded outcome - via defer, so an
// unexpected prompt unwind (including a panic) still closes the window
// instead of leaving it stale. A stale window would let a later execution
// that reuses this same attempt ID bind at the root and have a racing
// control claim, or hang indefinitely on, this dead session (see
// cancelwindow's package doc). response/err are read by the deferred closure
// only after the assignment below completes, so a normal return still
// records the actual outcome.
func (daemon *daemon) promptWithWindow(
	attemptID string,
	sessionID acpsdk.SessionId,
	connection *acpsdk.ClientSideConnection,
	prompt func() (acpsdk.PromptResponse, error),
) (response acpsdk.PromptResponse, err error) {
	cancelable := daemon.window.Begin(attemptID, sessionID, connection)
	defer func() {
		daemon.window.End(cancelable, err == nil && response.StopReason == acpsdk.StopReasonCancelled)
	}()
	response, err = prompt()
	return response, err
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
	daemon.recordNegotiated(initialized.AgentCapabilities)
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
	var requestErr *acpsdk.RequestError
	if errors.As(err, &requestErr) && isACPServerFailure(requestErr.Code) {
		// A server-side ACP failure is a provider dependency outcome. Workers
		// classifies that outcome as retryable and, when a session was opened,
		// retains the exact Provider Session for the retry continuation.
		kind = providers.ExecuteFailureKindDependency
	}
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

func isACPServerFailure(code int) bool {
	return code >= acpServerErrorMinimum && code <= acpServerErrorMaximum &&
		code != -32000 && code != acpErrorCodeResourceNotFound
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

// promptBlocks assembles one session/prompt turn's content blocks: an
// optional system-instructions block, the user message, then any input Work
// content and resource links derived from it.
func promptBlocks(request providers.ExecuteRequest) []acpsdk.ContentBlock {
	prompt := []acpsdk.ContentBlock{}
	if text := strings.TrimSpace(request.SystemPrompt); text != "" {
		prompt = append(prompt, acpsdk.TextBlock("System instructions:\n"+text))
	}
	prompt = append(prompt, acpsdk.TextBlock(request.UserMessage))
	prompt = append(prompt, inputWorkBlocks(request.InputTokens, request.UserMessage)...)
	prompt = append(prompt, resourceLinks(request.InputTokens, request.UserMessage)...)
	return prompt
}

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
		progress := c.failProgress(failureProgress...)
		f.Diagnostics = &providers.ExecuteDiagnostics{
			Progress:                progress,
			ProgressAlreadyObserved: c.streamedProgress(),
			Metadata:                map[string]string{"partial_content": c.content()},
		}
		if f.SessionRef == nil {
			f.SessionRef = c.sessionRef(provider)
		}
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
	stream          *promptProgressStream
}

// reset begins one turn's progress stream. observe may be nil, in which case
// the turn still normalizes its facts but delivers them only in the returned
// diagnostics.
func (c *client) reset(skipPermissions bool, observe providers.ProgressObserver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skipPermissions = skipPermissions
	c.text.Reset()
	c.sessionID = ""
	c.stream = newPromptProgressStream(observe)
}

func (c *client) SessionUpdate(_ context.Context, n acpsdk.SessionNotification) error {
	p, text := mapSessionUpdate(n.Update)
	c.mu.Lock()
	if text != "" {
		c.text.WriteString(text)
	}
	stream := c.stream
	var pending []providers.ExecuteProgress
	if stream != nil {
		stream.Observe(p)
		pending = stream.takePending()
	}
	c.mu.Unlock()
	// Handed off after the lock is released. This callback runs on the ACP
	// connection's notification path, and the SDK holds a turn's
	// session/prompt response until every pre-response notification handler
	// returns, so downstream publishing must not happen inline here.
	if stream != nil {
		stream.Deliver(pending)
	}
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

// completeProgress closes the turn normally and returns its full ordered fact
// list, which is exactly the sequence already streamed to the observer.
func (c *client) completeProgress() []providers.ExecuteProgress {
	return c.closeProgress(func(stream *promptProgressStream) []providers.ExecuteProgress {
		return stream.Complete()
	})
}

// failProgress closes a turn that never reached its own completed marker,
// folding the transport's own failure diagnostics into the same stream so the
// returned slice still matches what was observed.
func (c *client) failProgress(extra ...providers.ExecuteProgress) []providers.ExecuteProgress {
	return c.closeProgress(func(stream *promptProgressStream) []providers.ExecuteProgress {
		return stream.Fail(extra...)
	})
}

// closeProgress runs one turn-closing step under the client lock, then hands
// off and joins delivery outside it, so every fact this turn reports has
// reached the observer by the time the caller inspects the returned slice.
func (c *client) closeProgress(
	finish func(*promptProgressStream) []providers.ExecuteProgress,
) []providers.ExecuteProgress {
	c.mu.Lock()
	stream := c.stream
	var facts, pending []providers.ExecuteProgress
	if stream != nil {
		facts = finish(stream)
		pending = stream.takePending()
	}
	c.mu.Unlock()
	if stream == nil {
		return nil
	}
	stream.Deliver(pending)
	stream.close()
	return facts
}

// streamedProgress reports whether this turn delivered its facts live, so a
// caller knows not to publish the returned slice a second time.
func (c *client) streamedProgress() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream != nil && c.stream.observe != nil
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
					// ACP models a diff as content inside the tool call that
					// produced it. Carrying the owning call's id keeps that
					// ownership, so a consumer can attach the change to its
					// tool call instead of presenting an orphaned edit.
					"tool_call_id": string(update.ToolCallUpdate.ToolCallId),
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
		phase = "updated"
		itemID = "session"
		if update.SessionInfoUpdate.Title != nil {
			detail = *update.SessionInfoUpdate.Title
			metadata["title_present"] = "true"
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

// syncBuffer is a bytes.Buffer guarded for concurrent use.
//
// The daemon hands its stderr buffer to os/exec, which copies the child's
// stderr on its own goroutine for the process' whole lifetime, while the
// executing goroutine reads the accumulated text whenever it builds a failure
// diagnostic. Those are genuinely concurrent, so the buffer cannot be a bare
// bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
