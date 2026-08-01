package contracts

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

// ActivationEventSource supplies the retained-then-live event stream and the
// sanitized engine facts consumed by the private activation lifecycle owner.
type ActivationEventSource interface {
	SubscribeFactoryEvents(
		context.Context,
		*factorydefinitions.FactoryEventReconnectCursor,
		factorydefinitions.FactoryEventReconnectScope,
	) (*factorydefinitions.FactoryEventStream, error)
	GetEngineObservation(context.Context) (*activationlifecycle.EngineObservation, error)
}

// ActivationClock supplies observation timestamps to the private activation
// lifecycle owner.
type ActivationClock interface {
	Now() time.Time
}

// ActivationViewSink receives the activation lifecycle owner's projected view.
type ActivationViewSink interface {
	PresentFactoryView(activationlifecycle.View)
}

// LiveViewSource supplies the retained-then-live event stream and sanitized
// runtime facts consumed by the private live projection owner.
type LiveViewSource interface {
	SubscribeFactoryEvents(
		context.Context,
		*factorydefinitions.FactoryEventReconnectCursor,
		factorydefinitions.FactoryEventReconnectScope,
	) (*factorydefinitions.FactoryEventStream, error)
	GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error)
}

// LiveViewClock supplies observation timestamps to the private live projection
// owner.
type LiveViewClock interface {
	Now() time.Time
}

// LiveViewSink receives the live projection owner's projected view.
type LiveViewSink interface {
	PresentFactoryView(liveviewprojection.View)
}
