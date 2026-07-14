package provider

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	claudepkg "github.com/portpowered/infinite-you/pkg/workers/provider/claude"
	codexexitfailure "github.com/portpowered/infinite-you/pkg/workers/provider/codex/exitfailure"
	geminipkg "github.com/portpowered/infinite-you/pkg/workers/provider/gemini"
	kiropkg "github.com/portpowered/infinite-you/pkg/workers/provider/kiro"
	opencodepkg "github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
)

const (
	claudeFailureMessageBytes      = 1024
	claudeFailureScanBytes         = 64 * 1024
	claudeThrottleFailureMessage   = claudepkg.ThrottleFailureMessage
	claudeBadRequestFailureMessage = "Claude rejected the request as invalid."
	claudeAuthFailureMessage       = "Claude authentication failed."
	claudeConfigFailureMessage     = "Claude is not configured correctly."
	claudeServerFailureMessage     = "Claude encountered a temporary server error."
	claudeTimeoutFailureMessage    = claudepkg.TimeoutFailureMessage
	codexAuthFailureMessage        = codexexitfailure.AuthFailureMessage
	codexBadRequestFailureMessage  = "Codex rejected the request as invalid."
	codexThrottleFailureMessage    = "Codex is temporarily unavailable due to usage or capacity limits."
	codexTimeoutFailureMessage     = "Codex request timed out."
	codexServerFailureMessage      = "Codex encountered a temporary server error."
	codexFailureMessageBytes       = 1024
	codexErrorLineScanBytes        = 64 * 1024
	codexWindowsProcessFailureExitCode = codexexitfailure.WindowsProcessFailureExitCode
	codexHighDemandTemporaryErrorsNeedle = codexexitfailure.HighDemandTemporaryErrorsNeedle
	codexGPT56SolUpgradeMessage    = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	opencodeThrottleFailureMessage = opencodepkg.ThrottleFailureMessage
	opencodeBadRequestFailureMessage = opencodepkg.BadRequestFailureMessage
	opencodeTimeoutFailureMessage  = opencodepkg.TimeoutFailureMessage
	opencodeServerFailureMessage   = "OpenCode encountered a temporary server error."
	opencodeFailureMessageBytes    = 512
	geminiTimeoutFailureMessage    = geminipkg.TimeoutFailureMessage
	geminiThrottleFailureMessage   = "The provider is rate limited; retry after capacity becomes available."
)

func failureInput(result CommandResult) claudepkg.FailureInput {
	return claudepkg.FailureInput{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}
}

func ParseClaudeProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := claudepkg.ParseProviderFailure(failureInput(result))
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func ParseCodexProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := codexexitfailure.ParseExitFailure(codexexitfailure.ExitFailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func ParseGeminiProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := geminipkg.ParseProviderFailure(geminipkg.FailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func ParseKiroProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := kiropkg.ParseProviderFailure(kiropkg.FailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func ParseOpenCodeProviderFailure(result CommandResult) ProviderFailureResult {
	parsed := opencodepkg.ParseProviderFailure(opencodepkg.FailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	})
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

func codexTextFailureMessage(reason interfaces.WorkFailureType) string {
	parsed := ParseCodexProviderFailure(CommandResult{ExitCode: 1, Stderr: []byte("ERROR: unexpected status 401")})
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return codexAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return codexBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return codexThrottleFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return codexServerFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return codexTimeoutFailureMessage
	default:
		return parsed.Message
	}
}

func knownKiroFailure(reason interfaces.WorkFailureType) ProviderFailureResult {
	message := ""
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		message = "Kiro authentication failed. Sign in again and retry."
	case interfaces.WorkFailureTypePermanentBadRequest:
		message = "Kiro rejected the request as invalid."
	case interfaces.WorkFailureTypeThrottled:
		message = "Kiro is temporarily unavailable due to usage or capacity limits."
	case interfaces.WorkFailureTypeTimeout:
		message = kiropkg.TimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		message = "Kiro encountered a temporary service error."
	}
	return ProviderFailureResult{Reason: reason, Message: message}
}

func boundUTF8Bytes(message string, limit int) string {
	if limit <= 0 || len(message) <= limit {
		return message
	}
	bounded := []byte(message)[:limit]
	for len(bounded) > 0 && bounded[len(bounded)-1]&0xc0 == 0x80 {
		bounded = bounded[:len(bounded)-1]
	}
	return string(bounded)
}
