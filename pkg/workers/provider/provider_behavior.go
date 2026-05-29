package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

const (
	codexErrorLineScanBytes            = 64 * 1024
	codexWindowsProcessFailureExitCode = 4294967295
)

const codexHighDemandTemporaryErrorsNeedle = "we're currently experiencing high demand, which may cause temporary errors."

var codexThrottledFailureNeedles = []string{
	"rate limit",
	"too many requests",
	"429",
	"throttl",
	"at capacity",
	"model capacity",
	"try a different model",
	"usage limit",
}

var codexTemporaryServerFailureNeedles = []string{
	"unexpected status 500",
	"unexpected status 502",
	"unexpected status 503",
	"unexpected status 504",
	"server_error",
	"internal server error",
	"overloaded",
	"server had an error",
	"connection reset by peer",
	codexHighDemandTemporaryErrorsNeedle,
}

type providerBehavior interface {
	BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error)
	BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest
	FormatExitFailure(provider string, result CommandResult) string
	ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType
	FormatTimeoutFailure(result CommandResult) string
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
	sharedNonCodexProviderBehavior
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
	case string(ModelProviderCodex):
		return codexProviderBehavior{logger: logger}
	case string(ModelProviderGemini):
		return geminiProviderBehavior{logger: logger}
	case string(ModelProviderKiro):
		return kiroProviderBehavior{logger: logger}
	case string(ModelProviderCursor):
		return cursorProviderBehavior{logger: logger}
	case string(ModelProviderOpenCode):
		return openCodeProviderBehavior{logger: logger}
	default:
		return claudeProviderBehavior{logger: logger}
	}
}

func providerBehaviorForErrorClassification(provider string) providerBehavior {
	switch provider {
	case string(ModelProviderClaude):
		return claudeProviderBehavior{}
	case string(ModelProviderGemini):
		return geminiProviderBehavior{}
	case string(ModelProviderKiro):
		return kiroProviderBehavior{}
	case string(ModelProviderCursor):
		return cursorProviderBehavior{}
	case string(ModelProviderOpenCode):
		return openCodeProviderBehavior{}
	default:
		return codexProviderBehavior{}
	}
}

func (sharedNonCodexProviderBehavior) BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	return buildBaseProviderCommandRequest(req, args)
}

func (sharedNonCodexProviderBehavior) FormatTimeoutFailure(result CommandResult) string {
	return formatProviderOutputOrDefault(result, "execution timeout")
}

func (sharedNonCodexProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	return formatProviderOutputOrDefault(result, fmt.Sprintf("%s exited with code %d", provider, result.ExitCode))
}

func (sharedNonCodexProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	normalizedOutput := strings.ToLower(formatCombinedProviderOutput(result))
	switch {
	case containsAny(normalizedOutput, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		return interfaces.ProviderErrorTypeAuthFailure
	case containsAny(normalizedOutput, "invalid argument", "bad request", "invalid request"):
		return interfaces.ProviderErrorTypePermanentBadRequest
	case containsAny(normalizedOutput, "rate limit", "too many requests", "resource exhausted", "429"):
		return interfaces.ProviderErrorTypeThrottled
	case containsAny(normalizedOutput, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		return interfaces.ProviderErrorTypeInternalServerError
	case result.ExitCode == 124 || containsAny(normalizedOutput, "deadline exceeded", "timed out", "timeout"):
		return interfaces.ProviderErrorTypeTimeout
	default:
		return interfaces.ProviderErrorTypeUnknown
	}
}

func (b claudeProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
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
		logger.Info("inferencer: adding work directory to arguments", "worktree", req.Worktree)
		args = append(args, "--worktree", req.Worktree)
	}
	if req.SystemPrompt != "" {
		logger.Info("inferencer: adding system prompt to arguments", "system-prompt", req.SystemPrompt)
		args = append(args, "--system-prompt", req.SystemPrompt)
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		logger.Info("inferencer: resuming claude session", "session_id", req.SessionID)
		args = append(args, "--resume", req.SessionID)
	}
	args = append(args, req.UserMessage)
	return args, nil
}

func (b claudeProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	return fmt.Sprintf("%s exited with code %d", provider, result.ExitCode)
}

func (b claudeProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	normalizedOutput := strings.ToLower(formatCombinedProviderOutput(result))
	switch {
	case containsAny(normalizedOutput, `"type":"authentication_error"`, `"type":"permission_error"`, "api key", "authentication error", "permission error", "unauthorized", "forbidden"):
		return interfaces.ProviderErrorTypeAuthFailure
	case containsAny(normalizedOutput, `"type":"invalid_request_error"`, "invalid_request_error", "bad request", "invalid request", "request_too_large"):
		return interfaces.ProviderErrorTypePermanentBadRequest
	case containsAny(normalizedOutput, `"type":"rate_limit_error"`, `"type":"overloaded_error"`, "rate limit", "too many requests", "overloaded", "529"):
		return interfaces.ProviderErrorTypeThrottled
	case containsAny(normalizedOutput, `"type":"api_error"`, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		return interfaces.ProviderErrorTypeInternalServerError
	case result.ExitCode == 124 || containsAny(normalizedOutput, "deadline exceeded", "timed out", "timeout"):
		return interfaces.ProviderErrorTypeTimeout
	default:
		return interfaces.ProviderErrorTypeUnknown
	}
}

func (b codexProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
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
		logger.Debug("inferencer: codex passed a working directory argument", "working_directory", req.WorkingDirectory)
		// args = append(args, "--cd", req.WorkingDirectory)
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	imageArgs, err := codexImageArgs(req)
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

func (b codexProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	if codexError, ok := extractCodexErrorLine(result); ok {
		return codexError
	}
	return fmt.Sprintf("%s exited with code %d", provider, result.ExitCode)
}

func (b codexProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	normalizedOutput := strings.ToLower(formatCodexOutputForClassification(result))
	switch {
	case containsAny(normalizedOutput, `"type":"authentication_error"`, "authentication_error", "api key", "unauthorized", "forbidden", "401 unauthorized", "403 forbidden"):
		return interfaces.ProviderErrorTypeAuthFailure
	case containsAny(normalizedOutput, `"type":"invalid_request_error"`, "invalid_request_error", "bad request", "400 item", "400 previous response", "400 ") && !containsAny(normalizedOutput, "timeout"):
		return interfaces.ProviderErrorTypePermanentBadRequest
	case containsAny(normalizedOutput, codexThrottledFailureNeedles...):
		return interfaces.ProviderErrorTypeThrottled
	case containsAny(normalizedOutput, codexTemporaryServerFailureNeedles...):
		return interfaces.ProviderErrorTypeInternalServerError
	case result.ExitCode == 124 || containsAny(normalizedOutput, "deadline exceeded", "timed out", "timeout"):
		return interfaces.ProviderErrorTypeTimeout
	case result.ExitCode == codexWindowsProcessFailureExitCode:
		// Windows sometimes reports interrupted Codex subprocess failures as
		// 4294967295 without any audited provider signal. Keep that path on the
		// shared retryable provider/process-failure class instead of falling
		// through to a terminal bucket.
		return interfaces.ProviderErrorTypeInternalServerError
	default:
		return interfaces.ProviderErrorTypeUnknown
	}
}

func (b codexProviderBehavior) FormatTimeoutFailure(result CommandResult) string {
	if codexError, ok := extractCodexErrorLine(result); ok {
		return codexError
	}
	return formatProviderOutputOrDefault(result, "execution timeout")
}

func (b geminiProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
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

func (b geminiProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	return b.sharedNonCodexProviderBehavior.FormatExitFailure(provider, result)
}

func (b geminiProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	return b.sharedNonCodexProviderBehavior.ClassifyExitFailure(result)
}

func (b kiroProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
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

func (b kiroProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	return b.sharedNonCodexProviderBehavior.FormatExitFailure(provider, result)
}

func (b kiroProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	return b.sharedNonCodexProviderBehavior.ClassifyExitFailure(result)
}

func (b cursorProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
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
	prompt := strings.TrimSpace(req.UserMessage)
	if systemPrompt := strings.TrimSpace(req.SystemPrompt); systemPrompt != "" {
		prompt = buildKiroPrompt(req)
	}
	args = append(args, prompt)
	return args, nil
}

func (b cursorProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	return codexProviderBehavior{}.FormatExitFailure(provider, result)
}

func (b cursorProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	return codexProviderBehavior{}.ClassifyExitFailure(result)
}

func (b openCodeProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
	if err := validateOpenCodeOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := []string{"run"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
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

func (b openCodeProviderBehavior) FormatExitFailure(provider string, result CommandResult) string {
	return b.sharedNonCodexProviderBehavior.FormatExitFailure(provider, result)
}

func (b openCodeProviderBehavior) ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType {
	return b.sharedNonCodexProviderBehavior.ClassifyExitFailure(result)
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
	if req.Worktree != "" {
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

func formatCodexOutputForClassification(result CommandResult) string {
	if codexError, ok := extractCodexErrorLine(result); ok {
		return codexError
	}
	return formatCombinedProviderOutput(result)
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

func codexImageArgs(req interfaces.ProviderInferenceRequest) ([]string, error) {
	tokens := cloneInputTokens(req.InputTokens)
	if len(tokens) == 0 {
		return nil, nil
	}

	var args []string
	for tokenIndex, token := range tokens {
		for partIndex, part := range token.Color.Content {
			if part.Type != interfaces.WorkContentPartTypeImage {
				continue
			}
			if err := validateCodexImageFile(req.WorkingDirectory, part.File); err != nil {
				return nil, fmt.Errorf("input_tokens[%d].color.content[%d].file: %w", tokenIndex, partIndex, err)
			}
			args = append(args, "-i", part.File)
		}
	}
	return args, nil
}

func validateCodexImageFile(workingDirectory, imageFile string) error {
	if strings.TrimSpace(imageFile) == "" {
		return fmt.Errorf("codex image content file is required")
	}

	statPath := imageFile
	if workingDirectory != "" && !filepath.IsAbs(filepath.FromSlash(imageFile)) {
		statPath = filepath.Join(workingDirectory, filepath.FromSlash(imageFile))
	}
	info, err := os.Stat(statPath)
	if err != nil {
		return fmt.Errorf("codex image content file %q is not readable: %w", imageFile, err)
	}
	if info.IsDir() {
		return fmt.Errorf("codex image content file %q is a directory", imageFile)
	}
	file, err := os.Open(statPath)
	if err != nil {
		return fmt.Errorf("codex image content file %q is not readable: %w", imageFile, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("codex image content file %q could not be closed after validation: %w", imageFile, err)
	}
	return nil
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
