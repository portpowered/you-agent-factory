package workers

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
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
	BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) []string
	BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest
	FormatExitFailure(provider string, result CommandResult) string
	ClassifyExitFailure(result CommandResult) interfaces.ProviderErrorType
	FormatTimeoutFailure(result CommandResult) string
}

type claudeProviderBehavior struct {
	logger logging.Logger
}

type codexProviderBehavior struct {
	logger logging.Logger
}

func providerBehaviorFor(provider string, logger logging.Logger) providerBehavior {
	switch provider {
	case string(ModelProviderCodex):
		return codexProviderBehavior{logger: logger}
	default:
		return claudeProviderBehavior{logger: logger}
	}
}

func providerBehaviorForErrorClassification(provider string) providerBehavior {
	switch provider {
	case string(ModelProviderClaude):
		return claudeProviderBehavior{}
	default:
		return codexProviderBehavior{}
	}
}

func (b claudeProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) []string {
	logger := logging.EnsureLogger(b.logger)
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
	return args
}

func (b claudeProviderBehavior) BuildCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	return buildBaseProviderCommandRequest(req, args)
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

func (b claudeProviderBehavior) FormatTimeoutFailure(result CommandResult) string {
	return formatProviderOutputOrDefault(result, "execution timeout")
}

func (b codexProviderBehavior) BuildArgs(req interfaces.ProviderInferenceRequest, skipPermissions bool) []string {
	logger := logging.EnsureLogger(b.logger)
	args := []string{"exec"} // quiet mode for non-interactive use
	if skipPermissions {
		logger.Debug("inferencer: enabling skip permissions flag for codex dispatcher")
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}

	if req.Worktree != "" {
		logger.Debug("inferencer: codex passed a worktree argument, unsupported ignoring silently", "worktree", req.Worktree)
	}

	if req.WorkingDirectory != "" {
		// TODO: we should check and validate the working directory target for an inference dispatch at runtime and handle the request as failing if the working directory is invalid.
		logger.Debug("inferencer: codex passed a working directory argument", "working_directory", req.WorkingDirectory)
		// args = append(args, "--cd", req.WorkingDirectory)
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	args = append(args, "-")
	return args
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

func buildBaseProviderCommandRequest(req interfaces.ProviderInferenceRequest, args []string) CommandRequest {
	commandReq := subprocessRequestBase(req.Dispatch)
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
