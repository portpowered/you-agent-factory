package script_pollers

import (
	"fmt"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

const submitOperation = "script_poller.submit"

// SubmitFailedError reports that Work admission rejected or failed after a valid
// poller stdout payload was parsed. The underlying Work error is wrapped without
// reinterpreting admission policy.
func SubmitFailedError(err error) error {
	if err == nil {
		return nil
	}
	return &automations.Error{
		Op:   submitOperation,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("script poller submit failed: %w", err),
	}
}
