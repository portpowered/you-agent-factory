package adapter

import (
	"context"
	"errors"
	"fmt"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
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
	Publish  func(DecodeResult)
	// ObserveDraft receives correlated drafts as decoding progresses. Session
	// publication metadata remains owned by the caller.
	ObserveDraft func(responseevents.Draft)
}

// ExecuteResult contains neutral adapter outputs from one subprocess attempt.
// Event publication, retry execution, and transport formatting remain owned by
// callers.
type ExecuteResult struct {
	Outcome           CommandOutcome
	Capabilities      Capabilities
	Response          workerexecution.InferenceResponse
	Drafts            []responseevents.Draft
	Diagnostics       []Diagnostic
	Failure           *FailureFacts
	Command           workerprocess.CommandResult
	CommandError      error
	DecodeError       error
	FlushError        error
	ParseError        error
	FlushReason       FlushReason
	CapabilityUpdates []CapabilityUpdate
	Request           workerprocess.CommandRequest
}

// CapabilityUpdate records an invocation-specific fidelity change selected
// after execution began.
type CapabilityUpdate struct {
	Provider     Identity
	Capabilities Capabilities
	Diagnostic   Diagnostic
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

	result, attemptErr := executeAttempt(ctx, selected, runner, input)
	planner, supportsFallback := selected.(FallbackPlanner)
	if !supportsFallback {
		return result, attemptErr
	}
	plan, fallback, err := planner.PlanFallback(ctx, fallbackContext(result))
	if err != nil {
		return result, fmt.Errorf("plan provider adapter fallback: %w", err)
	}
	if !fallback {
		return result, attemptErr
	}
	if isNilAdapter(plan.Adapter) {
		return result, errors.New("provider adapter fallback returned a nil adapter")
	}

	fallbackResult, fallbackErr := executeAttempt(ctx, plan.Adapter, runner, input)
	fallbackResult.Diagnostics = append(fallbackResult.Diagnostics, plan.Diagnostic)
	fallbackResult.CapabilityUpdates = append(fallbackResult.CapabilityUpdates, CapabilityUpdate{
		Provider: selected.Identity(), Capabilities: fallbackResult.Capabilities, Diagnostic: plan.Diagnostic,
	})
	return fallbackResult, fallbackErr
}

func executeAttempt(ctx context.Context, selected Adapter, runner StreamingCommandRunner, input ExecuteInput) (ExecuteResult, error) {
	capabilityResult, err := selected.Capabilities(ctx, CapabilityContext{Request: input.Command.Request})
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("report provider adapter capabilities: %w", err)
	}
	built, err := selected.BuildCommand(ctx, input.Command)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("build provider adapter command: %w", err)
	}
	if built.Cleanup != nil {
		defer built.Cleanup()
	}
	decoder, err := selected.NewDecoder(ctx, input.Decoder)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("create provider adapter decoder: %w", err)
	}

	result := ExecuteResult{Capabilities: capabilityResult.Capabilities, Request: built.Request}
	commandResult, commandErr := runner.Run(ctx, built.Request, func(observation Observation) error {
		decoded, observeErr := decoder.Observe(ctx, observation)
		correlateDrafts(decoded.Drafts, input.Decoder)
		result.appendDecoded(decoded, input.ObserveDraft)
		publishDecoded(input.Publish, decoded)
		if observeErr != nil && result.DecodeError == nil {
			result.DecodeError = observeErr
		}
		return observeErr
	})
	result.Command, result.CommandError = commandResult, commandErr
	outcome, flushReason := commandOutcome(ctx, commandResult, commandErr)
	result.Outcome, result.FlushReason = outcome, flushReason

	flushed, flushErr := decoder.Flush(ctx, FlushContext{Reason: flushReason})
	correlateDrafts(flushed.Drafts, input.Decoder)
	result.appendDecoded(flushed, input.ObserveDraft)
	publishDecoded(input.Publish, flushed)
	result.FlushError = flushErr

	final, parseErr := selected.ParseFinal(ctx, FinalParseContext{
		CommandResult: commandResult, CommandError: commandErr, FlushReason: flushReason,
		RunID: input.Decoder.RunID, DispatchID: input.Decoder.DispatchID,
	})
	result.Response, result.ParseError = final.Response, parseErr
	correlateDrafts(final.Drafts, input.Decoder)
	result.Drafts = append(result.Drafts, final.Drafts...)
	observeDrafts(final.Drafts, input.ObserveDraft)
	publishDecoded(input.Publish, DecodeResult{Drafts: final.Drafts})
	classified := selected.ClassifyFailure(ctx, FailureContext{
		CommandResult: commandResult, CommandError: commandErr, DecodeError: result.DecodeError,
		FlushError: flushErr, ParseError: parseErr, FlushReason: flushReason,
	})
	result.Failure = classified.Failure
	return result, errors.Join(result.DecodeError, flushErr, parseErr)
}

func publishDecoded(publish func(DecodeResult), decoded DecodeResult) {
	if publish == nil || (len(decoded.Drafts) == 0 && len(decoded.Diagnostics) == 0) {
		return
	}
	publish(decoded)
}

func fallbackContext(result ExecuteResult) FallbackContext {
	return FallbackContext{
		CommandResult: result.Command, CommandError: result.CommandError,
		DecodeError: result.DecodeError, FlushError: result.FlushError, ParseError: result.ParseError,
		FlushReason: result.FlushReason, Drafts: result.Drafts, Diagnostics: result.Diagnostics,
	}
}

func correlateDrafts(drafts []responseevents.Draft, correlation DecoderContext) {
	for index := range drafts {
		if drafts[index].RunID == "" {
			drafts[index].RunID = correlation.RunID
		}
		if drafts[index].DispatchID == "" {
			drafts[index].DispatchID = correlation.DispatchID
		}
	}
}

func (r *ExecuteResult) appendDecoded(decoded DecodeResult, observe func(responseevents.Draft)) {
	r.Drafts = append(r.Drafts, decoded.Drafts...)
	r.Diagnostics = append(r.Diagnostics, decoded.Diagnostics...)
	observeDrafts(decoded.Drafts, observe)
}

func observeDrafts(drafts []responseevents.Draft, observe func(responseevents.Draft)) {
	if observe == nil {
		return
	}
	for _, draft := range drafts {
		observe(draft)
	}
}

func commandOutcome(ctx context.Context, result workerprocess.CommandResult, commandErr error) (CommandOutcome, FlushReason) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) || result.ExitCode == 124 {
		return CommandOutcomeCanceled, FlushReasonCanceled
	}
	if commandErr != nil || result.ExitCode != 0 {
		return CommandOutcomeProcessFailed, FlushReasonTerminated
	}
	return CommandOutcomeCompleted, FlushReasonCompleted
}
