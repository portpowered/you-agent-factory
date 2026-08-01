// Package live_view_projection owns retained-then-live subscription, cursor and
// event retention, sanitized Runtime observation consumption, Recordings query
// projection consumption, and Visualization-owned View emission behind the
// parent-private subservice boundary. Peers consume Factory Visualization root
// contracts; this package is not a peer-facing authority.
package live_view_projection

import (
	"context"
	"errors"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// RuntimeObservation carries sanitized Runtime facts on Visualization-owned Views.
type RuntimeObservation struct {
	TickCount     int
	FactoryState  string
	RuntimeStatus factorydefinitions.RuntimeStatus
	Uptime        time.Duration
}

// View is the transport-independent presentation input emitted after the
// canonical Factory event projection changes.
type View struct {
	ObservedAt         time.Time
	RetainedEventCount int
	Runtime            RuntimeObservation
	RenderData         recordings.SimpleDashboardRenderData
}

// RuntimeSnapshotFacts are sanitized Runtime observation inputs consumed by the
// private owner when projecting Views.
type RuntimeSnapshotFacts struct {
	RuntimeObservation
	ActiveThrottlePauses []factorydefinitions.ActiveThrottlePause
}

// SinkFunc adapts a presentation function to Sink.
type SinkFunc func(View)

func (f SinkFunc) PresentFactoryView(view View) { f(view) }

// ErrorReporter receives non-fatal projection or presentation-read failures.
type ErrorReporter func(error)

// ObserveMode selects how live projection is obtained through the private owner.
type ObserveMode string

const (
	// ObserveModeRetainedThenLive projects retained history then live
	// observation facts into one detached Visualization view.
	ObserveModeRetainedThenLive ObserveMode = "RETAINED_THEN_LIVE"
)

// ObserveReconnectCursor is the Visualization-owned reconnect observe input.
type ObserveReconnectCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// ObserveRequest carries explicit live-projection parameters.
type ObserveRequest struct {
	Mode      ObserveMode
	Reconnect *ObserveReconnectCursor
}

// ProjectedView is a Visualization-owned detached live view.
type ProjectedView struct {
	TickCount          int
	RetainedEventCount int
	ObservedAt         time.Time
}

// ObserveResult is the outcome of a successful Observe call.
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

// ProjectionError is a typed Visualization live-projection failure.
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

// Sentinel errors returned by the private live_view_projection owner.
var (
	ErrLiveViewProjectionAlreadyStarted = errors.New("start Factory visualization live view projection: already started")
	ErrLiveViewProjectionNotStarted     = errors.New("wait for Factory visualization live view projection: not started")
)

// Service is the singular live_view_projection subservice contract for the
// published retained-then-live subscribe, cursor retention, Runtime and
// Recordings consumption, and View emission slice of the Visualization root.
type Service interface {
	Start(context.Context) error
	Stop(context.Context) error
	Wait(context.Context) error
	Observe(context.Context, ObserveRequest) (ObserveResult, error)
	ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor
}
