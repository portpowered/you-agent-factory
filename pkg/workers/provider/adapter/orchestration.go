package adapter

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

// CommandOutcome describes the subprocess result without imposing retry or
// transport presentation policy.
type CommandOutcome string

const (
	CommandOutcomeCompleted     CommandOutcome = "completed"
	CommandOutcomeProcessFailed CommandOutcome = "process_failed"
	CommandOutcomeCanceled      CommandOutcome = "canceled"
)

// StreamingCommandRunner owns subprocess IO. It must serialize observe calls
// in the order chunks are received and stop the command when observe fails.
type StreamingCommandRunner interface {
	Run(context.Context, workerprocess.CommandRequest, func(Observation) error) (workerprocess.CommandResult, error)
}

// ExecuteInput contains the neutral facts needed for one adapter invocation.
type ExecuteInput struct {
	Provider Identity
	Command  CommandContext
	Decoder  DecoderContext
}

// ExecuteResult contains neutral adapter outputs from one subprocess attempt.
// Event publication, retry execution, and transport formatting remain owned by
// callers.
type ExecuteResult struct {
	Outcome      CommandOutcome
	Capabilities Capabilities
	Response     interfaces.InferenceResponse
	Drafts       []responseevents.Draft
	Diagnostics  []Diagnostic
	Failure      *FailureFacts
	Command      workerprocess.CommandResult
	CommandError error
	DecodeError  error
	FlushError   error
	ParseError   error
}

// Execute resolves one adapter and runs its complete neutral lifecycle. Once a
// decoder exists it is always flushed before the final result is parsed and
// failure facts are classified.
func Execute(ctx context.Context, registry *Registry, runner StreamingCommandRunner, input ExecuteInput) (ExecuteResult, error) {
	selected, err := registry.Lookup(input.Provider)
	if err != nil {
		return ExecuteResult{}, err
	}
	if runner == nil {
		return ExecuteResult{}, errors.New("provider adapter execution requires a command runner")
	}

	capabilityResult, err := selected.Capabilities(ctx, CapabilityContext{Request: input.Command.Request})
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("report provider adapter capabilities: %w", err)
	}
	built, err := selected.BuildCommand(ctx, input.Command)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("build provider adapter command: %w", err)
	}
	decoder, err := selected.NewDecoder(ctx, input.Decoder)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("create provider adapter decoder: %w", err)
	}

	result := ExecuteResult{Capabilities: capabilityResult.Capabilities}
	commandResult, commandErr := runner.Run(ctx, built.Request, func(observation Observation) error {
		decoded, observeErr := decoder.Observe(ctx, observation)
		result.appendDecoded(decoded)
		if observeErr != nil && result.DecodeError == nil {
			result.DecodeError = observeErr
		}
		return observeErr
	})
	result.Command, result.CommandError = commandResult, commandErr
	outcome, flushReason := commandOutcome(ctx, commandResult, commandErr)
	result.Outcome = outcome

	flushed, flushErr := decoder.Flush(ctx, FlushContext{Reason: flushReason})
	result.appendDecoded(flushed)
	result.FlushError = flushErr

	final, parseErr := selected.ParseFinal(ctx, FinalParseContext{
		CommandResult: commandResult, CommandError: commandErr, FlushReason: flushReason,
		RunID: input.Decoder.RunID, DispatchID: input.Decoder.DispatchID,
	})
	result.Response, result.ParseError = final.Response, parseErr
	result.Drafts = append(result.Drafts, final.Drafts...)
	classified := selected.ClassifyFailure(ctx, FailureContext{
		CommandResult: commandResult, CommandError: commandErr, DecodeError: result.DecodeError,
		FlushError: flushErr, ParseError: parseErr, FlushReason: flushReason,
	})
	result.Failure = classified.Failure
	return result, errors.Join(result.DecodeError, flushErr, parseErr)
}

func (r *ExecuteResult) appendDecoded(decoded DecodeResult) {
	r.Drafts = append(r.Drafts, decoded.Drafts...)
	r.Diagnostics = append(r.Diagnostics, decoded.Diagnostics...)
}

func commandOutcome(ctx context.Context, result workerprocess.CommandResult, commandErr error) (CommandOutcome, FlushReason) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) {
		return CommandOutcomeCanceled, FlushReasonCanceled
	}
	if commandErr != nil || result.ExitCode != 0 {
		return CommandOutcomeProcessFailed, FlushReasonTerminated
	}
	return CommandOutcomeCompleted, FlushReasonCompleted
}
