// Package factory_visualization owns the event-driven projection lifecycle for
// presenting one live Factory.
package factory_visualization

import (
	"context"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

// View is the transport-independent presentation input emitted after the
// canonical Factory event projection changes.
type View = liveviewprojection.View

// RuntimeObservation is the sanitized Runtime fact shape carried on emitted Views.
type RuntimeObservation = liveviewprojection.RuntimeObservation

// Sink presents one projected Factory view at an external boundary. The
// implementation lives with the private live-view owner; this alias preserves
// the existing process-edge interface without publishing another root-owned
// interface declaration.
type Sink = liveviewprojection.Sink

// SinkFunc adapts a presentation function to Sink.
type SinkFunc func(View)

func (f SinkFunc) PresentFactoryView(view View) { f(view) }

// RootObserver receives the published root composed for one runtime opening.
// Process-edge functional proofs use this seam to exercise Activate and Join
// without importing owner-private packages.
type RootObserver func(Root)

// Clock supplies observation timestamps without hiding process-global time.
type Clock = liveviewprojection.Clock

// Source supplies retained-then-live canonical events and the corresponding
// runtime snapshot. Implementations may adapt a currently selected Factory
// Session, but the visualization service never reaches into its registry.
type Source = liveviewprojection.Source

// ErrorReporter receives non-fatal projection or presentation-read failures.
type ErrorReporter = liveviewprojection.ErrorReporter

// RuntimeOpening allocates an inert session-scoped lifecycle handle over fixed
// injected owners. Requests carry only session and presentation resource identity.
type RuntimeOpening func(context.Context, string, RuntimeSinkID) (Service, error)
