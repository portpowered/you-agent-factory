package factory_visualization

import (
	"context"
	"errors"
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
	// Observe returns one detached retained-then-live Factory view projection
	// through Visualization-owned plain contracts.
	Observe(context.Context, ObserveRequest) (ObserveResult, error)
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

// ObserveMode selects how live projection is obtained through the root.
type ObserveMode string

const (
	// ObserveModeRetainedThenLive projects retained history then live
	// observation facts into one detached Visualization view.
	ObserveModeRetainedThenLive ObserveMode = "RETAINED_THEN_LIVE"
)

// ObserveReconnectCursor is the Visualization-owned reconnect observe input.
// Peers supply AfterEventID and/or AfterSequence; both empty is invalid.
type ObserveReconnectCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// ObserveRequest carries explicit live-projection parameters. A zero-value
// request is rejected as invalid input.
type ObserveRequest struct {
	Mode      ObserveMode
	Reconnect *ObserveReconnectCursor
}

// ProjectedView is a Visualization-owned detached live view. Engine-state and
// observation facts are plain values so peers do not import Recordings ledger
// storage or Runtime Petri/JavaScript internals.
type ProjectedView struct {
	TickCount          int
	RetainedEventCount int
	ObservedAt         time.Time
}

// ObserveResult is the published outcome of a successful Observe call.
type ObserveResult struct {
	View ProjectedView
}

// ProjectionErrorKind distinguishes typed Visualization live-projection outcomes.
type ProjectionErrorKind string

const (
	ProjectionErrorInvalidInput         ProjectionErrorKind = "INVALID_INPUT"
	ProjectionErrorSnapshotUnavailable  ProjectionErrorKind = "SNAPSHOT_UNAVAILABLE"
	ProjectionErrorReconstructionFailed ProjectionErrorKind = "RECONSTRUCTION_FAILED"
)

// ProjectionError is a typed Visualization live-projection failure peers can branch on.
type ProjectionError struct {
	Kind    ProjectionErrorKind
	Message string
	Cause   error
}

func (e *ProjectionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *ProjectionError) Unwrap() error {
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

// Observe implements Root by validating observe inputs, reading a detached
// engine-state snapshot, reconstructing the retained projection, and returning
// Visualization-owned view facts without exposing Recordings or Runtime types.
func (s *Service) Observe(ctx context.Context, req ObserveRequest) (ObserveResult, error) {
	if s == nil {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: service is required",
		}
	}
	if ctx == nil {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: context is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return ObserveResult{}, err
	}
	if req.Mode == "" {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: required request parameters are missing",
		}
	}
	if req.Mode != ObserveModeRetainedThenLive {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: fmt.Sprintf("observe Factory visualization: observe mode %q is not supported", req.Mode),
		}
	}
	if err := validateObserveReconnect(req.Reconnect); err != nil {
		return ObserveResult{}, err
	}

	snapshot, err := s.source.GetEngineStateSnapshot(ctx)
	if err != nil {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorSnapshotUnavailable,
			Message: "observe Factory visualization: snapshot is unavailable",
			Cause:   err,
		}
	}
	if snapshot == nil {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorSnapshotUnavailable,
			Message: "observe Factory visualization: snapshot is unavailable",
		}
	}

	s.mu.Lock()
	events := append([]factorydefinitions.FactoryEvent(nil), s.events...)
	s.mu.Unlock()

	if req.Reconnect != nil {
		cursor := factorydefinitions.FactoryEventReconnectCursor{
			AfterEventID:  req.Reconnect.AfterEventID,
			AfterSequence: req.Reconnect.AfterSequence,
		}
		if err := s.projections.ValidateReconnectReplay(
			events,
			cursor,
			factorydefinitions.FactoryEventReconnectScope{},
		); err != nil {
			return ObserveResult{}, &ProjectionError{
				Kind:    ProjectionErrorInvalidInput,
				Message: "observe Factory visualization: reconnect observe input is invalid",
				Cause:   err,
			}
		}
	}

	if _, err := s.projections.ReconstructFactoryWorldState(events, snapshot.TickCount); err != nil {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorReconstructionFailed,
			Message: "observe Factory visualization: projection reconstruction failed",
			Cause:   err,
		}
	}

	return ObserveResult{
		View: ProjectedView{
			TickCount:          snapshot.TickCount,
			RetainedEventCount: len(events),
			ObservedAt:         s.clock.Now(),
		},
	}, nil
}

func validateObserveReconnect(reconnect *ObserveReconnectCursor) error {
	if reconnect == nil {
		return nil
	}
	if reconnect.AfterEventID == "" && reconnect.AfterSequence == nil {
		return &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: reconnect cursor is empty",
		}
	}
	if reconnect.AfterSequence != nil && *reconnect.AfterSequence < 0 {
		return &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: reconnect after_sequence is invalid",
		}
	}
	return nil
}
