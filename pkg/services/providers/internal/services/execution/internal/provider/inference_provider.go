package provider

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	provideradapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/commandenv"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// DispatcherType selects which CLI tool the ScriptWrapProvider invokes.
type DispatcherType string

const (
	// DispatcherClaude runs the Claude Code CLI ("claude").
	DispatcherClaude DispatcherType = "claude"
	// DispatcherCodex runs the OpenAI Codex CLI ("codex").
	DispatcherCodex DispatcherType = "codex"
)

const (
	providerSessionKindSessionID      = "session_id"
	providerSessionKindConversationID = "conversation_id"
	providerSessionKindResponseID     = "response_id"
)

var providerAutomationEnvDefaults = commandenv.AutomationDefaults()

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

// ResponseStreamExecutor runs a provider-owned structured response lifecycle
// without making the core provider package depend on Factory Session types.
type ResponseStreamExecutor interface {
	Supports(provider string) bool
	Execute(context.Context, workerexecution.ProviderInferenceRequest, bool, work.ContentMaterializer, CommandRunner, InferenceProgressPublisher, logging.Logger) ResponseStreamExecutionResult
}

// ResponseStreamExecutionResult carries the structured execution outcome back
// to the provider boundary for existing diagnostics and failure publication.
type ResponseStreamExecutionResult struct {
	Response                  workerexecution.InferenceResponse
	Request                   CommandRequest
	Command                   CommandResult
	FailureType               workerexecution.WorkFailureType
	FailureMessage            string
	FailureSession            *workerexecution.ProviderSessionMetadata
	CanonicalFailurePublished bool
	Err                       error
}

// ScriptWrapProvider implements Provider by shelling out to a CLI tool
// (Claude Code or Codex) as a subprocess. It supports configurable
// dispatchers and skip-permissions.
type ScriptWrapProvider struct {
	// SkipPermissions enables --dangerously-skip-permissions (claude) or
	// --full-auto (codex).
	SkipPermissions bool
	// ContentMaterializer resolves dispatch-time Work content for provider image arguments.
	ContentMaterializer work.ContentMaterializer
	// Logger is the structured logger for inference diagnostics. Nil disables logging.
	Logger logging.Logger
	exec   CommandRunner

	progressPublisher      InferenceProgressPublisher
	responseStreamExecutor ResponseStreamExecutor
	operatingSystem        string
	clock                  workers.Clock
	temporaryFiles         platformfilesystem.TemporaryFileSystem
}

func (p *ScriptWrapProvider) commandExec() CommandRunner {
	if p.exec != nil {
		return p.exec
	}
	return workers.CommandRunnerWithLogging(nil, p.Logger, nil)
}

// NewScriptWrapProviderWithDependencies constructs a provider from direct
// collaborators. Required external effects must be supplied by composition.
func NewScriptWrapProviderWithDependencies(
	skipPermissions bool,
	logger logging.Logger,
	commandRunner CommandRunner,
	_ interface{},
	progressPublisher InferenceProgressPublisher,
	responseStreamExecutor ResponseStreamExecutor,
	contentMaterializer work.ContentMaterializer,
	clocks ...workers.Clock,
) *ScriptWrapProvider {
	if logger == nil {
		logger = logging.NoopLogger{}
	}
	provider := &ScriptWrapProvider{
		SkipPermissions:        skipPermissions,
		ContentMaterializer:    contentMaterializer,
		Logger:                 logger,
		exec:                   commandRunner,
		progressPublisher:      progressPublisher,
		responseStreamExecutor: responseStreamExecutor,
	}
	if len(clocks) > 0 {
		provider.clock = clocks[0]
	}
	return provider
}

// Infer shells out to the configured CLI dispatcher with the user message.
// It merges req.EnvVars into the subprocess environment.
func (p *ScriptWrapProvider) Infer(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return p.Execute(ctx, req)
}

// Execute implements the shared runner contract while preserving the current
// provider-backed subprocess execution path.
func (p *ScriptWrapProvider) Execute(ctx context.Context, req workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	logger := logging.EnsureLogger(p.Logger)

	logger.Info("inferencer: request starting",
		providerLogFields(req, "model", req.Model)...)
	if isConductorRoutedProvider(req.ModelProvider) {
		response, err := providerRequestValidationFailure(
			req,
			fmt.Errorf("%s executes through the provider-neutral conductor", strings.TrimSpace(req.ModelProvider)),
			logger,
		)
		return response, err
	}
	structuredResponseStream := p.progressPublisher != nil && p.responseStreamExecutor != nil && p.responseStreamExecutor.Supports(req.ModelProvider)
	if structuredResponseStream {
		return p.executeStructuredResponseStream(ctx, req, logger)
	}

	behavior := providerBehaviorFor(req.ModelProvider, logger)
	buildCtx := &ProviderBuildContext{
		ContentCache:        newDispatchContentCache(),
		ContentMaterializer: p.ContentMaterializer,
		operatingSystem:     p.operatingSystem,
		temporaryFiles:      p.temporaryFiles,
	}
	defer buildCtx.release()
	args, err := behavior.BuildArgs(ctx, req, p.SkipPermissions, buildCtx)
	if err != nil {
		return providerRequestValidationFailure(req, err, logger)
	}
	execReq := behavior.BuildCommandRequest(req, args)
	logger.Info("provider invocation prepared", providerPreparedLogFields(ctx, req, execReq)...)
	started := p.providerNow()
	result, err := p.commandExec().Run(ctx, execReq)
	duration := p.providerNow().Sub(started)
	commandDiagnostics := commandDiagnostics(execReq, result, duration, false)
	providerSession := effectiveProviderSession(req, result)
	if err != nil {
		logger.Error("inference dispatch failed with error",
			providerLogFields(req, "error", err.Error())...)
		providerErr := normalizeProviderExecutionError(
			req.ModelProvider, result, err, providerSession, commandDiagnostics,
		)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result, duration)...)
		p.publishFailureFragment(req.Dispatch.DispatchID, providerErr.ProviderSession, providerErr)
		return workerexecution.InferenceResponse{}, providerErr
	}
	if result.ExitCode != 0 {
		logger.Error("inference dispatch failed with non-zero exit code",
			providerLogFields(req,
				"exit_code", result.ExitCode,
				"stdout_bytes", len(result.Stdout),
				"stderr_bytes", len(result.Stderr))...)
		providerErr := normalizeProviderExitFailure(
			req.ModelProvider, result, providerSession, commandDiagnostics,
		)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result, duration)...)
		p.publishFailureFragment(req.Dispatch.DispatchID, providerErr.ProviderSession, providerErr)
		return workerexecution.InferenceResponse{}, providerErr
	}

	content := string(result.Stdout)
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req,
			"output_len", len(content)), providerSession)...)
	p.publishCompletedFragment(req.Dispatch.DispatchID, providerSession)

	return workerexecution.InferenceResponse{
		Content:         content,
		ProviderSession: providerSession,
		Diagnostics:     commandDiagnostics,
	}, nil
}

func isConductorRoutedProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderClaude),
		string(modelprovider.ProviderAntigravity):
		return true
	default:
		return false
	}
}

func (p *ScriptWrapProvider) executeStructuredResponseStream(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
	logger logging.Logger,
) (workerexecution.InferenceResponse, error) {
	started := p.providerNow()
	result := p.responseStreamExecutor.Execute(ctx, req, p.SkipPermissions, p.ContentMaterializer, p.exec, p.progressPublisher, logger)
	duration := p.providerNow().Sub(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, false)
	if result.Err != nil || result.FailureType != "" {
		failureType := result.FailureType
		if failureType == "" {
			failureType = workerexecution.WorkFailureTypeUnknown
		}
		message := strings.TrimSpace(result.FailureMessage)
		if message == "" {
			message = "Provider invocation failed."
		}
		providerErr := newProviderErrorWithDiagnostics(failureType, message, result.Err, result.FailureSession, diagnostics)
		p.publishFailureFragmentWithCanonicalState(req.Dispatch.DispatchID, result.FailureSession, providerErr, result.CanonicalFailurePublished)
		return workerexecution.InferenceResponse{}, providerErr
	}
	result.Response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(result.Response.Content)), result.Response.ProviderSession)...)
	p.publishCompletedFragment(req.Dispatch.DispatchID, result.Response.ProviderSession)
	return result.Response, nil
}

func (p *ScriptWrapProvider) providerNow() time.Time {
	if p == nil || p.clock == nil {
		return time.Time{}
	}
	return p.clock.Now()
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
func buildProviderEnv(processEnvironment []string, envVars map[string]string) []string {
	return commandenv.Build(processEnvironment, envVars)
}

func providerRequestValidationFailure(
	req workerexecution.ProviderInferenceRequest,
	err error,
	logger logging.Logger,
) (workerexecution.InferenceResponse, error) {
	logger.Error("inferencer: request argument validation failed",
		providerLogFields(req, "error", err.Error())...)
	return workerexecution.InferenceResponse{}, newProviderErrorWithDiagnostics(
		workerexecution.WorkFailureTypePermanentBadRequest,
		err.Error(),
		err,
		nil,
		workDiagnosticsForInferenceRequest(req),
	)
}

// TODO: right now the stderr/stdout for the print prints out the entire response log for the stdout....
// We don't necessarily want that....
//
//Failed     process              14:03:20   14:06:29   3m8s     prd-endpoint-state-panels prd-endpoint-state-panels provider error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)
//--------

func normalizeProviderExecutionError(provider string, result CommandResult, err error, session *workerexecution.ProviderSessionMetadata, diagnostics *workerexecution.WorkDiagnostics) *ProviderError {
	switch {
	case isProviderExecutionTimeout(err, result):
		if diagnostics != nil && diagnostics.Command != nil {
			diagnostics.Command.TimedOut = true
		}
		return newProviderErrorFromResultWithDiagnostics(
			parseProviderTimeoutFailure(provider, result),
			err,
			session,
			diagnostics,
		)
	case errors.Is(err, exec.ErrNotFound):
		message := formatProviderCommandFailure(provider, result, err)
		return newProviderErrorWithDiagnostics(workerexecution.WorkFailureTypeMissingExecutable, message, err, session, diagnostics)
	default:
		if result.ExitCode != 0 {
			return normalizeProviderExitFailure(provider, result, session, diagnostics)
		}
		message := formatProviderCommandFailure(provider, result, err)
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return newProviderErrorWithDiagnostics(workerexecution.WorkFailureTypeMisconfigured, message, err, session, diagnostics)
		}
		return newProviderErrorWithDiagnostics(workerexecution.WorkFailureTypeUnknown, message, err, session, diagnostics)
	}
}

func normalizeProviderExitFailure(provider string, result CommandResult, session *workerexecution.ProviderSessionMetadata, diagnostics *workerexecution.WorkDiagnostics) *ProviderError {
	parsed := parseProviderExitFailure(provider, result)
	if parsed.providerSession != nil {
		session = parsed.providerSession
	}
	return newProviderErrorFromResultWithDiagnostics(parsed.failure, ProviderFailureInternalCauseError(parsed.internalCause), session, diagnostics)
}

type parsedProviderFailure struct {
	failure         ProviderFailureResult
	internalCause   string
	providerSession *workerexecution.ProviderSessionMetadata
}

func providerErrorFromAdapterFailure(
	failure *provideradapter.FailureFacts,
	cause error,
	diagnostics *workerexecution.WorkDiagnostics,
) *ProviderError {
	return &ProviderError{
		Family: failure.Family, Type: failure.Type, Message: failure.Message,
		ProviderSession: workerexecution.CloneProviderSessionMetadata(failure.ProviderSession),
		Diagnostics:     workerexecution.CloneWorkDiagnostics(diagnostics), Cause: cause,
	}
}

func parseProviderExitFailure(provider string, result CommandResult) parsedProviderFailure {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	return parsedProviderFailure{failure: parseUnknownProviderFailure(normalizedProvider, result)}
}

func parseUnknownProviderFailure(provider string, result CommandResult) ProviderFailureResult {
	normalizedOutput := strings.ToLower(formatCombinedProviderOutput(result))
	reason := workerexecution.WorkFailureTypeUnknown
	switch {
	case containsAny(normalizedOutput, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		reason = workerexecution.WorkFailureTypeAuthFailure
	case containsAny(normalizedOutput, "invalid argument", "bad request", "invalid request"):
		reason = workerexecution.WorkFailureTypePermanentBadRequest
	case containsAny(normalizedOutput, "rate limit", "too many requests", "resource exhausted", "429"):
		reason = workerexecution.WorkFailureTypeThrottled
	case containsAny(normalizedOutput, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		reason = workerexecution.WorkFailureTypeInternalServerError
	case result.ExitCode == 124 || containsAny(normalizedOutput, "deadline exceeded", "timed out", "timeout"):
		reason = workerexecution.WorkFailureTypeTimeout
	}
	return ProviderFailureResult{
		Reason:  reason,
		Message: formatProviderOutputOrDefault(result, fmt.Sprintf("%s exited with code %d", provider, result.ExitCode)),
	}
}

// NormalizeProviderExitFailure exposes the canonical provider exit-failure
// normalization path for compatibility shims and behavior-focused tests.
func NormalizeProviderExitFailure(provider string, result CommandResult, session *workerexecution.ProviderSessionMetadata, diagnostics *workerexecution.WorkDiagnostics) *ProviderError {
	return normalizeProviderExitFailure(provider, result, session, diagnostics)
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

func parseProviderTimeoutFailure(_ string, result CommandResult) ProviderFailureResult {
	message := formatProviderOutputOrDefault(result, "execution timeout")
	return ProviderFailureResult{
		Reason:  workerexecution.WorkFailureTypeTimeout,
		Message: message,
	}
}

func (p *ScriptWrapProvider) publishCompletedFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	p.progressPublisher(CompletedFragment(dispatchID, providerSession))
}

func (p *ScriptWrapProvider) publishFailureFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, err error) {
	p.publishFailureFragmentWithCanonicalState(dispatchID, providerSession, err, false)
}

func (p *ScriptWrapProvider) publishFailureFragmentWithCanonicalState(
	dispatchID string,
	providerSession *workerexecution.ProviderSessionMetadata,
	err error,
	canonicalPublished bool,
) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	message := ""
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message = strings.TrimSpace(providerErr.Message)
	}
	if message == "" && err != nil {
		message = strings.TrimSpace(err.Error())
	}
	fragment := FailedFragment(dispatchID, providerSession, message)
	fragment.CanonicalEventAlreadyPublished = canonicalPublished
	p.progressPublisher(fragment)
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

func providerSessionFromCommandResult(provider string, result CommandResult) *workerexecution.ProviderSessionMetadata {
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
		return &workerexecution.ProviderSessionMetadata{
			Provider: workerexecution.CanonicalProviderSessionProvider(provider),
			Kind:     candidate.kind,
			ID:       identifier,
		}
	}

	return nil
}

func effectiveProviderSession(req workerexecution.ProviderInferenceRequest, result CommandResult) *workerexecution.ProviderSessionMetadata {
	session := providerSessionFromCommandResult(req.ModelProvider, result)
	if session != nil {
		return session
	}
	if (req.ModelProvider == string(modelprovider.ProviderClaude) || req.ModelProvider == string(modelprovider.ProviderAntigravity)) && req.SessionID != "" {
		return &workerexecution.ProviderSessionMetadata{
			Provider: workerexecution.CanonicalProviderSessionProvider(req.ModelProvider),
			Kind:     providerSessionKindSessionID,
			ID:       req.SessionID,
		}
	}
	return nil
}

var _ providercontract.Provider = (*ScriptWrapProvider)(nil)
