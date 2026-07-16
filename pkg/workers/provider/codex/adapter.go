package codex

import (
	"context"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	codexexitfailure "github.com/portpowered/infinite-you/pkg/workers/provider/codex/exitfailure"
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
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{
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
	if input.CommandError == nil && input.CommandResult.ExitCode == 0 &&
		input.DecodeError == nil && input.FlushError == nil && input.ParseError == nil {
		return adapter.FailureResult{}
	}
	flushReason := ""
	if input.FlushReason == adapter.FlushReasonCanceled {
		flushReason = provider.CodexFlushReasonCanceled
	}
	resolution := codexexitfailure.ResolutionInput{
		CommandError: input.CommandError,
		FlushReason:  flushReason,
	}
	exitInput := codexexitfailure.ExitFailureInput{
		Stdout: input.CommandResult.Stdout, Stderr: input.CommandResult.Stderr, ExitCode: input.CommandResult.ExitCode,
	}
	if input.DecodeError != nil || input.FlushError != nil || input.ParseError != nil {
		if resolved, ok := codexexitfailure.ResolveFailure(exitInput, resolution); ok {
			session := codexProviderSessionFromStdout(input.CommandResult.Stdout, resolved.Result)
			return normalizedFailureResult(resolved, session)
		}
		return normalizedFailureResult(codexexitfailure.FailureResolution{
			Result: codexexitfailure.ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeUnknown,
				Message: "Codex did not produce a valid completed response.",
			},
		}, nil)
	}
	resolved, ok := codexexitfailure.ResolveFailure(exitInput, resolution)
	if !ok {
		return adapter.FailureResult{}
	}
	session := codexProviderSessionFromStdout(input.CommandResult.Stdout, resolved.Result)
	return normalizedFailureResult(resolved, session)
}

func codexProviderSessionFromStdout(stdout []byte, resolved codexexitfailure.ExitFailureResult) *workerexecution.ProviderSessionMetadata {
	if terminal, ok := ParseTerminalFailure(stdout); ok &&
		terminal.Type == resolved.Reason && terminal.Message == resolved.Message {
		return terminal.ProviderSession
	}
	return nil
}

func normalizedFailureResult(resolution codexexitfailure.FailureResolution, session *workerexecution.ProviderSessionMetadata) adapter.FailureResult {
	providerError := provider.NewProviderErrorFromResult(provider.ProviderFailureResult{
		Reason: resolution.Result.Reason, Message: resolution.Result.Message,
	}, provider.ProviderFailureInternalCauseError(resolution.InternalCause))
	decision := provider.WorkFailureDecisionFromProviderError(providerError)
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: providerError.Family, Type: providerError.Type, Message: providerError.Message,
		Retry: adapter.RetryGuidance{Retryable: decision.Retryable}, ProviderSession: session,
	}}
}

var _ adapter.Adapter = ResponseAdapter{}
