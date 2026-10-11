package agy

import (
	"errors"
	"os/exec"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
)

// ErrMissingExecutable reports that the configured Agy binary could not be resolved.
var ErrMissingExecutable = errors.New("agy: executable not found")

const (
	// TimeoutFailureMessage is the canonical Agy timeout outcome.
	TimeoutFailureMessage = "Agy request timed out."
)

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
