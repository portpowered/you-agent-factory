package agy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
)

// ErrMissingExecutable reports that the configured Agy binary could not be resolved.
var ErrMissingExecutable = errors.New("agy: executable not found")

const (
	// TimeoutFailureMessage is the canonical Agy timeout outcome.
	TimeoutFailureMessage = "Agy request timed out."
)

func nativePTYError(ctx context.Context, result agypty.SessionResult, err error) error {
	if declared, ok := declaredFailureFromPTY(ctx, result, err); ok {
		return declared
	}
	return err
}

func declaredFailureFromPTY(
	ctx context.Context,
	result agypty.SessionResult,
	err error,
) (providers.ExecuteFailure, bool) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}, true
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: TimeoutFailureMessage,
		}, true
	}
	if setup := classifySetupError(err); setup != nil {
		return *setup, true
	}
	if errors.Is(err, agypty.ErrSessionTimedOut) || result.ExitCode == 124 {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: TimeoutFailureMessage,
		}, true
	}
	if failureType := classifyOutputFailure(result.CleanedText); failureType != workers.WorkFailureTypeUnknown {
		return declaredFailureFromWorkFailure(failureType), true
	}
	if errors.Is(err, agypty.ErrNonzeroExit) || result.ExitCode != 0 {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: fmt.Sprintf("Agy execution exited with code %d.", result.ExitCode),
		}, true
	}
	return providers.ExecuteFailure{}, false
}

func classifySetupError(err error) *providers.ExecuteFailure {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrMissingExecutable), errors.Is(err, exec.ErrNotFound):
		return &providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "Agy executable could not be found.",
		}
	case errors.Is(err, agypty.ErrPTYAllocationFailed):
		return &providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "Agy PTY allocation failed.",
		}
	case errors.Is(err, agypty.ErrUnsupportedPlatform):
		return &providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "Agy PTY allocation is not supported on this platform.",
		}
	default:
		return nil
	}
}

func classifyOutputFailure(stdout string) workers.WorkFailureType {
	normalized := strings.ToLower(strings.TrimSpace(stdout))
	if normalized == "" {
		return workers.WorkFailureTypeUnknown
	}
	for _, signal := range []string{
		"api key", "authentication", "unauthorized", "forbidden",
		"login required", "not authenticated",
	} {
		if strings.Contains(normalized, signal) {
			return workers.WorkFailureTypeAuthFailure
		}
	}
	return workers.WorkFailureTypeUnknown
}

func declaredFailureFromWorkFailure(failureType workers.WorkFailureType) providers.ExecuteFailure {
	switch failureType {
	case workers.WorkFailureTypeAuthFailure:
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindAuthentication,
			Message: "Agy authentication failed.",
		}
	default:
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: "Agy reported an execution failure.",
		}
	}
}

func orchestrationFailure(err error) error {
	if setup := classifySetupError(err); setup != nil {
		return setup.Clone()
	}
	return execution.AttemptFailure{NativeError: err}
}
