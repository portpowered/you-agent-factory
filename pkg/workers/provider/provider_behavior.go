package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/work/content"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	cursorpkg "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
)

const (
	codexErrorLineScanBytes            = 64 * 1024
	codexWindowsProcessFailureExitCode = 4294967295
)

const codexHighDemandTemporaryErrorsNeedle = "we're currently experiencing high demand, which may cause temporary errors."

// Cursor is commonly installed through a Windows command shim whose practical
// command-line limit is lower than CreateProcess' documented maximum. Keep the
// prompt below the observed 8 KiB boundary and materialize larger prompts.
const cursorWindowsPromptArgumentLimit = 7 * 1024

// ProviderBuildContext carries dispatch-scoped resources for provider argument building.
type ProviderBuildContext struct {
	ContentCache    *materialize.DispatchCache
	MaterializeOpts *materialize.Options
	operatingSystem string
	tempDir         string
	cleanup         []func()
}

func (c *ProviderBuildContext) release() {
	if c == nil {
		return
	}
	for index := len(c.cleanup) - 1; index >= 0; index-- {
		c.cleanup[index]()
	}
	if c.ContentCache != nil {
		c.ContentCache.Release()
	}
}

func (c *ProviderBuildContext) registerCleanup(cleanup func()) {
	if c != nil && cleanup != nil {
		c.cleanup = append(c.cleanup, cleanup)
	}
}

type providerBehavior interface {
	BuildArgs(ctx context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, buildCtx *ProviderBuildContext) ([]string, error)
	BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest
}

type sharedNonCodexProviderBehavior struct{}

type claudeProviderBehavior struct {
	sharedNonCodexProviderBehavior
	logger logging.Logger
}

type codexProviderBehavior struct {
	logger logging.Logger
}

type geminiProviderBehavior struct {
	sharedNonCodexProviderBehavior
	logger logging.Logger
}

type kiroProviderBehavior struct {
	logger logging.Logger
}

type cursorProviderBehavior struct {
	sharedNonCodexProviderBehavior
	logger logging.Logger
}

type openCodeProviderBehavior struct {
	sharedNonCodexProviderBehavior
	logger logging.Logger
}

func providerBehaviorFor(provider string, logger logging.Logger) providerBehavior {
	switch provider {
	case string(interfaces.ModelProviderCodex):
		return codexProviderBehavior{logger: logger}
	case string(interfaces.ModelProviderGemini):
		return geminiProviderBehavior{logger: logger}
	case string(interfaces.ModelProviderKiro):
		return kiroProviderBehavior{logger: logger}
	case string(interfaces.ModelProviderCursor):
		return cursorProviderBehavior{logger: logger}
	case string(interfaces.ModelProviderOpenCode):
		return openCodeProviderBehavior{logger: logger}
	default:
		return claudeProviderBehavior{logger: logger}
	}
}

func (sharedNonCodexProviderBehavior) BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	return buildBaseProviderCommandRequest(req, args)
}

func (b claudeProviderBehavior) BuildArgs(_ context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, _ *ProviderBuildContext) ([]string, error) {
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
	if req.SessionID != "" {
		logger.Info("inferencer: resuming claude session")
		args = append(args, "--resume", req.SessionID)
	}
	args = append(args, req.UserMessage)
	return args, nil
}

func (b codexProviderBehavior) BuildArgs(ctx context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, buildCtx *ProviderBuildContext) ([]string, error) {
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
	imageArgs, err := codexImageArgs(ctx, req, buildCtx)
	if err != nil {
		return nil, err
	}
	args = append(args, imageArgs...)
	args = append(args, "-")
	return args, nil
}

func (b codexProviderBehavior) BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	commandReq := buildBaseProviderCommandRequest(req, args)
	// Codex CLI reliably preserves multiline prompts when they are streamed
	// over stdin instead of passed as a positional argument.
	commandReq.Stdin = []byte(req.UserMessage)
	return commandReq
}

func (b geminiProviderBehavior) BuildArgs(_ context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, _ *ProviderBuildContext) ([]string, error) {
	if err := validateGeminiOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := []string{"--prompt", req.UserMessage}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	if skipPermissions {
		args = append(args, "--approval-mode", "yolo", "--sandbox", "false")
	}
	return args, nil
}

func (b kiroProviderBehavior) BuildArgs(_ context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, _ *ProviderBuildContext) ([]string, error) {
	if err := validateKiroOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := []string{"chat", "--no-interactive"}
	if req.SessionID != "" {
		args = append(args, "--resume-id", req.SessionID)
	}
	if skipPermissions {
		args = append(args, "--trust-all-tools")
	}
	if prompt := buildKiroPrompt(req); prompt != "" {
		args = append(args, prompt)
	}
	return args, nil
}

func (kiroProviderBehavior) BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	return buildBaseProviderCommandRequest(req, args)
}

func (b cursorProviderBehavior) BuildArgs(_ context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, buildCtx *ProviderBuildContext) ([]string, error) {
	if err := validateCursorOptionalCapabilities(req); err != nil {
		return nil, err
	}
	var args []string
	if skipPermissions {
		args = append(args, "-f")
	}
	args = append(args, "-p")
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	if req.WorkingDirectory != "" {
		args = append(args, "--workspace", req.WorkingDirectory)
	}

	if req.Worktree != "" {
		args = append(args, "--worktree", req.Worktree)
	}

	args = append(args, "--output-format", cursorpkg.CursorOutputFormatStreamJSON, "--stream-partial-output")

	prompt := buildCursorPrompt(req)
	operatingSystem := runtime.GOOS
	if buildCtx != nil && buildCtx.operatingSystem != "" {
		operatingSystem = buildCtx.operatingSystem
	}
	if operatingSystem == "windows" && len(utf16.Encode([]rune(prompt))) > cursorWindowsPromptArgumentLimit {
		tempDir := req.WorkingDirectory
		if tempDir == "" {
			tempDir = "."
		}
		if buildCtx != nil && buildCtx.tempDir != "" {
			tempDir = buildCtx.tempDir
		}
		promptFile, err := b.writeTempFile(tempDir, prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to write temporary prompt file: %w", err)
		}
		buildCtx.registerCleanup(func() { _ = os.Remove(promptFile) })
		prompt = "@" + promptFile
	}
	args = append(args, prompt)
	return args, nil
}

func buildCursorPrompt(req interfaces.ProviderInferenceRequest) string {
	prompt := strings.TrimSpace(req.UserMessage)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		prompt = buildKiroPrompt(req)
	}
	return prompt
}

func (b cursorProviderBehavior) writeTempFile(tempDir, prompt string) (string, error) {
	f, err := os.CreateTemp(tempDir, "cursor_prompt_*.md")
	if err != nil {
		b.logger.Error("failed to create temporary prompt file", "error", err)
		return "", err
	}
	path := f.Name()
	if _, err = f.WriteString(prompt); err != nil {
		b.logger.Error("failed to write to temporary prompt file", "error", err)
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (b openCodeProviderBehavior) BuildArgs(_ context.Context, req interfaces.ProviderInferenceRequest, skipPermissions bool, _ *ProviderBuildContext) ([]string, error) {
	if err := validateOpenCodeOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := []string{"run"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.OpenCodeAgent != "" {
		args = append(args, "--agent", req.OpenCodeAgent)
	}
	if req.SessionID != "" {
		args = append(args, "--session", req.SessionID)
	}
	if req.WorkingDirectory != "" {
		args = append(args, "--dir", req.WorkingDirectory)
	}
	if skipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, req.UserMessage)
	return args, nil
}

func validateGeminiOptionalCapabilities(req interfaces.ProviderInferenceRequest) error {
	unsupported := map[interfaces.RunnerOptionalCapability]string{
		interfaces.RunnerOptionalCapabilityImageInput:       "image input is not supported by the gemini runner in v1",
		interfaces.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the gemini runner in v1",
		interfaces.RunnerOptionalCapabilitySessionResume:    "session resume is not supported by the gemini runner in v1",
		interfaces.RunnerOptionalCapabilityWorkingDirectory: "working directory is not supported by the gemini runner in v1",
		interfaces.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the gemini runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	if req.SessionID != "" {
		return errors.New("session resume is not supported by the gemini runner in v1")
	}
	return nil
}

func validateCodexOptionalCapabilities(req interfaces.ProviderInferenceRequest) error {
	for _, capability := range req.RequiredOptionalCapabilities {
		if capability == interfaces.RunnerOptionalCapabilityWorktree {
			return errors.New("worktree selection is not supported by the codex runner in v1")
		}
	}
	if req.Worktree != "" && req.WorkingDirectory == "" {
		return errors.New("worktree selection is not supported by the codex runner in v1")
	}
	return nil
}

func validateKiroOptionalCapabilities(req interfaces.ProviderInferenceRequest) error {
	unsupported := map[interfaces.RunnerOptionalCapability]string{
		interfaces.RunnerOptionalCapabilityImageInput:       "image input is not supported by the kiro runner in v1",
		interfaces.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the kiro runner in v1",
		interfaces.RunnerOptionalCapabilityWorkingDirectory: "working directory is not supported by the kiro runner in v1",
		interfaces.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the kiro runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	return nil
}

func validateCursorOptionalCapabilities(req interfaces.ProviderInferenceRequest) error {
	unsupported := map[interfaces.RunnerOptionalCapability]string{
		interfaces.RunnerOptionalCapabilityImageInput:       "image input is not supported by the cursor-cli runner in v1",
		interfaces.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the cursor-cli runner in v1",
		interfaces.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the cursor-cli runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	return nil
}

func validateOpenCodeOptionalCapabilities(req interfaces.ProviderInferenceRequest) error {
	unsupported := map[interfaces.RunnerOptionalCapability]string{
		interfaces.RunnerOptionalCapabilityImageInput:       "image input is not supported by the opencode runner in v1",
		interfaces.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the opencode runner in v1",
		interfaces.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the opencode runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	return nil
}

func buildKiroPrompt(req interfaces.ProviderInferenceRequest) string {
	systemPrompt := strings.TrimSpace(req.SystemPrompt)
	userMessage := strings.TrimSpace(req.UserMessage)
	switch {
	case systemPrompt == "":
		return userMessage
	case userMessage == "":
		return systemPrompt
	default:
		return "System instructions:\n" + systemPrompt + "\n\nUser request:\n" + userMessage
	}
}

func buildBaseProviderCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	commandReq := workerprocess.SubprocessRequestBase(req.Dispatch)
	commandReq.Command = string(req.ModelProvider)
	commandReq.Args = append([]string(nil), args...)
	commandReq.Env = buildProviderEnv(req.EnvVars)
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

func extractCodexErrorLine(result CommandResult) (string, bool) {
	combined := strings.Join([]string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}, "\n")

	var match string
	for _, line := range strings.Split(combined, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ERROR:") {
			match = trimmed
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
}

func tailForCodexErrorScan(output []byte) string {
	if len(output) <= codexErrorLineScanBytes {
		return string(output)
	}
	return string(output[len(output)-codexErrorLineScanBytes:])
}

func codexImageArgs(ctx context.Context, req interfaces.ProviderInferenceRequest, buildCtx *ProviderBuildContext) ([]string, error) {
	tokens := cloneInputTokens(req.InputTokens)
	if len(tokens) == 0 {
		return nil, nil
	}

	var materializeOpts *materialize.Options
	var cache *materialize.DispatchCache
	if buildCtx != nil {
		materializeOpts = buildCtx.MaterializeOpts
		cache = buildCtx.ContentCache
	}

	var args []string
	for tokenIndex, token := range tokens {
		for partIndex, part := range token.Color.Content {
			if part.Type != interfaces.WorkContentPartTypeImage {
				continue
			}
			contentURL, err := codexImageContentURL(part)
			if err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].url: %w", tokenIndex, partIndex, err)
			}
			resolvedURL, err := content.ResolveDispatchContentURL(req.WorkingDirectory, contentURL)
			if err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].url: %w", tokenIndex, partIndex, err)
			}
			localPath, _, err := materializeCodexImageURL(ctx, cache, resolvedURL, materializeOpts)
			if err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].url: %w", tokenIndex, partIndex, err)
			}
			args = append(args, "-i", localPath)
		}
	}
	return args, nil
}

func codexImageContentURL(part interfaces.WorkContentPart) (string, error) {
	if strings.TrimSpace(part.URL) != "" {
		return part.URL, nil
	}
	if strings.TrimSpace(part.File) != "" {
		return content.FilesystemPathToContentURL(part.File)
	}
	return "", fmt.Errorf("codex image content url is required")
}

func materializeCodexImageURL(ctx context.Context, cache *materialize.DispatchCache, rawURL string, opts *materialize.Options) (string, materialize.CleanupFunc, error) {
	if cache != nil {
		return cache.MaterializeContentURL(ctx, rawURL, opts)
	}
	return materialize.MaterializeContentURL(ctx, rawURL, opts)
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
func WorkFailureDecisionFromProviderError(err *ProviderError) interfaces.WorkFailureDecision {
	return WorkFailureDecisionFromMetadata(WorkFailureMetadataFromError(err))
}

// WorkFailureDecisionFromMetadata resolves retry behavior from durable
// generalized failure metadata carried across runtime boundaries.
// The normalized type is canonical when present; family remains a fallback for
// older or partial metadata that omitted type.
func WorkFailureDecisionFromMetadata(metadata *interfaces.WorkFailureMetadata) interfaces.WorkFailureDecision {
	if metadata == nil {
		return interfaces.WorkFailureDecision{}
	}
	if metadata.Type != "" {
		return providerFailurePolicyForReason(metadata.Type).Decision
	}
	return providerFailureDecisionForFamily(metadata.Family)
}

type providerFailurePolicy struct {
	Family   interfaces.WorkFailureFamily
	Decision interfaces.WorkFailureDecision
}

func providerFailurePolicyForReason(reason interfaces.WorkFailureType) providerFailurePolicy {
	switch reason {
	case interfaces.WorkFailureTypeThrottled:
		return providerFailurePolicy{
			Family: interfaces.WorkFailureFamilyThrottle,
			Decision: interfaces.WorkFailureDecision{
				Retryable:             true,
				TriggersThrottlePause: true,
			},
		}
	case interfaces.WorkFailureTypeInternalServerError, interfaces.WorkFailureTypeTimeout:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyRetryable,
			Decision: interfaces.WorkFailureDecision{Retryable: true},
		}
	case interfaces.WorkFailureTypeAuthFailure,
		interfaces.WorkFailureTypePermanentBadRequest,
		interfaces.WorkFailureTypeUnknown,
		interfaces.WorkFailureTypeMisconfigured,
		interfaces.WorkFailureTypeMissingExecutable,
		interfaces.WorkFailureTypeCommandLineTooLong:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyTerminal,
			Decision: interfaces.WorkFailureDecision{Terminal: true},
		}
	default:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyTerminal,
			Decision: interfaces.WorkFailureDecision{Terminal: true},
		}
	}
}

func providerFailureDecisionForFamily(family interfaces.WorkFailureFamily) interfaces.WorkFailureDecision {
	switch family {
	case interfaces.WorkFailureFamilyRetryable:
		return interfaces.WorkFailureDecision{Retryable: true}
	case interfaces.WorkFailureFamilyThrottle:
		return interfaces.WorkFailureDecision{Retryable: true, TriggersThrottlePause: true}
	case interfaces.WorkFailureFamilyTerminal:
		return interfaces.WorkFailureDecision{Terminal: true}
	default:
		return interfaces.WorkFailureDecision{Terminal: true}
	}
}

func providerErrorFamilyForType(errorType interfaces.WorkFailureType) interfaces.WorkFailureFamily {
	return providerFailurePolicyForReason(errorType).Family
}

// WorkFailureMetadataFromError projects a provider-shaped execution error onto
// the in-process failure contract carried on WorkResult.FailureMetadata.
