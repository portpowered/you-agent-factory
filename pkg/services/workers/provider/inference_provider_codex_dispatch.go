package provider

import (
	codexexitfailure "github.com/portpowered/infinite-you/pkg/services/workers/provider/codex/exitfailure"
)

// CodexFailureResolutionInput carries runtime cancellation and flush facts that
// outrank structured, stderr, and exit signals.
type CodexFailureResolutionInput = codexexitfailure.ResolutionInput

const CodexFlushReasonCanceled = codexexitfailure.FlushReasonCanceled

// ResolveCodexProviderFailure dispatches Codex exit-failure resolution through
// the Codex-owned package and maps neutral outcomes for shared orchestration.
func ResolveCodexProviderFailure(result CommandResult, input CodexFailureResolutionInput) (ProviderFailureResolution, bool) {
	resolved, ok := codexexitfailure.ResolveFailure(exitFailureInputFromCommand(result), input)
	if !ok {
		return ProviderFailureResolution{}, false
	}
	return ProviderFailureResolution{
		Result: ProviderFailureResult{
			Reason:  resolved.Result.Reason,
			Message: resolved.Result.Message,
		},
		InternalCause: resolved.InternalCause,
	}, true
}

// ParseCodexProviderFailure dispatches bounded Codex subprocess parsing through
// the Codex-owned package.
func ParseCodexProviderFailure(result CommandResult) ProviderFailureResult {
	resolved, ok := codexexitfailure.ResolveFailure(exitFailureInputFromCommand(result), codexexitfailure.ResolutionInput{})
	if ok {
		return ProviderFailureResult{Reason: resolved.Result.Reason, Message: resolved.Result.Message}
	}
	parsed := codexexitfailure.ParseExitFailure(exitFailureInputFromCommand(result))
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

// ParseCodexProviderFailureLayers returns structured or stderr-classified Codex
// outcomes without applying cross-signal precedence.
func ParseCodexProviderFailureLayers(result CommandResult) ProviderFailureResult {
	parsed := codexexitfailure.ParseFailureLayers(exitFailureInputFromCommand(result))
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

// CodexStructuredStreamReportingOutcome classifies terminal JSONL stdout without
// applying process-exit stderr or exit-status fallback.
func CodexStructuredStreamReportingOutcome(stdout []byte) (ProviderFailureResult, bool) {
	parsed, ok := codexexitfailure.StructuredStreamReportingOutcome(stdout)
	if !ok {
		return ProviderFailureResult{}, false
	}
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}, true
}

// CodexProcessExitReportingOutcome classifies stderr and exit status without
// structured-stream JSONL signals.
func CodexProcessExitReportingOutcome(result CommandResult) ProviderFailureResult {
	parsed := codexexitfailure.ProcessExitReportingOutcome(exitFailureInputFromCommand(result))
	return ProviderFailureResult{Reason: parsed.Reason, Message: parsed.Message}
}

// CodexSanitizedFailureFixture is the safe public projection used by Codex
// alignment tests.
type CodexSanitizedFailureFixture = codexexitfailure.SanitizedFailureFixture

// CodexSanitizedFailureFixtureFromResolution projects one winning Codex failure
// resolution onto the sanitized alignment fixture shape.
func CodexSanitizedFailureFixtureFromResolution(resolution ProviderFailureResolution) CodexSanitizedFailureFixture {
	providerErr := NewProviderErrorFromResult(resolution.Result, ProviderFailureInternalCauseError(resolution.InternalCause))
	decision := WorkFailureDecisionFromProviderError(providerErr)
	return CodexSanitizedFailureFixture{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		InternalCause: resolution.InternalCause,
	}
}

// CodexSanitizedFailureFixtureFromProviderError projects a normalized provider
// error onto the sanitized alignment fixture shape.
func CodexSanitizedFailureFixtureFromProviderError(providerErr *ProviderError) CodexSanitizedFailureFixture {
	if providerErr == nil {
		return CodexSanitizedFailureFixture{}
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	internalCause := ""
	if providerErr.Cause != nil {
		internalCause = providerErr.Cause.Error()
	}
	return CodexSanitizedFailureFixture{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		InternalCause: internalCause,
	}
}

func exitFailureInputFromCommand(result CommandResult) codexexitfailure.ExitFailureInput {
	return codexexitfailure.ExitFailureInput{
		Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode,
	}
}

func extractCodexErrorLine(result CommandResult) (string, bool) {
	return codexexitfailure.ExtractErrorLine(exitFailureInputFromCommand(result))
}
