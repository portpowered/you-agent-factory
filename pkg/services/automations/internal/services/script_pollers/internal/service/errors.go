package service

import (
	"fmt"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

const submitOperation = "script_poller.submit"

func submitFailedError(err error) error {
	if err == nil {
		return nil
	}
	return &automations.Error{
		Op:   submitOperation,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("script poller submit failed: %w", err),
	}
}
