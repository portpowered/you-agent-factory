package providers

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidControlRequest reports that a control-attempt request has an
// empty attempt id or an unrecognized control action. Empty provider identity
// fails with ErrInvalidID for consistency with the rest of the root contract.
var ErrInvalidControlRequest = errors.New("provider control request is invalid")

// ControlAction is the closed Providers-owned attempt-control action
// vocabulary. Peers branch on these typed values instead of provider-specific
// control strings.
type ControlAction string

const (
	ControlActionPause     ControlAction = "pause"
	ControlActionCancel    ControlAction = "cancel"
	ControlActionTerminate ControlAction = "terminate"
)

// Validate checks that action is one of the closed, non-zero control-action
// values.
func (action ControlAction) Validate() error {
	switch action {
	case ControlActionPause, ControlActionCancel, ControlActionTerminate:
		return nil
	default:
		return fmt.Errorf("%w: unsupported control action %q", ErrInvalidControlRequest, string(action))
	}
}

// ControlOutcome is the closed Providers-owned attempt-control outcome
// vocabulary. Unsupported is a successful capability result distinct from a
// request-validation error or a genuine operation failure.
type ControlOutcome string

const (
	ControlOutcomeCompleted   ControlOutcome = "completed"
	ControlOutcomeUnsupported ControlOutcome = "unsupported"
)

// ControlAttemptRequest identifies one Providers-owned provider attempt and
// the requested pause, cancel, or terminate action.
type ControlAttemptRequest struct {
	Provider  ID
	AttemptID string
	Action    ControlAction
}

// Validate checks that Provider is non-empty, AttemptID is non-empty after
// trimming, and Action is one of the closed control-action values.
func (request ControlAttemptRequest) Validate() error {
	if err := request.Provider.Validate(); err != nil {
		return fmt.Errorf("%w", err)
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return fmt.Errorf("%w: empty attempt id", ErrInvalidControlRequest)
	}
	return request.Action.Validate()
}

// ControlAttemptResult echoes the requested provider, attempt, and action
// alongside the closed completed/unsupported outcome. Every field is a plain
// value, so a result is always detached and safe to hold or compare directly.
type ControlAttemptResult struct {
	Provider  ID
	AttemptID string
	Action    ControlAction
	Outcome   ControlOutcome
}
