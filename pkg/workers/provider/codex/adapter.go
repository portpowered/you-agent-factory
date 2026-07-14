package codex

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/codex/exitfailure"
)

// ResponseAdapter exposes the production Codex JSONL decoder, final parser,
// and failure classifier through the provider-neutral adapter contract.
type ResponseAdapter struct{}

func NewResponseAdapter() adapter.Adapter { return ResponseAdapter{} }

func (ResponseAdapter) Identity() adapter.Identity { return "codex" }

func (ResponseAdapter) BuildCommand(ctx context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	command, cleanup, err := provider.BuildCodexStructuredCommand(ctx, input.Request, input.SkipPermissions, input.MaterializeOptions)
	return adapter.CommandBuildResult{Request: command, Cleanup: cleanup}, err
}

func (ResponseAdapter) NewDecoder(_ context.Context, input adapter.DecoderContext) (adapter.Decoder, error) {
	return NewDecoder(input), nil
}

func (ResponseAdapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	parsed, err := ParseFinalOutput(input.CommandResult.Stdout)
	if err != nil {
		return adapter.FinalParseResult{}, err
	}
	return adapter.FinalParseResult{Response: interfaces.InferenceResponse{
		Content: parsed.Content, ProviderSession: parsed.ProviderSession,
	}}, nil
}

func (ResponseAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageSnapshots: true, ReasoningSummaries: true,
		ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
	}}, nil
}

func (ResponseAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if errors.Is(input.CommandError, context.Canceled) {
		return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, "Codex execution was canceled.", nil)
	}
	if errors.Is(input.CommandError, context.DeadlineExceeded) || input.CommandResult.ExitCode == 124 {
		return normalizedFailureResult(interfaces.WorkFailureTypeTimeout, "Codex execution timed out.", nil)
	}
	if input.FlushReason == adapter.FlushReasonCanceled {
		return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, "Codex execution was canceled.", nil)
	}
	if failure, ok := ParseTerminalFailure(input.CommandResult.Stdout); ok {
		return terminalFailureResult(failure)
	}
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		parsed := exitfailure.ParseExitFailure(exitfailure.ExitFailureInput{
			Stdout: input.CommandResult.Stdout, Stderr: input.CommandResult.Stderr, ExitCode: input.CommandResult.ExitCode,
		})
		return normalizedFailureResult(parsed.Reason, parsed.Message, nil)
	}
	if input.DecodeError != nil || input.FlushError != nil || input.ParseError != nil {
		return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, "Codex did not produce a valid completed response.", nil)
	}
	return adapter.FailureResult{}
}

func terminalFailureResult(failure TerminalFailure) adapter.FailureResult {
	return normalizedFailureResult(failure.Type, failure.Message, failure.ProviderSession)
}

func normalizedFailureResult(failureType interfaces.WorkFailureType, message string, session *interfaces.ProviderSessionMetadata) adapter.FailureResult {
	providerError := provider.NewProviderErrorFromResult(provider.ProviderFailureResult{Reason: failureType, Message: message}, nil)
	decision := provider.WorkFailureDecisionFromProviderError(providerError)
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: providerError.Family, Type: providerError.Type, Message: providerError.Message,
		Retry: adapter.RetryGuidance{Retryable: decision.Retryable}, ProviderSession: session,
	}}
}

var _ adapter.Adapter = ResponseAdapter{}
