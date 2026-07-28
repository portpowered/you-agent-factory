package pi

import (
	"context"
	"errors"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func declaredFailureFromCommandOutput(stdout []byte) (providers.ExecuteFailure, bool) {
	if failure := parseTerminalFailure(stdout); failure != nil {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: failure.Error(),
		}, true
	}
	return providers.ExecuteFailure{}, false
}

func piDeclaredFailureMessage(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return "Pi authentication failed."
	case providers.ExecuteFailureKindInvalidRequest:
		return "Pi rejected the request as invalid."
	case providers.ExecuteFailureKindThrottled:
		return "Pi is temporarily unavailable due to usage or capacity limits."
	case providers.ExecuteFailureKindTimeout:
		return TimeoutFailureMessage
	case providers.ExecuteFailureKindDependency:
		return "Pi encountered a temporary server error."
	default:
		return "Pi reported a terminal failure"
	}
}

func piInvocationFailureMessage() string {
	return "Pi invocation failed."
}

const (
	// TimeoutFailureMessage is the canonical Pi timeout outcome.
	TimeoutFailureMessage = "Pi execution timed out."
)

func exitFailureFromCommandResult(result workers.CommandResult) error {
	if declared, ok := declaredFailureFromCommandOutput(result.Stdout); ok {
		return declared
	}
	if retry := piRetryFailure(result.Stdout); retry != nil {
		return *retry
	}
	if result.ExitCode == 124 {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: TimeoutFailureMessage,
		}
	}
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: piInvocationFailureMessage(),
	}
}

func nativeCommandError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: TimeoutFailureMessage,
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindCanceled,
			Message: "Pi execution was canceled.",
		}
	}
	return err
}
