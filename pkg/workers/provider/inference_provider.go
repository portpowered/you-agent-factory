package provider

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/interfaces/responseevents"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	provideradapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
	agyadapter "github.com/portpowered/infinite-you/pkg/workers/provider/agy"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	"github.com/portpowered/infinite-you/pkg/workers/provider/commandenv"
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

// WithOpenCodeCapabilityResolver injects the process-lifetime OpenCode
// negotiation cache. It is primarily useful for production-boundary tests.
func WithOpenCodeCapabilityResolver(resolver *opencodeadapter.Resolver) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.openCodeResolver = resolver
	}
}

// WithInferenceProgressPublisher injects the internal session-stream publisher
// used for additive provider progress and terminal stream markers.
func WithInferenceProgressPublisher(publisher InferenceProgressPublisher) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.progressPublisher = publisher
	}
}

// ResponseStreamExecutor runs a provider-owned structured response lifecycle
// without making the core provider package depend on Factory Session types.
type ResponseStreamExecutor interface {
	Supports(provider string) bool
	Execute(context.Context, interfaces.ProviderInferenceRequest, bool, *materialize.Options, CommandRunner, InferenceProgressPublisher, logging.Logger) ResponseStreamExecutionResult
}

// ResponseStreamExecutionResult carries the structured execution outcome back
// to the provider boundary for existing diagnostics and failure publication.
type ResponseStreamExecutionResult struct {
	Response                  interfaces.InferenceResponse
	Request                   CommandRequest
	Command                   CommandResult
	FailureType               interfaces.WorkFailureType
	FailureMessage            string
	FailureSession            *interfaces.ProviderSessionMetadata
	CanonicalFailurePublished bool
	Err                       error
}

// WithResponseStreamExecutor injects structured provider mode selection.
func WithResponseStreamExecutor(executor ResponseStreamExecutor) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.responseStreamExecutor = executor
	}
}

// WithAgyFactoryRoot sets the factory root used for Agy workspace normalization.
func WithAgyFactoryRoot(factoryRoot string) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.agyFactoryRoot = factoryRoot
	}
}

// WithAgyPTYAllocator injects a mock or platform PTY allocator for Agy execution tests.
func WithAgyPTYAllocator(allocator agypty.PTYAllocator) ScriptWrapProviderOption {
	return func(p *ScriptWrapProvider) {
		p.agyAllocator = allocator
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

	progressPublisher      InferenceProgressPublisher
	responseStreamExecutor ResponseStreamExecutor
	openCodeResolver       *opencodeadapter.Resolver
	openCodeResolverErr    error
	agyFactoryRoot         string
	agyAllocator           agypty.PTYAllocator
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
	if p.openCodeResolver == nil {
		p.openCodeResolver, p.openCodeResolverErr = opencodeadapter.NewDefaultResolver(p.exec)
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
		providerLogFields(req, "model", req.Model)...)
	if strings.EqualFold(strings.TrimSpace(req.ModelProvider), string(interfaces.ModelProviderOpenCode)) {
		if err := validateOpenCodeOptionalCapabilities(req); err != nil {
			return interfaces.InferenceResponse{}, p.openCodeRequestValidationError(req, err)
		}
		return p.executeNegotiatedOpenCode(ctx, req, logger)
	}
	if strings.EqualFold(strings.TrimSpace(req.ModelProvider), string(interfaces.ModelProviderAgy)) {
		return p.executeAgy(ctx, req, logger)
	}
	structuredResponseStream := p.progressPublisher != nil && p.responseStreamExecutor != nil && p.responseStreamExecutor.Supports(req.ModelProvider)
	if structuredResponseStream && strings.EqualFold(strings.TrimSpace(req.ModelProvider), string(interfaces.ModelProviderCodex)) {
		structuredResponseStream = p.codexResponseStreamCapable()
	}
	if structuredResponseStream {
		if strings.EqualFold(strings.TrimSpace(req.ModelProvider), string(interfaces.ModelProviderClaude)) {
			if err := unsupportedImageContentError(req.InputTokens, "model provider claude"); err != nil {
				return providerRequestValidationFailure(req, err, logger)
			}
		}
		return p.executeStructuredResponseStream(ctx, req, logger)
	}

	behavior := providerBehaviorFor(req.ModelProvider, logger)
	buildCtx := &ProviderBuildContext{
		ContentCache:    materialize.NewDispatchCache(),
		MaterializeOpts: p.MaterializeOptions,
	}
	defer buildCtx.release()
	args, err := behavior.BuildArgs(ctx, req, p.SkipPermissions, buildCtx)
	if err != nil {
		return providerRequestValidationFailure(req, err, logger)
	}
	execReq := behavior.BuildCommandRequest(req, args)
	logger.Info("provider invocation prepared", providerPreparedLogFields(ctx, req, execReq)...)
	started := time.Now()
	result, err := p.commandExec().Run(ctx, execReq)
	duration := time.Since(started)
	commandDiagnostics := commandDiagnostics(execReq, result, duration, false)
	providerSession := effectiveProviderSession(req, result)
	cursorProvider := req.ModelProvider == string(interfaces.ModelProviderCursor)
	if err != nil {
		logger.Error("inference dispatch failed with error",
			providerLogFields(req, "error", err.Error())...)
		providerErr := normalizeProviderExecutionError(
			req.ModelProvider, result, err, providerSession,
			cursorInferenceFailureDiagnostics(cursorProvider, commandDiagnostics, result),
		)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result, duration)...)
		logger.Error("inferencer: request failed",
			cursorFailureLogFields(req, cursorProvider, result, "has_error", true)...)
		p.publishFailureFragment(req.Dispatch.DispatchID, providerErr.ProviderSession, providerErr)
		return interfaces.InferenceResponse{}, providerErr
	}
	if result.ExitCode != 0 {
		logger.Error("inference dispatch failed with non-zero exit code",
			providerLogFields(req,
				"exit_code", result.ExitCode,
				"stdout_bytes", len(result.Stdout),
				"stderr_bytes", len(result.Stderr))...)
		providerErr := normalizeProviderExitFailure(
			req.ModelProvider, result, providerSession,
			cursorInferenceFailureDiagnostics(cursorProvider, commandDiagnostics, result),
		)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result, duration)...)
		logger.Error("inferencer: request failed",
			cursorFailureLogFields(req, cursorProvider, result, "exit_code", result.ExitCode)...)
		p.publishFailureFragment(req.Dispatch.DispatchID, providerErr.ProviderSession, providerErr)
		return interfaces.InferenceResponse{}, providerErr
	}

	if cursorProvider {
		return p.completeCursorInference(req, result, commandDiagnostics, logger)
	}
	content := string(result.Stdout)
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req,
			"output_len", len(content)), providerSession)...)
	p.publishCompletedFragment(req.Dispatch.DispatchID, providerSession)

	return interfaces.InferenceResponse{
		Content:         content,
		ProviderSession: providerSession,
		Diagnostics:     commandDiagnostics,
	}, nil
}

func (p *ScriptWrapProvider) executeNegotiatedOpenCode(
	ctx context.Context,
	req interfaces.ProviderInferenceRequest,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	if p.openCodeResolverErr != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, p.openCodeResolverErr)
	}
	if p.openCodeResolver == nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, errors.New("OpenCode capability resolver is unavailable"))
	}
	decision, err := p.openCodeResolver.Resolve(ctx, string(interfaces.ModelProviderOpenCode))
	if err != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, err)
	}
	p.publishOpenCodeCapability(req.Dispatch.DispatchID, decision.Capabilities(), nil)
	if err := validateOpenCodeNegotiatedCapabilities(req, decision); err != nil {
		return interfaces.InferenceResponse{}, p.openCodeRequestValidationError(req, err)
	}
	negotiated, err := opencodeadapter.NewNegotiatedAdapterForRequest(decision, p.openCodeResolver, req)
	if err != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, err)
	}
	registry, err := provideradapter.NewRegistry(negotiated)
	if err != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, err)
	}
	started := time.Now()
	result, executeErr := provideradapter.Execute(ctx, registry, p.openCodeAdapterRunner(), provideradapter.ExecuteInput{
		Provider: negotiated.Identity(),
		Command:  provideradapter.CommandContext{Request: req, SkipPermissions: p.SkipPermissions},
		Decoder:  provideradapter.DecoderContext{RunID: req.Dispatch.DispatchID, DispatchID: req.Dispatch.DispatchID},
		Publish:  p.publishOpenCodeDecoded(req.Dispatch.DispatchID),
	})
	duration := time.Since(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, result.Outcome == provideradapter.CommandOutcomeCanceled)
	for index := range result.CapabilityUpdates {
		update := result.CapabilityUpdates[index]
		p.publishOpenCodeCapability(req.Dispatch.DispatchID, update.Capabilities, &update.Diagnostic)
	}
	if result.Failure != nil {
		providerErr := providerErrorFromAdapterFailure(result.Failure, executeErr, diagnostics)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
		p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}
	if executeErr != nil {
		providerErr := normalizeProviderExecutionError(req.ModelProvider, result.Command, executeErr, result.Response.ProviderSession, diagnostics)
		p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}

	response := result.Response
	response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(response.Content)), response.ProviderSession)...)
	p.publishOpenCodeCompleted(req.Dispatch.DispatchID, response.ProviderSession, len(result.Drafts) > 0)
	return response, nil
}

func (p *ScriptWrapProvider) executeAgy(
	ctx context.Context,
	req interfaces.ProviderInferenceRequest,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	factoryRoot := strings.TrimSpace(p.agyFactoryRoot)
	if factoryRoot == "" {
		return interfaces.InferenceResponse{}, p.agyRequestValidationError(req, errors.New("Agy factory root is unavailable"))
	}
	opts := []agyadapter.Option{}
	if p.agyAllocator != nil {
		opts = append(opts, agyadapter.WithAllocator(p.agyAllocator))
	}
	providerAdapter := agyadapter.NewAdapter(factoryRoot, opts...)
	registry, err := provideradapter.NewRegistry(providerAdapter)
	if err != nil {
		return interfaces.InferenceResponse{}, p.agyRequestValidationError(req, err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		return interfaces.InferenceResponse{}, p.agyRequestValidationError(req, err)
	}
	started := time.Now()
	result, executeErr := provideradapter.Execute(ctx, registry, runner, provideradapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command:  provideradapter.CommandContext{Request: req, SkipPermissions: p.SkipPermissions, MaterializeOptions: p.MaterializeOptions},
		Decoder:  provideradapter.DecoderContext{RunID: req.Dispatch.DispatchID, DispatchID: req.Dispatch.DispatchID},
		ObserveDraft: func(draft responseevents.Draft) {
			if p.progressPublisher != nil {
				p.progressPublisher(CanonicalDraftFragment(draft.DispatchID, draft))
			}
		},
	})
	duration := time.Since(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, result.Outcome == provideradapter.CommandOutcomeCanceled)
	if result.Failure != nil {
		providerErr := providerErrorFromAdapterFailure(result.Failure, executeErr, diagnostics)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
		p.publishAgyFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}
	if executeErr != nil {
		providerErr := normalizeProviderExecutionError(req.ModelProvider, result.Command, executeErr, result.Response.ProviderSession, diagnostics)
		p.publishAgyFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}
	response := result.Response
	response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(response.Content)), response.ProviderSession)...)
	p.publishAgyCompleted(req.Dispatch.DispatchID, response.ProviderSession, len(result.Drafts) > 0)
	return response, nil
}

func (p *ScriptWrapProvider) agyRequestValidationError(req interfaces.ProviderInferenceRequest, err error) *ProviderError {
	providerErr := newProviderErrorWithDiagnostics(
		interfaces.WorkFailureTypePermanentBadRequest,
		err.Error(),
		err,
		nil,
		workDiagnosticsForInferenceRequest(req),
	)
	p.publishFailureFragment(req.Dispatch.DispatchID, nil, providerErr)
	return providerErr
}

func (p *ScriptWrapProvider) publishAgyCompleted(
	dispatchID string,
	providerSession *interfaces.ProviderSessionMetadata,
	skipCanonical bool,
) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	fragment := CompletedFragment(dispatchID, providerSession)
	fragment.CanonicalEventAlreadyPublished = skipCanonical
	p.progressPublisher(fragment)
}

func (p *ScriptWrapProvider) publishAgyFailure(dispatchID string, err error, skipCanonical bool) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	message := ""
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message = providerErr.Message
	}
	var providerSession *interfaces.ProviderSessionMetadata
	if providerErr != nil {
		providerSession = providerErr.ProviderSession
	}
	fragment := FailedFragment(dispatchID, providerSession, message)
	fragment.CanonicalEventAlreadyPublished = skipCanonical
	p.progressPublisher(fragment)
}

func validateOpenCodeNegotiatedCapabilities(req interfaces.ProviderInferenceRequest, decision opencodeadapter.Decision) error {
	if decision.Mode == opencodeadapter.ModeStructured {
		return nil
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if capability == interfaces.RunnerOptionalCapabilityStructuredOutput {
			return errors.New("structured output is required but is not supported by the installed opencode runner")
		}
	}
	return nil
}

func (p *ScriptWrapProvider) openCodeRequestValidationError(req interfaces.ProviderInferenceRequest, err error) *ProviderError {
	providerErr := newProviderErrorWithDiagnostics(
		interfaces.WorkFailureTypePermanentBadRequest,
		err.Error(),
		err,
		nil,
		workDiagnosticsForInferenceRequest(req),
	)
	p.publishFailureFragment(req.Dispatch.DispatchID, nil, providerErr)
	return providerErr
}

func (p *ScriptWrapProvider) openCodeResolutionError(req interfaces.ProviderInferenceRequest, err error) *ProviderError {
	providerErr := normalizeProviderExecutionError(req.ModelProvider, CommandResult{}, err, nil, workDiagnosticsForInferenceRequest(req))
	p.publishFailureFragment(req.Dispatch.DispatchID, nil, providerErr)
	return providerErr
}

func providerErrorFromAdapterFailure(
	failure *provideradapter.FailureFacts,
	cause error,
	diagnostics *interfaces.WorkDiagnostics,
) *ProviderError {
	return &ProviderError{
		Family: failure.Family, Type: failure.Type, Message: failure.Message,
		ProviderSession: interfaces.CloneProviderSessionMetadata(failure.ProviderSession),
		Diagnostics:     interfaces.CloneWorkDiagnostics(diagnostics), Cause: cause,
	}
}

type bufferedAdapterRunner struct{ runner CommandRunner }

func (r bufferedAdapterRunner) Run(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(provideradapter.Observation) error,
) (workerprocess.CommandResult, error) {
	result, err := r.runner.Run(ctx, req)
	if len(result.Stdout) > 0 {
		if observeErr := observe(provideradapter.Observation{Stream: provideradapter.OutputStreamStdout, Chunk: result.Stdout}); observeErr != nil {
			return result, observeErr
		}
	}
	if len(result.Stderr) > 0 {
		if observeErr := observe(provideradapter.Observation{Stream: provideradapter.OutputStreamStderr, Chunk: result.Stderr}); observeErr != nil {
			return result, observeErr
		}
	}
	return result, err
}

type execAdapterRunner struct{ logger logging.Logger }

func (r execAdapterRunner) Run(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(provideradapter.Observation) error,
) (workerprocess.CommandResult, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var observeMu sync.Mutex
	var observeErr error
	result, err := (workerprocess.StreamingExecCommandRunner{
		Logger: r.logger,
		Observer: func(stream string, chunk []byte) {
			observeMu.Lock()
			defer observeMu.Unlock()
			if observeErr != nil {
				return
			}
			observeErr = observe(provideradapter.Observation{Stream: provideradapter.OutputStream(stream), Chunk: chunk})
			if observeErr != nil {
				cancel()
			}
		},
	}).Run(runCtx, req)
	return result, errors.Join(err, observeErr)
}

func (p *ScriptWrapProvider) openCodeAdapterRunner() provideradapter.StreamingCommandRunner {
	switch runner := p.exec.(type) {
	case workerprocess.ExecCommandRunner, *workerprocess.ExecCommandRunner:
		return execAdapterRunner{logger: logging.EnsureLogger(p.Logger)}
	case InferenceProgressPublishingCommandRunner:
		return execAdapterRunner{logger: logging.EnsureLogger(runner.Logger)}
	case *InferenceProgressPublishingCommandRunner:
		if runner != nil {
			return execAdapterRunner{logger: logging.EnsureLogger(runner.Logger)}
		}
	default:
	}
	return bufferedAdapterRunner{runner: p.commandExec()}
}

func (p *ScriptWrapProvider) publishOpenCodeDecoded(dispatchID string) func(provideradapter.DecodeResult) {
	return func(decoded provideradapter.DecodeResult) {
		if p == nil || p.progressPublisher == nil {
			return
		}
		for _, draft := range decoded.Drafts {
			p.progressPublisher(CanonicalDraftFragment(draft.DispatchID, draft))
		}
		for _, diagnostic := range decoded.Diagnostics {
			p.progressPublisher(InferenceProgressFragment{
				DispatchID: dispatchID, Kind: ProgressFragmentKind, Type: NormalizedEventTypeProgress,
				Payload: diagnostic.Message, ExternalEventType: diagnostic.Code,
				Metadata: map[string]string{"runner_id": string(interfaces.ModelProviderOpenCode), "diagnostic_code": diagnostic.Code},
			})
		}
	}
}

func (p *ScriptWrapProvider) publishOpenCodeCapability(
	dispatchID string,
	capabilities provideradapter.Capabilities,
	diagnostic *provideradapter.Diagnostic,
) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	mode, fidelity, message, eventType := "structured", "normalized", "OpenCode selected structured output mode.", "provider_capability_selected"
	if capabilities.FinalOnly {
		mode, fidelity, message = "final_only", "final_only", "OpenCode selected final-only output mode."
	}
	metadata := map[string]string{
		"runner_id": string(interfaces.ModelProviderOpenCode), "selected_mode": mode, "fidelity": fidelity,
	}
	if diagnostic != nil {
		message, eventType = diagnostic.Message, diagnostic.Code
		metadata["downgrade_reason"] = "unsupported_format"
	}
	p.progressPublisher(InferenceProgressFragment{
		DispatchID: dispatchID, Kind: ProgressFragmentKind, Type: NormalizedEventTypeProgress,
		Payload: message, ExternalEventType: eventType, Metadata: metadata,
	})
}

func (p *ScriptWrapProvider) publishOpenCodeCompleted(
	dispatchID string,
	providerSession *interfaces.ProviderSessionMetadata,
	skipCanonical bool,
) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	fragment := CompletedFragment(dispatchID, providerSession)
	fragment.CanonicalEventAlreadyPublished = skipCanonical
	p.progressPublisher(fragment)
}

func (p *ScriptWrapProvider) publishOpenCodeFailure(dispatchID string, err error, skipCanonical bool) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	message := ""
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message = providerErr.Message
	}
	var providerSession *interfaces.ProviderSessionMetadata
	if providerErr != nil {
		providerSession = providerErr.ProviderSession
	}
	fragment := FailedFragment(dispatchID, providerSession, message)
	fragment.CanonicalEventAlreadyPublished = skipCanonical
	p.progressPublisher(fragment)
}

func (p *ScriptWrapProvider) codexResponseStreamCapable() bool {
	if p == nil || p.exec == nil {
		return false
	}
	capable, ok := p.exec.(interface{ SupportsResponseStreaming() bool })
	return ok && capable.SupportsResponseStreaming()
}

func (p *ScriptWrapProvider) executeStructuredResponseStream(
	ctx context.Context,
	req interfaces.ProviderInferenceRequest,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	started := time.Now()
	result := p.responseStreamExecutor.Execute(ctx, req, p.SkipPermissions, p.MaterializeOptions, p.exec, p.progressPublisher, logger)
	duration := time.Since(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, false)
	if result.Err != nil || result.FailureType != "" {
		failureType := result.FailureType
		if failureType == "" {
			failureType = interfaces.WorkFailureTypeUnknown
		}
		message := strings.TrimSpace(result.FailureMessage)
		if message == "" {
			message = "Provider invocation failed."
		}
		providerErr := newProviderErrorWithDiagnostics(failureType, message, result.Err, result.FailureSession, diagnostics)
		p.publishFailureFragmentWithCanonicalState(req.Dispatch.DispatchID, result.FailureSession, providerErr, result.CanonicalFailurePublished)
		return interfaces.InferenceResponse{}, providerErr
	}
	result.Response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(result.Response.Content)), result.Response.ProviderSession)...)
	p.publishCompletedFragment(req.Dispatch.DispatchID, result.Response.ProviderSession)
	return result.Response, nil
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
	return commandenv.Build(envVars)
}

func providerRequestValidationFailure(
	req interfaces.ProviderInferenceRequest,
	err error,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	logger.Error("inferencer: request argument validation failed",
		providerLogFields(req, "error", err.Error())...)
	return interfaces.InferenceResponse{}, newProviderErrorWithDiagnostics(
		interfaces.WorkFailureTypePermanentBadRequest,
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

func normalizeProviderExecutionError(provider string, result CommandResult, err error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	if normalizedProvider == string(interfaces.ModelProviderCursor) {
		fallbackReason := interfaces.WorkFailureTypeUnknown
		switch {
		case isProviderExecutionTimeout(err, result):
			fallbackReason = interfaces.WorkFailureTypeTimeout
			if diagnostics != nil && diagnostics.Command != nil {
				diagnostics.Command.TimedOut = true
			}
		case errors.Is(err, exec.ErrNotFound):
			fallbackReason = interfaces.WorkFailureTypeMisconfigured
		default:
			var execErr *exec.Error
			if errors.As(err, &execErr) {
				fallbackReason = interfaces.WorkFailureTypeMisconfigured
			}
		}
		return cursorProviderError(result, fallbackReason, "", err, session, diagnostics)
	}
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
		return newProviderErrorWithDiagnostics(interfaces.WorkFailureTypeMissingExecutable, message, err, session, diagnostics)
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
	parsed := parseProviderExitFailure(provider, result)
	if parsed.providerSession != nil {
		session = parsed.providerSession
	}
	return newProviderErrorFromResultWithDiagnostics(parsed.failure, nil, session, diagnostics)
}

type parsedProviderFailure struct {
	failure         ProviderFailureResult
	providerSession *interfaces.ProviderSessionMetadata
}

func parseProviderExitFailure(provider string, result CommandResult) parsedProviderFailure {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	switch normalizedProvider {
	case string(interfaces.ModelProviderClaude):
		return parsedProviderFailure{failure: ParseClaudeProviderFailure(result)}
	case string(interfaces.ModelProviderCodex):
		return parsedProviderFailure{failure: ParseCodexProviderFailure(result)}
	case string(interfaces.ModelProviderKiro):
		return parsedProviderFailure{failure: ParseKiroProviderFailure(result)}
	case string(interfaces.ModelProviderOpenCode):
		return parsedProviderFailure{failure: ParseOpenCodeProviderFailure(result)}
	case string(interfaces.ModelProviderGemini):
		return parsedProviderFailure{failure: ParseGeminiProviderFailure(result)}
	case string(interfaces.ModelProviderCursor):
		failure := cursorpkg.ParseProviderFailure(cursorpkg.FailureInput{
			Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
		})
		return parsedProviderFailure{
			failure:         ProviderFailureResult{Reason: failure.Reason, Message: failure.Message},
			providerSession: failure.ProviderSession,
		}
	default:
		return parsedProviderFailure{failure: parseUnknownProviderFailure(normalizedProvider, result)}
	}
}

func parseUnknownProviderFailure(provider string, result CommandResult) ProviderFailureResult {
	normalizedOutput := strings.ToLower(formatCombinedProviderOutput(result))
	reason := interfaces.WorkFailureTypeUnknown
	switch {
	case containsAny(normalizedOutput, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		reason = interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalizedOutput, "invalid argument", "bad request", "invalid request"):
		reason = interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalizedOutput, "rate limit", "too many requests", "resource exhausted", "429"):
		reason = interfaces.WorkFailureTypeThrottled
	case containsAny(normalizedOutput, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		reason = interfaces.WorkFailureTypeInternalServerError
	case result.ExitCode == 124 || containsAny(normalizedOutput, "deadline exceeded", "timed out", "timeout"):
		reason = interfaces.WorkFailureTypeTimeout
	}
	return ProviderFailureResult{
		Reason:  reason,
		Message: formatProviderOutputOrDefault(result, fmt.Sprintf("%s exited with code %d", provider, result.ExitCode)),
	}
}

func cursorProviderError(
	result CommandResult,
	fallbackReason interfaces.WorkFailureType,
	fallbackMessage string,
	cause error,
	session *interfaces.ProviderSessionMetadata,
	diagnostics *interfaces.WorkDiagnostics,
) *ProviderError {
	failure := cursorpkg.ParseProviderFailure(cursorpkg.FailureInput{
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		FallbackReason:  fallbackReason,
		FallbackMessage: fallbackMessage,
	})
	if failure.ProviderSession != nil {
		session = failure.ProviderSession
	}
	return newProviderErrorFromResultWithDiagnostics(
		ProviderFailureResult{Reason: failure.Reason, Message: failure.Message},
		cause,
		session,
		diagnostics,
	)
}

// NormalizeProviderExitFailure exposes the canonical provider exit-failure
// normalization path for compatibility shims and behavior-focused tests.
func NormalizeProviderExitFailure(provider string, result CommandResult, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
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

func parseProviderTimeoutFailure(provider string, result CommandResult) ProviderFailureResult {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	message := formatProviderOutputOrDefault(result, "execution timeout")
	switch normalizedProvider {
	case string(interfaces.ModelProviderCodex):
		if codexError, ok := extractCodexErrorLine(result); ok {
			message = codexError
		}
	case string(interfaces.ModelProviderGemini):
		message = geminiTimeoutFailureMessage
	case string(interfaces.ModelProviderKiro):
		message = kiroTimeoutFailureMessage
	}
	return ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeTimeout,
		Message: message,
	}
}

func cursorFailureLogFields(req interfaces.RunnerExecutionRequest, cursorProvider bool, result CommandResult, extra ...any) []any {
	return providerLogFields(req, extra...)
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
		providerSession := parseErr.ProviderSession
		if providerSession == nil {
			providerSession = effectiveProviderSession(req, result)
		}
		providerErr := cursorParseProviderError(result, parseErr, providerSession, failureDiagnostics)
		p.publishFailureFragment(req.Dispatch.DispatchID, providerErr.ProviderSession, providerErr)
		return interfaces.InferenceResponse{}, providerErr
	}
	diagnostics := cursorpkg.WithResponseMetadata(commandDiagnostics, parsed.ResponseMetadata)
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req,
			"output_len", len(parsed.Content)), parsed.ProviderSession)...)
	p.publishCompletedFragment(req.Dispatch.DispatchID, parsed.ProviderSession)
	return interfaces.InferenceResponse{
		Content:         parsed.Content,
		ProviderSession: parsed.ProviderSession,
		Diagnostics:     diagnostics,
	}, nil
}

func cursorParseProviderError(
	result CommandResult,
	parseErr *cursorpkg.ParseFailure,
	session *interfaces.ProviderSessionMetadata,
	diagnostics *interfaces.WorkDiagnostics,
) *ProviderError {
	failure, canonical := parseErr.CanonicalResult()
	if !canonical {
		return cursorProviderError(
			result, parseErr.Type, parseErr.Message, parseErr.Cause, session, diagnostics,
		)
	}
	if failure.ProviderSession != nil {
		session = failure.ProviderSession
	}
	return newProviderErrorFromResultWithDiagnostics(
		ProviderFailureResult{Reason: failure.Reason, Message: failure.Message},
		parseErr.Cause,
		session,
		diagnostics,
	)
}

func (p *ScriptWrapProvider) publishCompletedFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	p.progressPublisher(CompletedFragment(dispatchID, providerSession))
}

func (p *ScriptWrapProvider) publishFailureFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, err error) {
	p.publishFailureFragmentWithCanonicalState(dispatchID, providerSession, err, false)
}

func (p *ScriptWrapProvider) publishFailureFragmentWithCanonicalState(
	dispatchID string,
	providerSession *interfaces.ProviderSessionMetadata,
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
	if (req.ModelProvider == string(interfaces.ModelProviderClaude) || req.ModelProvider == string(interfaces.ModelProviderCursor) || req.ModelProvider == string(interfaces.ModelProviderOpenCode) || req.ModelProvider == string(interfaces.ModelProviderPi)) && req.SessionID != "" {
		return &interfaces.ProviderSessionMetadata{
			Provider: interfaces.CanonicalProviderSessionProvider(req.ModelProvider),
			Kind:     providerSessionKindSessionID,
			ID:       req.SessionID,
		}
	}
	return nil
}

var _ Provider = (*ScriptWrapProvider)(nil)
