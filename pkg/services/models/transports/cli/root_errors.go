package cli

import (
	"errors"
	"fmt"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

func mapModelsRootError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, modelinference.ErrNotFound):
		return fmt.Errorf("%w: %s", ErrModelNotFound, err.Error())
	case errors.Is(err, modelinference.ErrMissing),
		errors.Is(err, modelinference.ErrLoading),
		errors.Is(err, modelinference.ErrFailed),
		errors.Is(err, modelinference.ErrUnsupported),
		errors.Is(err, modelinference.ErrNotAvailable):
		return err
	case errors.Is(err, modelinference.ErrUnsupportedOperation),
		errors.Is(err, modelinference.ErrUnsupportedResponseMode),
		errors.Is(err, modelinference.ErrUnsupportedModelOperation):
		return err
	default:
		var pullErr *modelinference.PullError
		if errors.As(err, &pullErr) {
			return fmt.Errorf(
				"managed runtime pull failed (%s readiness %s)",
				pullErr.Result.ManagedPullOutcome,
				pullErr.Result.ReadinessState,
			)
		}
		return err
	}
}
