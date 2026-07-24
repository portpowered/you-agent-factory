package factory_visualization

import (
	"context"
	"errors"
	"fmt"
)

// Root is the singular peer-facing Factory Visualization contract.
//
// Cross-service consumers depend on this named root for request-activated
// lifecycle, live projection, and presentation/drain slices. Collaborator
// ports and legacy presentation helpers are not additional Visualization
// authority interfaces for those published slices.
//
// Concrete Service retains Start/Stop/Wait for initializer lifecycle.Component
// compatibility; peers use the request-parameter methods below.
type Root interface {
	// Activate leaves the inert constructed state through explicit request
	// parameters and starts retained-then-live Factory event projection.
	Activate(context.Context, ActivateRequest) (ActivateResult, error)
	// Join waits for the live subscription to exit. Calling Join before
	// Activate returns a typed not-activated failure while the root remains
	// inert.
	Join(context.Context, JoinRequest) (JoinResult, error)
	// StopDrain cancels the live subscription and emits one final projected
	// view through the Visualization-owned drain path.
	StopDrain(context.Context, StopDrainRequest) (StopDrainResult, error)
}

// ActivateMode selects how visualization leaves the inert constructed state.
type ActivateMode string

const (
	// ActivateModeRetainedThenLive activates retained history projection then
	// live event observation — the existing Visualization Start vocabulary.
	ActivateModeRetainedThenLive ActivateMode = "RETAINED_THEN_LIVE"
)

// LifecycleState is the published request-activated lifecycle vocabulary.
type LifecycleState string

const (
	LifecycleStateInert   LifecycleState = "INERT"
	LifecycleStateStarted LifecycleState = "STARTED"
	LifecycleStateStopped LifecycleState = "STOPPED"
)

// LifecycleErrorKind distinguishes typed Visualization lifecycle outcomes.
type LifecycleErrorKind string

const (
	LifecycleErrorMissingParameters LifecycleErrorKind = "MISSING_PARAMETERS"
	LifecycleErrorAlreadyActivated  LifecycleErrorKind = "ALREADY_ACTIVATED"
	LifecycleErrorNotActivated      LifecycleErrorKind = "NOT_ACTIVATED"
)

// ActivateRequest carries the explicit parameters required to leave the inert
// constructed state. A zero-value request is rejected.
type ActivateRequest struct {
	Mode ActivateMode
}

// ActivateResult is the published outcome of a successful Activate call.
type ActivateResult struct {
	State LifecycleState
}

// JoinRequest carries wait/join parameters for the live subscription.
type JoinRequest struct{}

// JoinResult is the published outcome of a successful Join call.
type JoinResult struct {
	State LifecycleState
}

// StopDrainRequest carries stop-and-drain-final-view parameters.
type StopDrainRequest struct{}

// StopDrainResult is the published outcome of a successful StopDrain call.
type StopDrainResult struct {
	State LifecycleState
}

// LifecycleError is a typed Visualization lifecycle failure peers can branch on.
type LifecycleError struct {
	Kind    LifecycleErrorKind
	Message string
	Cause   error
}

func (e *LifecycleError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *LifecycleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Compile-time proof that the existing lifecycle Service remains reachable
// through the singular Root seam.
var _ Root = (*Service)(nil)

// Activate implements Root by validating explicit request parameters then
// delegating to the existing Start path.
func (s *Service) Activate(ctx context.Context, req ActivateRequest) (ActivateResult, error) {
	if req.Mode == "" {
		return ActivateResult{}, &LifecycleError{
			Kind:    LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: required request parameters are missing",
		}
	}
	if req.Mode != ActivateModeRetainedThenLive {
		return ActivateResult{}, &LifecycleError{
			Kind:    LifecycleErrorMissingParameters,
			Message: fmt.Sprintf("activate Factory visualization: activate mode %q is not supported", req.Mode),
		}
	}
	err := s.Start(ctx)
	if err == nil {
		return ActivateResult{State: LifecycleStateStarted}, nil
	}
	if errors.Is(err, errAlreadyStarted) {
		return ActivateResult{}, &LifecycleError{
			Kind:    LifecycleErrorAlreadyActivated,
			Message: "activate Factory visualization: already activated",
			Cause:   err,
		}
	}
	return ActivateResult{}, err
}

// Join implements Root by delegating to Wait and mapping not-started failures.
func (s *Service) Join(ctx context.Context, _ JoinRequest) (JoinResult, error) {
	err := s.Wait(ctx)
	if err == nil {
		return JoinResult{State: LifecycleStateStarted}, nil
	}
	if errors.Is(err, errNotStarted) {
		return JoinResult{}, &LifecycleError{
			Kind:    LifecycleErrorNotActivated,
			Message: "join Factory visualization: not activated",
			Cause:   err,
		}
	}
	return JoinResult{}, err
}

// StopDrain implements Root by delegating to Stop (cancel, join, final view).
func (s *Service) StopDrain(ctx context.Context, _ StopDrainRequest) (StopDrainResult, error) {
	if err := s.Stop(ctx); err != nil {
		return StopDrainResult{}, err
	}
	return StopDrainResult{State: LifecycleStateStopped}, nil
}
