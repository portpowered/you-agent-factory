package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workcontent/materialize"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	cursorpkg "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
)

// Provider abstracts LLM inference calls. Implementations handle the
// specifics of communicating with a particular model provider.
type Provider interface {
	// Infer sends a prompt to the model and returns the raw text response.
	// The request carries system prompt, user message, output schema,
	// and execution-level settings (model, working directory, env vars).
	Infer(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error)
}

// DispatcherType selects which CLI tool the ScriptWrapProvider invokes.
type DispatcherType string

const (
	// DispatcherClaude runs the Claude Code CLI ("claude").
	DispatcherClaude DispatcherType = "claude"
	// DispatcherCodex runs the OpenAI Codex CLI ("codex").
	DispatcherCodex DispatcherType = "codex"
)

// ScriptWrapProviderOption configures a ScriptWrapProvider.
type ScriptWrapProviderOption func(*ScriptWrapProvider)

const (
	providerSessionKindSessionID      = "session_id"
	providerSessionKindConversationID = "conversation_id"
	providerSessionKindResponseID     = "response_id"
)

var providerAutomationEnvDefaults = []workerprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

var providerSessionPatterns = []struct {
	kind    string
	pattern *regexp.Regexp
}{
	{
		kind:    providerSessionKindSessionID,
		pattern: regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(session_id|sessionid|session id)\s*["=: ]+\s*"?([a-z0-9][a-z0-9._:-]*)"?`),
	},
	{
		kind:    providerSessionKindConversationID,
		pattern: regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(conversation_id|conversationid|conversation id)\s*["=: ]+\s*"?([a-z0-9][a-z0-9._:-]*)"?`),
	},
	{
		kind:    providerSessionKindResponseID,
		pattern: regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(response_id|responseid|response id)\s*["=: ]+\s*"?([a-z0-9][a-z0-9._:-]*)"?`),
	},
}

// WithSkipPermissions enables the dangerously-skip-permissions flag.
func WithSkipPermissions(skip bool) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.SkipPermissions = skip
	}
}

// WithProviderLogger sets the structured logger for inference diagnostics.
func WithProviderLogger(l logging.Logger) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.Logger = l
	}
}

func WithProviderCommandRunner(runner CommandRunner) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.exec = runner
	}
}

// WithMaterializeOptions configures dispatch-time content URL materialization (used by Codex image args).
func WithMaterializeOptions(opts *materialize.Options) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.MaterializeOptions = opts
	}
}

// ScriptWrapProvider implements Provider by shelling out to a CLI tool
// (Claude Code or Codex) as a subprocess. It supports configurable
// dispatchers and skip-permissions.
type ScriptWrapProvider struct {
	// SkipPermissions enables --dangerously-skip-permissions (claude) or
	// --full-auto (codex).
	SkipPermissions bool
	// MaterializeOptions configures dispatch-time content URL materialization for Codex image args.
	MaterializeOptions *materialize.Options
	// Logger is the structured logger for inference diagnostics. Nil disables logging.
	Logger logging.Logger
	exec   CommandRunner
}

func (p *ScriptWrapProvider) commandExec() CommandRunner {
	if p.exec != nil {
		return workerprocess.CommandRunnerWithLogging(p.exec, p.Logger)
	}
	return workerprocess.CommandRunnerWithLogging(workerprocess.ExecCommandRunner{}, p.Logger)
}

// NewScriptWrapProvider creates a ScriptWrapProvider with functional options.
func NewScriptWrapProvider(opts ...ScriptWrapProviderOption) *ScriptWrapProvider {
	p := &ScriptWrapProvider{
		Logger: logging.NoopLogger{},
		exec:   workerprocess.ExecCommandRunner{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Possible errors
// TODO: add retries?
// errors: https://platform.claude.com/docs/en/api/errors
// {"dispatcher": "claude", "error": "exit status 1", "output": "API Error: 500 {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"Internal server error\"},\"request_id\":\"req_011CZhAfuooABjwfNx9wrdQ7\"}\n", "stderr": ""}
// Rate limit, need exponential backoff, for our case, we just want it to wait for 5 hours or something.
// permission error, should be handled by entire failure
// authentication error, should be handled as misconfiguration and fail server
// api_error, should be handled by retry + exponential backoff
// 400 invalid_reuqest_error -> we should fail the request item.
// need to decleare new error types structs int he interfaces package, and have the service handle variosu failures

// Infer shells out to the configured CLI dispatcher with the user message.
// It merges req.EnvVars into the subprocess environment.
func (p *ScriptWrapProvider) Infer(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	return p.Execute(ctx, req)
}

// Execute implements the shared runner contract while preserving the current
// provider-backed subprocess execution path.
func (p *ScriptWrapProvider) Execute(ctx context.Context, req interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	logger := logging.EnsureLogger(p.Logger)

	logger.Info("inferencer: request starting",
		workLogFields(req.Dispatch.Execution,
			"dispatcher", string(req.ModelProvider),
			"model", req.Model)...)

	behavior := providerBehaviorFor(req.ModelProvider, logger)
	buildCtx := &ProviderBuildContext{
		ContentCache:    materialize.NewDispatchCache(),
		MaterializeOpts: p.MaterializeOptions,
	}
	defer buildCtx.ContentCache.Release()
	args, err := behavior.BuildArgs(ctx, req, p.SkipPermissions, buildCtx)
	if err != nil {
		logger.Error("inferencer: request argument validation failed",
			workLogFields(req.Dispatch.Execution,
				"dispatcher", string(req.ModelProvider),
				"error", err.Error())...)
		return interfaces.InferenceResponse{}, newProviderErrorWithDiagnostics(
			interfaces.WorkFailureTypePermanentBadRequest,
			err.Error(),
			err,
			nil,
			workDiagnosticsForInferenceRequest(req),
		)
	}
	logger.Info("inferencer: request arguments",
		workLogFields(req.Dispatch.Execution, "arguments", args)...)
	execReq := behavior.BuildCommandRequest(req, args)

	logger.Debug("inferencer: request input",
		workLogFields(req.Dispatch.Execution, "request", req.UserMessage)...)
	started := time.Now()
	result, err := p.commandExec().Run(ctx, execReq)
	duration := time.Since(started)
	commandDiagnostics := commandDiagnostics(execReq, result, duration, false)
	providerSession := effectiveProviderSession(req, result)
	cursorProvider := req.ModelProvider == string(interfaces.ModelProviderCursor)
	if err != nil {
		logger.Error("inferencer: request failed",
			cursorFailureLogFields(req, cursorProvider, result,
				"error", err.Error())...)
		return interfaces.InferenceResponse{}, normalizeProviderExecutionError(
			req.ModelProvider, result, err, providerSession,
			cursorInferenceFailureDiagnostics(cursorProvider, commandDiagnostics, result),
		)
	}
	if result.ExitCode != 0 {
		logger.Error("inferencer: request failed",
			cursorFailureLogFields(req, cursorProvider, result,
				"exit_code", result.ExitCode)...)
		return interfaces.InferenceResponse{}, normalizeProviderExitFailure(
			req.ModelProvider, result, providerSession,
			cursorInferenceFailureDiagnostics(cursorProvider, commandDiagnostics, result),
		)
	}

	if cursorProvider {
		return p.completeCursorInference(req, result, commandDiagnostics, logger)
	}

	content := string(result.Stdout)
	logger.Debug("inference results:",
		workLogFields(req.Dispatch.Execution, "output", result.Stdout)...)
	logger.Info("inferencer: request completed",
		workLogFields(req.Dispatch.Execution,
			"dispatcher", string(req.ModelProvider),
			"output_len", len(content))...)

	return interfaces.InferenceResponse{
		Content:         content,
		ProviderSession: providerSession,
		Diagnostics:     commandDiagnostics,
	}, nil
}

// ContainsStopToken checks whether the output text contains the given stop token.
// The check is case-sensitive and looks for the token as a substring.
// This is extracted as a pure function for independent unit testing.
func ContainsStopToken(output, stopToken string) bool {
	if stopToken == "" {
		return false
	}
	return strings.Contains(output, stopToken)
}

// buildProviderEnv merges subprocess environment sources with deterministic
// precedence: process environment, provider env vars, then automation defaults.
func buildProviderEnv(envVars map[string]string) []string {
	return workerprocess.MergeCommandEnv(os.Environ(), workerprocess.CommandEnvEntriesFromMap(envVars), providerAutomationEnvDefaults)
}

// TODO: right now the stderr/stdout for the print prints out the entire response log for the stdout....
// We don't necessarily want that....
//
//Failed     process              14:03:20   14:06:29   3m8s     prd-endpoint-state-panels prd-endpoint-state-panels provider error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)
//--------

func formatProviderExitFailure(provider string, result CommandResult) string {
	return providerBehaviorForErrorClassification(provider).FormatExitFailure(provider, result)
}

func normalizeProviderExecutionError(provider string, result CommandResult, err error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	switch {
	case isProviderExecutionTimeout(err, result):
		if diagnostics != nil && diagnostics.Command != nil {
			diagnostics.Command.TimedOut = true
		}
		message := formatProviderTimeoutFailure(provider, result)
		return newProviderErrorWithDiagnostics(interfaces.WorkFailureTypeTimeout, message, err, session, diagnostics)
	case errors.Is(err, exec.ErrNotFound):
		message := formatProviderCommandFailure(provider, result, err)
		return newProviderErrorWithDiagnostics(interfaces.WorkFailureTypeMisconfigured, message, err, session, diagnostics)
	default:
		message := formatProviderCommandFailure(provider, result, err)
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return newProviderErrorWithDiagnostics(interfaces.WorkFailureTypeMisconfigured, message, err, session, diagnostics)
		}
		return newProviderErrorWithDiagnostics(interfaces.WorkFailureTypeUnknown, message, err, session, diagnostics)
	}
}

func normalizeProviderExitFailure(provider string, result CommandResult, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	message := formatProviderExitFailure(provider, result)
	errorType := classifyProviderExitFailure(provider, result)
	return newProviderErrorWithDiagnostics(errorType, message, nil, session, diagnostics)
}

// NormalizeProviderExitFailure exposes the canonical provider exit-failure
// normalization path for compatibility shims and behavior-focused tests.
func NormalizeProviderExitFailure(provider string, result CommandResult, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	return normalizeProviderExitFailure(provider, result, session, diagnostics)
}

func classifyProviderExitFailure(provider string, result CommandResult) interfaces.WorkFailureType {
	return providerBehaviorForErrorClassification(provider).ClassifyExitFailure(result)
}

func isProviderExecutionTimeout(err error, result CommandResult) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) && providerOutputContainsTimeout(result) {
		return true
	}
	return false
}

func providerOutputContainsTimeout(result CommandResult) bool {
	normalizedOutput := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(string(result.Stderr)),
		strings.TrimSpace(string(result.Stdout)),
	}, "\n"))
	return containsAny(normalizedOutput,
		"deadline exceeded",
		"context deadline",
		"execution timeout",
		"command timeout",
		"command timed out",
		"provider timeout",
		"request timeout",
		"request timed out",
		"timed out",
		"timeout",
	)
}

func formatProviderTimeoutFailure(provider string, result CommandResult) string {
	return providerBehaviorForErrorClassification(provider).FormatTimeoutFailure(result)
}

func cursorFailureLogFields(req interfaces.RunnerExecutionRequest, cursorProvider bool, result CommandResult, extra ...any) []any {
	fields := workLogFields(req.Dispatch.Execution,
		append([]any{"dispatcher", string(req.ModelProvider)}, extra...)...)
	if !cursorProvider {
		fields = append(fields,
			"output", string(result.Stdout),
			"stderr", string(result.Stderr),
		)
		return fields
	}
	fields = append(fields,
		"stdout_preview", cursorpkg.BoundedCommandOutputExcerpt(result.Stdout, cursorpkg.CommandOutputLogPreviewLimit),
		"stderr_preview", cursorpkg.BoundedCommandOutputExcerpt(result.Stderr, cursorpkg.CommandOutputLogPreviewLimit),
	)
	return fields
}

func cursorInferenceFailureDiagnostics(
	cursorProvider bool,
	commandDiagnostics *interfaces.WorkDiagnostics,
	result CommandResult,
) *interfaces.WorkDiagnostics {
	if !cursorProvider {
		return commandDiagnostics
	}
	return cursorpkg.WithCommandOutputExcerpts(commandDiagnostics, result.Stdout, result.Stderr)
}

func (p *ScriptWrapProvider) completeCursorInference(
	req interfaces.RunnerExecutionRequest,
	result CommandResult,
	commandDiagnostics *interfaces.WorkDiagnostics,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	parsed, parseErr := cursorpkg.ParseInferenceResult(req.ModelProvider, result.Stdout)
	if parseErr != nil {
		logger.Error("inferencer: cursor JSON parse failed",
			cursorFailureLogFields(req, true, result,
				"error", parseErr.Message)...)
		failureDiagnostics := cursorpkg.WithCommandOutputExcerpts(commandDiagnostics, result.Stdout, result.Stderr)
		return interfaces.InferenceResponse{}, newProviderErrorWithDiagnostics(
			parseErr.Type,
			parseErr.Message,
			parseErr.Cause,
			effectiveProviderSession(req, result),
			failureDiagnostics,
		)
	}
	diagnostics := cursorpkg.WithResponseMetadata(commandDiagnostics, parsed.ResponseMetadata)
	logger.Debug("inference results:",
		workLogFields(req.Dispatch.Execution, "output", parsed.Content)...)
	logger.Info("inferencer: request completed",
		workLogFields(req.Dispatch.Execution,
			"dispatcher", string(req.ModelProvider),
			"output_len", len(parsed.Content),
			"session_id", parsed.ProviderSession.ID)...)
	return interfaces.InferenceResponse{
		Content:         parsed.Content,
		ProviderSession: parsed.ProviderSession,
		Diagnostics:     diagnostics,
	}, nil
}

func formatProviderCommandFailure(provider string, result CommandResult, err error) string {
	message := fmt.Sprintf("%s exited with error: %v", provider, err)
	if stderr := strings.TrimSpace(string(result.Stderr)); stderr != "" {
		message += fmt.Sprintf(": stderr: %s", stderr)
	}
	if stdout := strings.TrimSpace(string(result.Stdout)); stdout != "" {
		message += fmt.Sprintf("; stdout: %s", stdout)
	}
	return message
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func providerSessionFromCommandResult(provider string, result CommandResult) *interfaces.ProviderSessionMetadata {
	combined := strings.Join([]string{
		string(result.Stdout),
		string(result.Stderr),
	}, "\n")
	for _, candidate := range providerSessionPatterns {
		matches := candidate.pattern.FindStringSubmatch(combined)
		if len(matches) < 3 {
			continue
		}
		identifier := strings.Trim(matches[2], "\"' \t\r\n")
		if identifier == "" {
			continue
		}
		return &interfaces.ProviderSessionMetadata{
			Provider: interfaces.CanonicalProviderSessionProvider(provider),
			Kind:     candidate.kind,
			ID:       identifier,
		}
	}

	return nil
}

func effectiveProviderSession(req interfaces.ProviderInferenceRequest, result CommandResult) *interfaces.ProviderSessionMetadata {
	session := providerSessionFromCommandResult(req.ModelProvider, result)
	if session != nil {
		return session
	}
	if (req.ModelProvider == string(interfaces.ModelProviderClaude) || req.ModelProvider == string(interfaces.ModelProviderCursor) || req.ModelProvider == string(interfaces.ModelProviderOpenCode)) && req.SessionID != "" {
		return &interfaces.ProviderSessionMetadata{
			Provider: interfaces.CanonicalProviderSessionProvider(req.ModelProvider),
			Kind:     providerSessionKindSessionID,
			ID:       req.SessionID,
		}
	}
	return nil
}

var _ Provider = (*ScriptWrapProvider)(nil)

const (
	ProgressFragmentKind         = "PROGRESS_FRAGMENT"
	ResponseFragmentKind         = "RESPONSE_FRAGMENT"
	NormalizedEventTypeUnknown   = "UNKNOWN"
	NormalizedEventTypeStarted   = "STARTED"
	NormalizedEventTypeProgress  = "PROGRESS"
	NormalizedEventTypeTextDelta = "TEXT_DELTA"
	NormalizedEventTypeFinalText = "FINAL_TEXT"
	NormalizedEventTypeFailed    = "FAILED"
	NormalizedEventTypeCanceled  = "CANCELED"
)

const (
	codexRetainedTextBytes      = 4096
	codexRetainedProgressBytes  = 1024
	codexMetadataRunnerIDKey    = "runner_id"
	codexMetadataWorkIDKey      = "work_id"
	codexMetadataWorkstationKey = "workstation_name"
)

// InferenceProgressFragment is the provider-boundary shape for transient internal
// session progress that must not enter canonical factory event history.
type InferenceProgressFragment struct {
	DispatchID         string
	Kind               string
	Type               string
	Payload            string
	ProviderSessionRef *interfaces.ProviderSessionMetadata
	ExternalEventType  string
	Metadata           map[string]string
}

// InferenceProgressPublisher receives provider progress fragments for one live
// Factory Session internal response stream.
type InferenceProgressPublisher func(fragment InferenceProgressFragment)

// ProgressFragment builds one ordered progress fragment for a dispatch.
func ProgressFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ProgressFragmentKind,
		Type:               NormalizedEventTypeProgress,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// ResponseFragment builds one ordered response fragment for a dispatch.
func ResponseFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ResponseFragmentKind,
		Type:               NormalizedEventTypeTextDelta,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// InferenceProgressPublishingCommandRunner publishes internal response-stream
// fragments while provider subprocess stdout/stderr grow.
type InferenceProgressPublishingCommandRunner struct {
	Publisher InferenceProgressPublisher
	Logger    logging.Logger
}

// Run executes the provider subprocess and publishes incremental stdout/stderr
// fragments into the configured internal session response stream.
func (r InferenceProgressPublishingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Publisher == nil {
		return workerprocess.ExecCommandRunner{}.Run(ctx, req)
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	var cursorStream *cursorpkg.StreamParser
	if strings.TrimSpace(req.Command) == string(interfaces.ModelProviderCursor) {
		cursorStream = cursorpkg.NewStreamParser(req.Command, func(fragment cursorpkg.StreamFragment) {
			switch fragment.Kind {
			case cursorpkg.StreamFragmentKindResponse:
				r.Publisher(ResponseFragment(dispatchID, fragment.ProviderSession, fragment.Payload))
			case cursorpkg.StreamFragmentKindProgress:
				r.Publisher(ProgressFragment(dispatchID, fragment.ProviderSession, fragment.Payload))
			}
		})
	}
	normalizer := newCommandOutputNormalizer(req, r.Publisher)
	observer := func(stream string, chunk []byte) {
		if len(chunk) == 0 {
			return
		}
		if cursorStream != nil && stream == workerprocess.OutputStreamStdout {
			cursorStream.Consume(chunk)
			return
		}
		if normalizer != nil && normalizer.Observe(stream, chunk) {
			return
		}
		payload := string(chunk)
		switch stream {
		case workerprocess.OutputStreamStdout:
			r.Publisher(ResponseFragment(dispatchID, nil, payload))
		case workerprocess.OutputStreamStderr:
			r.Publisher(ProgressFragment(dispatchID, nil, payload))
		}
	}
	result, err := workerprocess.StreamingExecCommandRunner{
		Observer: observer,
		Logger:   logging.EnsureLogger(r.Logger),
	}.Run(ctx, req)
	if cursorStream != nil {
		cursorStream.Flush()
	}
	if normalizer != nil {
		normalizer.Flush()
	}
	return result, err
}

type commandOutputNormalizer interface {
	Observe(stream string, chunk []byte) bool
	Flush()
}

func newCommandOutputNormalizer(req CommandRequest, publisher InferenceProgressPublisher) commandOutputNormalizer {
	if !isCodexCommand(req.Command) {
		return nil
	}
	return &codexCommandOutputNormalizer{
		req:       interfaces.CloneSubprocessExecutionRequest(req),
		publisher: publisher,
		hasher:    sha256.New(),
	}
}

type codexCommandOutputNormalizer struct {
	req       CommandRequest
	publisher InferenceProgressPublisher

	stdoutBuffer string
	stderrBuffer string

	pendingSSEEvent string
	pendingSSEData  []string
	providerSession *interfaces.ProviderSessionMetadata
	hasher          hash.Hash
}

func (n *codexCommandOutputNormalizer) Observe(stream string, chunk []byte) bool {
	switch stream {
	case workerprocess.OutputStreamStdout:
		n.stdoutBuffer += string(chunk)
		n.drainLines(&n.stdoutBuffer, n.handleStdoutLine)
	case workerprocess.OutputStreamStderr:
		n.stderrBuffer += string(chunk)
		n.drainLines(&n.stderrBuffer, n.handleStderrLine)
	default:
		return false
	}
	return true
}

func (n *codexCommandOutputNormalizer) Flush() {
	if trimmed := strings.TrimSpace(n.stdoutBuffer); trimmed != "" {
		n.handleStdoutLine(trimmed)
	}
	if trimmed := strings.TrimSpace(n.stderrBuffer); trimmed != "" {
		n.handleStderrLine(trimmed)
	}
	n.flushPendingSSEEvent()
}

func (n *codexCommandOutputNormalizer) drainLines(buffer *string, handle func(string)) {
	for {
		index := strings.IndexByte(*buffer, '\n')
		if index < 0 {
			return
		}
		line := strings.TrimRight((*buffer)[:index], "\r")
		*buffer = (*buffer)[index+1:]
		handle(line)
	}
}

func (n *codexCommandOutputNormalizer) handleStdoutLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		n.flushPendingSSEEvent()
		return
	}
	if strings.HasPrefix(trimmed, "event:") {
		n.flushPendingSSEEvent()
		n.pendingSSEEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		return
	}
	if strings.HasPrefix(trimmed, "data:") {
		n.pendingSSEData = append(n.pendingSSEData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		return
	}
	n.flushPendingSSEEvent()
	if n.publishStructuredCodexEvent(trimmed, "") {
		return
	}
	n.publishResponseEvent(NormalizedEventTypeTextDelta, "", trimmed, nil)
}

func (n *codexCommandOutputNormalizer) handleStderrLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	if n.publishStructuredCodexEvent(trimmed, "") {
		return
	}
	n.publishProgressEvent(NormalizedEventTypeProgress, "", trimmed, nil)
}

func (n *codexCommandOutputNormalizer) publishStructuredCodexEvent(raw string, fallbackEventType string) bool {
	normalized, status := normalizeCodexStructuredEvent(raw, fallbackEventType, n.hasher)
	if status == codexStructuredEventStatusNotStructured {
		return false
	}
	if normalized.ProviderSessionRef != nil {
		n.providerSession = interfaces.CloneProviderSessionMetadata(normalized.ProviderSessionRef)
	}
	if normalized.ProviderSessionRef == nil {
		normalized.ProviderSessionRef = interfaces.CloneProviderSessionMetadata(n.providerSession)
	}
	normalized.DispatchID = strings.TrimSpace(n.req.DispatchID)
	normalized.Metadata = mergeStringMaps(baseFragmentMetadata(n.req), normalized.Metadata)
	n.publisher(normalized)
	return true
}

func (n *codexCommandOutputNormalizer) flushPendingSSEEvent() {
	if n.pendingSSEEvent == "" && len(n.pendingSSEData) == 0 {
		return
	}
	raw := strings.Join(n.pendingSSEData, "\n")
	n.publishStructuredCodexEvent(raw, strings.TrimSpace(n.pendingSSEEvent))
	n.pendingSSEEvent = ""
	n.pendingSSEData = nil
}

type codexStructuredEventStatus string

const (
	codexStructuredEventStatusNotStructured codexStructuredEventStatus = "NOT_STRUCTURED"
	codexStructuredEventStatusNormalized    codexStructuredEventStatus = "NORMALIZED"
	codexStructuredEventStatusMalformed     codexStructuredEventStatus = "MALFORMED"
)

func normalizeCodexStructuredEvent(raw string, fallbackEventType string, hasher hash.Hash) (InferenceProgressFragment, codexStructuredEventStatus) {
	trimmedRaw := strings.TrimSpace(raw)
	trimmedFallback := strings.TrimSpace(fallbackEventType)
	if trimmedRaw == "" && trimmedFallback == "" {
		return InferenceProgressFragment{}, codexStructuredEventStatusNotStructured
	}
	if !looksLikeStructuredCodexPayload(trimmedRaw, trimmedFallback) {
		return InferenceProgressFragment{}, codexStructuredEventStatusNotStructured
	}
	var payload map[string]any
	if trimmedRaw == "" {
		return malformedCodexStructuredEvent(trimmedRaw, trimmedFallback, codexDiagnosticIncompleteSSE, hasher), codexStructuredEventStatusMalformed
	}
	if err := json.Unmarshal([]byte(trimmedRaw), &payload); err != nil {
		return malformedCodexStructuredEvent(trimmedRaw, trimmedFallback, codexDiagnosticMalformedJSON, hasher), codexStructuredEventStatusMalformed
	}
	externalEventType := firstNonEmptyString(
		stringValue(payload["event"]),
		stringValue(payload["type"]),
		trimmedFallback,
	)
	normalizedType, kind := classifyCodexStructuredEvent(externalEventType)
	fragment := InferenceProgressFragment{
		Kind:               kind,
		Type:               normalizedType,
		ExternalEventType:  externalEventType,
		ProviderSessionRef: providerSessionFromStructuredCodexEvent(payload),
		Metadata:           codexStructuredEventMetadata(payload),
	}
	switch normalizedType {
	case NormalizedEventTypeStarted:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			externalEventType,
		)
		fragment.Payload = boundedCodexPayload(original, codexRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case NormalizedEventTypeProgress:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			stringValue(payload["status"]),
			externalEventType,
		)
		fragment.Payload = boundedCodexPayload(original, codexRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case NormalizedEventTypeTextDelta, NormalizedEventTypeFinalText:
		original := extractCodexEventText(payload)
		fragment.Payload = boundedCodexPayload(original, codexRetainedTextBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case NormalizedEventTypeFailed, NormalizedEventTypeCanceled:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			stringValue(payload["error"]),
			stringValue(payload["status"]),
			externalEventType,
		)
		fragment.Payload = boundedCodexPayload(original, codexRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	default:
		fragment.Payload = "codex event omitted"
		fragment.Metadata = mergeStringMaps(fragment.Metadata, codexRawDiagnosticMetadata(trimmedRaw, codexDiagnosticUnknownEvent, hasher))
	}
	return fragment, codexStructuredEventStatusNormalized
}

func classifyCodexStructuredEvent(externalEventType string) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(externalEventType))
	switch normalized {
	case "session.created", "response.created", "response.started", "turn.started":
		return NormalizedEventTypeStarted, ProgressFragmentKind
	case "response.output_text.delta", "response.delta", "message.delta", "output_text.delta":
		return NormalizedEventTypeTextDelta, ResponseFragmentKind
	case "response.completed", "response.output_text.done", "message.completed", "output.completed":
		return NormalizedEventTypeFinalText, ResponseFragmentKind
	case "response.failed", "response.error", "error":
		return NormalizedEventTypeFailed, ProgressFragmentKind
	case "response.canceled", "response.cancelled", "session.canceled", "session.cancelled":
		return NormalizedEventTypeCanceled, ProgressFragmentKind
	case "response.progress", "progress", "response.updated":
		return NormalizedEventTypeProgress, ProgressFragmentKind
	}
	switch {
	case strings.Contains(normalized, "cancel"):
		return NormalizedEventTypeCanceled, ProgressFragmentKind
	case strings.Contains(normalized, "fail"), strings.Contains(normalized, "error"):
		return NormalizedEventTypeFailed, ProgressFragmentKind
	case strings.Contains(normalized, "delta"):
		return NormalizedEventTypeTextDelta, ResponseFragmentKind
	case strings.Contains(normalized, "complete"), strings.Contains(normalized, "final"), strings.Contains(normalized, "done"):
		return NormalizedEventTypeFinalText, ResponseFragmentKind
	case strings.Contains(normalized, "start"), strings.Contains(normalized, "created"), strings.Contains(normalized, "begin"):
		return NormalizedEventTypeStarted, ProgressFragmentKind
	case strings.Contains(normalized, "progress"), strings.Contains(normalized, "update"):
		return NormalizedEventTypeProgress, ProgressFragmentKind
	default:
		return NormalizedEventTypeUnknown, ProgressFragmentKind
	}
}

func providerSessionFromStructuredCodexEvent(payload map[string]any) *interfaces.ProviderSessionMetadata {
	candidates := []struct {
		kind  string
		value string
	}{
		{kind: providerSessionKindSessionID, value: firstNonEmptyString(stringValue(payload["session_id"]), stringValue(payload["sessionId"]))},
		{kind: providerSessionKindResponseID, value: firstNonEmptyString(stringValue(payload["response_id"]), stringValue(payload["responseId"]))},
		{kind: providerSessionKindConversationID, value: firstNonEmptyString(stringValue(payload["conversation_id"]), stringValue(payload["conversationId"]))},
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.value) == "" {
			continue
		}
		return &interfaces.ProviderSessionMetadata{
			Provider: interfaces.CanonicalProviderSessionProvider(string(interfaces.ModelProviderCodex)),
			Kind:     candidate.kind,
			ID:       strings.TrimSpace(candidate.value),
		}
	}
	return nil
}

func codexStructuredEventMetadata(payload map[string]any) map[string]string {
	metadata := map[string]string{}
	for _, key := range []string{"session_id", "response_id", "conversation_id"} {
		if value := stringValue(payload[key]); value != "" {
			metadata[key] = value
		}
	}
	if text := extractCodexEventText(payload); text != "" {
		metadata[codexMetadataTextBytesKey] = strconv.Itoa(len([]byte(text)))
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func extractCodexEventText(payload map[string]any) string {
	if delta := stringValue(payload["delta"]); strings.TrimSpace(delta) != "" {
		return delta
	}
	if text := stringValue(payload["text"]); strings.TrimSpace(text) != "" {
		return text
	}
	return strings.TrimSpace(collectCodexText(payload))
}

func collectCodexText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if strings.EqualFold(stringValue(typed["type"]), "output_text") {
			if text := stringValue(typed["text"]); text != "" {
				return text
			}
		}
		parts := make([]string, 0, 4)
		for _, nestedKey := range []string{"response", "output", "content", "item", "message"} {
			if nested, ok := typed[nestedKey]; ok {
				if text := collectCodexText(nested); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := collectCodexText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func boundedCodexPayload(payload string, limit int) string {
	trimmed := strings.TrimSpace(payload)
	if limit <= 0 || len([]byte(trimmed)) <= limit {
		return trimmed
	}
	bytes := []byte(trimmed)
	return strings.TrimSpace(string(bytes[:limit]))
}

func baseFragmentMetadata(req CommandRequest) map[string]string {
	metadata := map[string]string{
		codexMetadataRunnerIDKey: string(interfaces.ModelProviderCodex),
	}
	if workID := primaryWorkID(req.Execution.WorkIDs); workID != "" {
		metadata[codexMetadataWorkIDKey] = workID
	}
	if workstation := strings.TrimSpace(req.WorkstationName); workstation != "" {
		metadata[codexMetadataWorkstationKey] = workstation
	}
	return metadata
}

func mergeStringMaps(base, overlay map[string]string) map[string]string {
	switch {
	case len(base) == 0 && len(overlay) == 0:
		return nil
	case len(base) == 0:
		return cloneStringMap(overlay)
	case len(overlay) == 0:
		return cloneStringMap(base)
	}
	merged := cloneStringMap(base)
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func (n *codexCommandOutputNormalizer) publishResponseEvent(eventType string, externalEventType string, payload string, metadata map[string]string) {
	n.publisher(InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(n.req.DispatchID),
		Kind:               ResponseFragmentKind,
		Type:               eventType,
		Payload:            boundedCodexPayload(payload, codexRetainedTextBytes),
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(n.providerSession),
		ExternalEventType:  strings.TrimSpace(externalEventType),
		Metadata:           mergeStringMaps(baseFragmentMetadata(n.req), metadata),
	})
}

func (n *codexCommandOutputNormalizer) publishProgressEvent(eventType string, externalEventType string, payload string, metadata map[string]string) {
	n.publisher(InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(n.req.DispatchID),
		Kind:               ProgressFragmentKind,
		Type:               eventType,
		Payload:            boundedCodexPayload(payload, codexRetainedProgressBytes),
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(n.providerSession),
		ExternalEventType:  strings.TrimSpace(externalEventType),
		Metadata:           mergeStringMaps(baseFragmentMetadata(n.req), metadata),
	})
}

// NewInferenceProgressPublishingCommandRunner constructs a provider command
// runner that publishes internal response-stream fragments during subprocess IO.
func NewInferenceProgressPublishingCommandRunner(
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) CommandRunner {
	if publisher == nil {
		return workerprocess.ExecCommandRunner{}
	}
	return InferenceProgressPublishingCommandRunner{
		Publisher: publisher,
		Logger:    logger,
	}
}

func isCodexCommand(command string) bool {
	return strings.EqualFold(filepath.Base(strings.TrimSpace(command)), string(interfaces.ModelProviderCodex))
}
