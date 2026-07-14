package agy

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

type terminalError struct {
	failureType interfaces.WorkFailureType
	message     string
	retryable   bool
}

func (e *terminalError) Error() string { return e.message }

func processTerminalError(result workerprocess.CommandResult, commandErr error) *terminalError {
	if errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) {
		return &terminalError{
			failureType: interfaces.WorkFailureTypeTimeout,
			message:     "Agy request was canceled or timed out.",
			retryable:   true,
		}
	}
	if result.ExitCode == 124 {
		return &terminalError{
			failureType: interfaces.WorkFailureTypeTimeout,
			message:     "Agy request timed out.",
			retryable:   true,
		}
	}
	return &terminalError{
		failureType: interfaces.WorkFailureTypeUnknown,
		message:     fmt.Sprintf("Agy execution exited with code %d.", result.ExitCode),
	}
}

func classifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.FlushReason == adapter.FlushReasonCanceled {
		return failureResult(&terminalError{
			failureType: interfaces.WorkFailureTypeTimeout,
			message:     "Agy request was canceled or timed out.",
			retryable:   true,
		})
	}
	for _, candidate := range []error{input.ParseError, input.DecodeError, input.FlushError, input.CommandError} {
		if candidate == nil {
			continue
		}
		var terminal *terminalError
		if errors.As(candidate, &terminal) {
			return failureResult(terminal)
		}
		return failureResult(&terminalError{
			failureType: interfaces.WorkFailureTypeUnknown,
			message:     "Agy output could not be processed.",
		})
	}
	if input.CommandResult.ExitCode != 0 {
		return failureResult(processTerminalError(input.CommandResult, nil))
	}
	return adapter.FailureResult{}
}

func failureResult(terminal *terminalError) adapter.FailureResult {
	family := interfaces.WorkFailureFamilyTerminal
	if terminal.retryable {
		family = interfaces.WorkFailureFamilyRetryable
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: family, Type: terminal.failureType, Message: terminal.message,
		Retry: adapter.RetryGuidance{Retryable: terminal.retryable},
	}}
}
