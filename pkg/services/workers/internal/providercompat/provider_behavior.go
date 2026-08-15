package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"sync"
)

// ProviderBuildContext carries dispatch-scoped resources for provider argument building.
type ProviderBuildContext struct {
	ContentCache        *dispatchContentCache
	ContentMaterializer work.ContentMaterializer
	operatingSystem     string
	tempDir             string
	temporaryFiles      platformfilesystem.TemporaryFileSystem
}

type dispatchContentCache struct {
	mu      sync.Mutex
	entries map[string]dispatchContentEntry
}

type dispatchContentEntry struct {
	localPath string
	cleanup   work.ContentCleanup
}

func newDispatchContentCache() *dispatchContentCache {
	return &dispatchContentCache{entries: make(map[string]dispatchContentEntry)}
}

func (c *dispatchContentCache) materialize(
	ctx context.Context,
	rawURL string,
	materializer work.ContentMaterializer,
) (string, work.ContentCleanup, error) {
	if materializer == nil {
		return "", func() {}, fmt.Errorf("Work content materializer is required")
	}
	c.mu.Lock()
	if entry, ok := c.entries[rawURL]; ok {
		c.mu.Unlock()
		return entry.localPath, func() {}, nil
	}
	c.mu.Unlock()
	path, cleanup, err := materializer.MaterializeContentURL(ctx, rawURL)
	if err != nil {
		return "", func() {}, err
	}
	c.mu.Lock()
	if entry, ok := c.entries[rawURL]; ok {
		c.mu.Unlock()
		cleanup()
		return entry.localPath, func() {}, nil
	}
	c.entries[rawURL] = dispatchContentEntry{localPath: path, cleanup: cleanup}
	c.mu.Unlock()
	return path, func() {}, nil
}

func (c *dispatchContentCache) release() {
	c.mu.Lock()
	entries := c.entries
	c.entries = make(map[string]dispatchContentEntry)
	c.mu.Unlock()
	for _, entry := range entries {
		if entry.cleanup != nil {
			entry.cleanup()
		}
	}
}

func (c *ProviderBuildContext) release() {
	if c == nil {
		return
	}
	if c.ContentCache != nil {
		c.ContentCache.release()
	}
}

type providerBehavior interface {
	BuildArgs(ctx context.Context, req workerexecution.ProviderInferenceRequest, skipPermissions bool, buildCtx *ProviderBuildContext) ([]string, error)
	BuildCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) CommandRequest
}

type sharedNonCodexProviderBehavior struct{}

type claudeProviderBehavior struct {
	sharedNonCodexProviderBehavior
	logger logging.Logger
}

type codexProviderBehavior struct {
	logger logging.Logger
}

func providerBehaviorFor(provider string, logger logging.Logger) providerBehavior {
	switch provider {
	case string(modelprovider.ProviderCodex):
		return codexProviderBehavior{logger: logger}
	default:
		return claudeProviderBehavior{logger: logger}
	}
}

func (sharedNonCodexProviderBehavior) BuildCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) CommandRequest {
	return buildBaseProviderCommandRequest(req, args)
}

func (b claudeProviderBehavior) BuildArgs(_ context.Context, req workerexecution.ProviderInferenceRequest, skipPermissions bool, _ *ProviderBuildContext) ([]string, error) {
	logger := logging.EnsureLogger(b.logger)
	if err := unsupportedImageContentError(req.InputTokens, "model provider claude"); err != nil {
		return nil, err
	}
	args := []string{"-p"}
	if skipPermissions {
		logger.Info("inferencer: enabling skip permissions flag for claude dispatcher")
		args = append(args, "--dangerously-skip-permissions")
	}
	if req.Worktree != "" {
		logger.Info("inferencer: adding work directory to arguments")
		args = append(args, "--worktree", req.Worktree)
	}
	if req.SystemPrompt != "" {
		logger.Info("inferencer: adding system prompt to arguments")
		args = append(args, "--system-prompt", req.SystemPrompt)
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	effort, ok := providers.ReasoningEffort(req.ReasoningEffort).Canonical()
	if !ok {
		return nil, fmt.Errorf("unsupported reasoning effort %q", req.ReasoningEffort)
	}
	if effort != "" {
		if effort == "minimal" {
			return nil, fmt.Errorf(`Claude does not support reasoning effort "minimal"`)
		}
		args = append(args, "--effort", effort)
	}
	if req.SessionID != "" {
		logger.Info("inferencer: resuming claude session")
		args = append(args, "--resume", req.SessionID)
	}
	args = append(args, req.UserMessage)
	return args, nil
}

func (b codexProviderBehavior) BuildArgs(ctx context.Context, req workerexecution.ProviderInferenceRequest, skipPermissions bool, buildCtx *ProviderBuildContext) ([]string, error) {
	if err := validateCodexOptionalCapabilities(req); err != nil {
		return nil, err
	}
	logger := logging.EnsureLogger(b.logger)
	args := []string{"exec"} // quiet mode for non-interactive use
	if skipPermissions {
		logger.Debug("inferencer: enabling skip permissions flag for codex dispatcher")
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}

	if req.WorkingDirectory != "" {
		// TODO: we should check and validate the working directory target for an inference dispatch at runtime and handle the request as failing if the working directory is invalid.
		logger.Debug("inferencer: codex passed a working directory argument")
		// args = append(args, "--cd", req.WorkingDirectory)
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	effort, ok := providers.ReasoningEffort(req.ReasoningEffort).Canonical()
	if !ok {
		return nil, fmt.Errorf("unsupported reasoning effort %q", req.ReasoningEffort)
	}
	if effort != "" {
		args = append(args, "--config", `model_reasoning_effort="`+effort+`"`)
	}
	imageArgs, err := codexImageArgs(ctx, req, buildCtx)
	if err != nil {
		return nil, err
	}
	args = append(args, imageArgs...)
	args = append(args, "-")
	return args, nil
}

func (b codexProviderBehavior) BuildCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) CommandRequest {
	commandReq := buildBaseProviderCommandRequest(req, args)
	// Codex CLI reliably preserves multiline prompts when they are streamed
	// over stdin instead of passed as a positional argument.
	commandReq.Stdin = []byte(req.UserMessage)
	return commandReq
}

// BuildCodexStructuredCommand applies the established Codex command and image
// materialization policy for a typed adapter invocation. Cleanup must run after
// the subprocess attempt so dispatch-scoped image files remain available.
func BuildCodexStructuredCommand(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
	skipPermissions bool,
	contentMaterializer work.ContentMaterializer,
) (CommandRequest, func(), error) {
	buildCtx := &ProviderBuildContext{ContentCache: newDispatchContentCache(), ContentMaterializer: contentMaterializer}
	cleanup := func() { buildCtx.release() }
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	args, err := behavior.BuildArgs(ctx, req, skipPermissions, buildCtx)
	if err != nil {
		cleanup()
		return CommandRequest{}, nil, err
	}
	args = append(args[:1], append([]string{"--json"}, args[1:]...)...)
	return behavior.BuildCommandRequest(req, args), cleanup, nil
}

func validateCodexOptionalCapabilities(req workerexecution.ProviderInferenceRequest) error {
	for _, capability := range req.RequiredOptionalCapabilities {
		if capability == workerexecution.RunnerOptionalCapabilityWorktree {
			return errors.New("worktree selection is not supported by the codex runner in v1")
		}
	}
	if req.Worktree != "" && req.WorkingDirectory == "" {
		return errors.New("worktree selection is not supported by the codex runner in v1")
	}
	return nil
}

func buildBaseProviderCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) CommandRequest {
	commandReq := workerexecution.SubprocessRequestBase(req.Dispatch)
	commandReq.Command = string(req.ModelProvider)
	commandReq.Args = append([]string(nil), args...)
	commandReq.Env = buildProviderEnv(req.ProcessEnvironment, req.EnvVars)
	commandReq.WorkDir = req.WorkingDirectory
	commandReq.InputTokens = cloneRawInputTokens(req.InputTokens)
	if req.WorkerType != "" {
		commandReq.WorkerType = req.WorkerType
	}
	if req.WorkstationType != "" {
		commandReq.WorkstationName = req.WorkstationType
	}
	if req.ProjectID != "" {
		commandReq.ProjectID = req.ProjectID
	}
	return commandReq
}

func formatCombinedProviderOutput(result CommandResult) string {
	return strings.Join([]string{
		strings.TrimSpace(string(result.Stderr)),
		strings.TrimSpace(string(result.Stdout)),
	}, "\n")
}

func codexImageArgs(ctx context.Context, req workerexecution.ProviderInferenceRequest, buildCtx *ProviderBuildContext) ([]string, error) {
	tokens := cloneInputTokens(req.InputTokens)
	if len(tokens) == 0 {
		return nil, nil
	}

	var contentMaterializer work.ContentMaterializer
	var cache *dispatchContentCache
	if buildCtx != nil {
		contentMaterializer = buildCtx.ContentMaterializer
		cache = buildCtx.ContentCache
	}

	var args []string
	for tokenIndex, token := range tokens {
		for partIndex, part := range token.Color.Content {
			if part.Type != work.WorkContentPartTypeImage {
				continue
			}
			contentURL, err := codexImageContentURL(part)
			if err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].url: %w", tokenIndex, partIndex, err)
			}
			resolvedURL, err := work.ResolveDispatchContentURL(req.WorkingDirectory, contentURL)
			if err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].url: %w", tokenIndex, partIndex, err)
			}
			localPath, _, err := materializeCodexImageURL(ctx, cache, resolvedURL, contentMaterializer)
			if err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].url: %w", tokenIndex, partIndex, err)
			}
			args = append(args, "-i", localPath)
		}
	}
	return args, nil
}

func codexImageContentURL(part work.WorkContentPart) (string, error) {
	if strings.TrimSpace(part.URL) != "" {
		return part.URL, nil
	}
	if strings.TrimSpace(part.File) != "" {
		return work.FilesystemPathToContentURL(part.File)
	}
	return "", fmt.Errorf("codex image content url is required")
}

func materializeCodexImageURL(ctx context.Context, cache *dispatchContentCache, rawURL string, materializer work.ContentMaterializer) (string, work.ContentCleanup, error) {
	if cache != nil {
		return cache.materialize(ctx, rawURL, materializer)
	}
	if materializer == nil {
		return "", func() {}, fmt.Errorf("Work content materializer is required")
	}
	return materializer.MaterializeContentURL(ctx, rawURL)
}

func formatProviderOutputOrDefault(result CommandResult, fallback string) string {
	for _, output := range []string{
		string(result.Stderr),
		string(result.Stdout),
	} {
		detail := strings.TrimSpace(output)
		if detail != "" {
			return detail
		}
	}
	return fallback
}

// WorkFailureDecisionFromProviderError resolves retry behavior from a normalized
// provider error using the same FailureMetadata projection as WorkResult.
func WorkFailureDecisionFromProviderError(err *ProviderError) workerexecution.WorkFailureDecision {
	return WorkFailureDecisionFromMetadata(WorkFailureMetadataFromError(err))
}

// WorkFailureDecisionFromMetadata resolves retry behavior from durable
// generalized failure metadata carried across runtime boundaries.
// The normalized type is canonical when present; family remains a fallback for
// older or partial metadata that omitted type.
func WorkFailureDecisionFromMetadata(metadata *workerexecution.WorkFailureMetadata) workerexecution.WorkFailureDecision {
	return workerexecution.FailureDecisionFromMetadata(metadata)
}

type providerFailurePolicy struct {
	Family   workerexecution.WorkFailureFamily
	Decision workerexecution.WorkFailureDecision
}

func providerFailurePolicyForReason(reason workerexecution.WorkFailureType) providerFailurePolicy {
	switch reason {
	case workerexecution.WorkFailureTypeThrottled:
		return providerFailurePolicy{
			Family: workerexecution.WorkFailureFamilyThrottle,
			Decision: workerexecution.WorkFailureDecision{
				Retryable:             true,
				TriggersThrottlePause: true,
			},
		}
	case workerexecution.WorkFailureTypeInternalServerError, workerexecution.WorkFailureTypeTimeout:
		return providerFailurePolicy{
			Family:   workerexecution.WorkFailureFamilyRetryable,
			Decision: workerexecution.WorkFailureDecision{Retryable: true},
		}
	case workerexecution.WorkFailureTypeAuthFailure,
		workerexecution.WorkFailureTypePermanentBadRequest,
		workerexecution.WorkFailureTypeUnknown,
		workerexecution.WorkFailureTypeMisconfigured,
		workerexecution.WorkFailureTypeMissingExecutable,
		workerexecution.WorkFailureTypeCommandLineTooLong:
		return providerFailurePolicy{
			Family:   workerexecution.WorkFailureFamilyTerminal,
			Decision: workerexecution.WorkFailureDecision{Terminal: true},
		}
	default:
		return providerFailurePolicy{
			Family:   workerexecution.WorkFailureFamilyTerminal,
			Decision: workerexecution.WorkFailureDecision{Terminal: true},
		}
	}
}

func providerErrorFamilyForType(errorType workerexecution.WorkFailureType) workerexecution.WorkFailureFamily {
	return providerFailurePolicyForReason(errorType).Family
}

// WorkFailureMetadataFromError projects a provider-shaped execution error onto
// the in-process failure contract carried on WorkResult.FailureMetadata.
// FailureSignalTier orders competing invocation failure signals. Lower tiers
// outrank higher tiers when multiple candidates are present for one outcome.
type FailureSignalTier int

const (
	FailureSignalTierCancelTimeout FailureSignalTier = iota
	FailureSignalTierStructured
	FailureSignalTierStderr
	FailureSignalTierExit
)

// ProviderFailureResolution is the winning provider failure outcome plus a
// bounded internal cause excerpt from the selected signal.
type ProviderFailureResolution struct {
	Result        ProviderFailureResult
	InternalCause string
}

// CompetingFailureSignal is one candidate outcome before shared precedence
// selection. Recognized marks whether the tier produced a known failure class
// rather than an unknown fallback excerpt.
type CompetingFailureSignal struct {
	Tier          FailureSignalTier
	Recognized    bool
	Result        ProviderFailureResult
	InternalCause string
}

// SelectFailureByPrecedence collapses competing signals to one authoritative
// outcome using the shared precedence model:
//
//  1. cancellation and timeout
//  2. recognized structured provider errors
//  3. recognized stderr classification
//  4. unrecognized structured provider errors
//  5. unrecognized stderr classification
//  6. generic process-exit fallback
func SelectFailureByPrecedence(signals []CompetingFailureSignal) (ProviderFailureResolution, bool) {
	if len(signals) == 0 {
		return ProviderFailureResolution{}, false
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierCancelTimeout, false); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStructured, true); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStderr, true); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStructured, false); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStderr, false); ok {
		return selected, true
	}
	return selectFailureForTier(signals, FailureSignalTierExit, false)
}

func selectFailureForTier(signals []CompetingFailureSignal, tier FailureSignalTier, recognizedOnly bool) (ProviderFailureResolution, bool) {
	var selected ProviderFailureResolution
	var found bool
	for _, signal := range signals {
		if signal.Tier != tier {
			continue
		}
		if recognizedOnly && !signal.Recognized {
			continue
		}
		selected = ProviderFailureResolution{
			Result:        signal.Result,
			InternalCause: signal.InternalCause,
		}
		found = true
	}
	return selected, found
}
