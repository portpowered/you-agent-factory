// Package activationlifecycle defines the Factory Visualization-owned activation
// lifecycle. Consumers outside Factory Visualization use the Visualization root
// service instead of this parent-private subservice contract.
package activationlifecycle

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// ActivateMode selects how visualization leaves the inert constructed state.
type ActivateMode string

const (
	// ActivateModeRetainedThenLive activates retained history projection then
	// live event observation.
	ActivateModeRetainedThenLive ActivateMode = "RETAINED_THEN_LIVE"
)

// LifecycleState is the parent-private activation lifecycle vocabulary.
type LifecycleState string

const (
	LifecycleStateInert   LifecycleState = "INERT"
	LifecycleStateStarted LifecycleState = "STARTED"
	LifecycleStateStopped LifecycleState = "STOPPED"
)

// LifecycleErrorKind distinguishes typed activation lifecycle outcomes.
type LifecycleErrorKind string

const (
	LifecycleErrorMissingParameters LifecycleErrorKind = "MISSING_PARAMETERS"
	LifecycleErrorAlreadyActivated  LifecycleErrorKind = "ALREADY_ACTIVATED"
	LifecycleErrorNotActivated      LifecycleErrorKind = "NOT_ACTIVATED"
)

// ActivateRequest carries explicit activation parameters.
type ActivateRequest struct {
	Mode ActivateMode
}

// ActivateResult is the outcome of a successful Activate call.
type ActivateResult struct {
	State LifecycleState
}

// JoinRequest carries wait/join parameters for the live subscription.
type JoinRequest struct{}

// JoinResult is the outcome of a successful Join call.
type JoinResult struct {
	State LifecycleState
}

// StopDrainRequest carries stop-and-drain parameters.
type StopDrainRequest struct{}

// StopDrainResult is the outcome of a successful StopDrain call.
type StopDrainResult struct {
	State LifecycleState
}

// LifecycleError is a typed activation lifecycle failure.
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

// EngineObservation carries Visualization-owned runtime facts required for
// retained projection without exposing Factory Runtime StateSnapshot types on
// the activation lifecycle contract.
type EngineObservation struct {
	TickCount            int
	ActiveThrottlePauses []factorydefinitions.ActiveThrottlePause
}

// EventSource supplies retained-then-live canonical events and engine
// observation facts for the selected Factory Session runtime.
type EventSource interface {
	SubscribeFactoryEvents(
		context.Context,
		*factorydefinitions.FactoryEventReconnectCursor,
		factorydefinitions.FactoryEventReconnectScope,
	) (*factorydefinitions.FactoryEventStream, error)
	GetEngineObservation(context.Context) (*EngineObservation, error)
}

// Clock supplies observation timestamps.
type Clock interface {
	Now() time.Time
}

// View is the transport-independent presentation input emitted after projection.
type View struct {
	EngineObservation EngineObservation
	RenderData        recordings.SimpleDashboardRenderData
	ObservedAt        time.Time
}

// ViewSink presents one projected Factory view.
type ViewSink interface {
	PresentFactoryView(View)
}

// ErrorReporter receives non-fatal projection or presentation-read failures.
type ErrorReporter func(error)

// Service owns request-option interpretation, session/runtime binding, and
// Start/Stop/Wait/cleanup behind the Visualization root.
type Service interface {
	Start(context.Context) error
	Stop(context.Context) error
	Wait(context.Context) error
	Activate(context.Context, ActivateRequest) (ActivateResult, error)
	Join(context.Context, JoinRequest) (JoinResult, error)
	StopDrain(context.Context, StopDrainRequest) (StopDrainResult, error)
	RetainedEvents() []factorydefinitions.FactoryEvent
	ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor
}
