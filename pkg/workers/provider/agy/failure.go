package agy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

// ErrMissingExecutable reports that the configured Agy binary could not be resolved.
var ErrMissingExecutable = errors.New("agy: executable not found")

type terminalError struct {
	failureType interfaces.WorkFailureType
	message     string
	retryable   bool
}

func (e *terminalError) Error() string { return e.message }

func timeoutTerminalError() *terminalError {
	return &terminalError{
		failureType: interfaces.WorkFailureTypeTimeout,
		message:     "Agy request timed out.",
		retryable:   true,
	}
}

func processTerminalError(result workerprocess.CommandResult, commandErr error) *terminalError {
	if errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) {
		return &terminalError{
			failureType: interfaces.WorkFailureTypeTimeout,
			message:     "Agy request was canceled or timed out.",
			retryable:   true,
		}
	}
	if errors.Is(commandErr, agypty.ErrSessionTimedOut) || result.ExitCode == 124 {
		return timeoutTerminalError()
	}
	if setup := classifySetupCommandError(commandErr); setup != nil {
		return setup
	}
	if failureType := classifyOutputFailure(result); failureType != interfaces.WorkFailureTypeUnknown {
		return terminalErrorForType(failureType)
	}
	if errors.Is(commandErr, agypty.ErrNonzeroExit) || result.ExitCode != 0 {
		return &terminalError{
			failureType: interfaces.WorkFailureTypeUnknown,
			message:     fmt.Sprintf("Agy execution exited with code %d.", result.ExitCode),
		}
	}
	return &terminalError{
		failureType: interfaces.WorkFailureTypeUnknown,
		message:     "Agy output could not be processed.",
	}
}

func classifySetupCommandError(err error) *terminalError {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrMissingExecutable), errors.Is(err, exec.ErrNotFound):
		return &terminalError{
			failureType: interfaces.WorkFailureTypeMissingExecutable,
			message:     "Agy executable could not be found.",
		}
	case errors.Is(err, agypty.ErrPTYAllocationFailed):
		return &terminalError{
			failureType: interfaces.WorkFailureTypeMisconfigured,
			message:     "Agy PTY allocation failed.",
		}
	case errors.Is(err, agypty.ErrUnsupportedPlatform):
		return &terminalError{
			failureType: interfaces.WorkFailureTypeMisconfigured,
			message:     "Agy PTY allocation is not supported on this platform.",
		}
	default:
		return nil
	}
}

func classifyOutputFailure(result workerprocess.CommandResult) interfaces.WorkFailureType {
	normalized := strings.ToLower(strings.TrimSpace(string(result.Stdout)))
	if normalized == "" {
		return interfaces.WorkFailureTypeUnknown
	}
	if containsAuthSignal(normalized) {
		return interfaces.WorkFailureTypeAuthFailure
	}
	return interfaces.WorkFailureTypeUnknown
}

func containsAuthSignal(normalized string) bool {
	for _, signal := range []string{
		"api key", "authentication", "unauthorized", "forbidden",
		"login required", "not authenticated",
	} {
		if strings.Contains(normalized, signal) {
			return true
		}
	}
	return false
}

func terminalErrorForType(failureType interfaces.WorkFailureType) *terminalError {
	switch failureType {
	case interfaces.WorkFailureTypeAuthFailure:
		return &terminalError{
			failureType: interfaces.WorkFailureTypeAuthFailure,
			message:     "Agy authentication failed.",
		}
	case interfaces.WorkFailureTypeTimeout:
		return timeoutTerminalError()
	default:
		return &terminalError{
			failureType: interfaces.WorkFailureTypeUnknown,
			message:     "Agy reported an execution failure.",
		}
	}
}

func classifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.FlushReason == adapter.FlushReasonCanceled {
		return failureResult(timeoutTerminalError())
	}
	for _, candidate := range []error{input.ParseError, input.DecodeError, input.FlushError} {
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
	if setup := classifySetupCommandError(input.CommandError); setup != nil {
		return failureResult(setup)
	}
	if input.CommandResult.ExitCode != 0 || input.CommandError != nil {
		return failureResult(processTerminalError(input.CommandResult, input.CommandError))
	}
	return adapter.FailureResult{}
}

// ClassifyOrchestrationError maps pre-adapter orchestration failures such as
// command build errors into adapter failure facts.
func ClassifyOrchestrationError(err error) adapter.FailureResult {
	if setup := classifySetupCommandError(err); setup != nil {
		return failureResult(setup)
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
