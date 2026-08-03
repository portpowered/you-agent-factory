package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// provideEventsMaxRetainedRecordsPerTopic projects the process-edge Events
// retention override so provideEventsService stays a focused construction
// call. Zero (the default when no functional test overrides it) keeps
// eventswire.NewServiceWithRetention on the same production default
// NewService uses.
func provideEventsMaxRetainedRecordsPerTopic(edges serviceedges.Edges) int {
	return edges.EventsMaxRetainedRecordsPerTopic
}

// provideEventsService constructs the singular canonical events.Service
// instance through its focused wire provider. It is the one construction
// path to a Service value in production code: no alternate constructor,
// dependency bag, or secondary injector exists. Canonical application
// construction (provideApplicationProcessLifecycle below) folds this exact
// instance's Close into the composed process shutdown path.
func provideEventsService(logger logging.Logger, maxRetainedRecordsPerTopic int) (events.Service, error) {
	return eventswire.NewServiceWithRetention(maxRetainedRecordsPerTopic, logger)
}

// eventsLifecycle is the exact Close(context.Context) error shutdown role
// this package needs from the constructed Events service. It is asserted
// structurally rather than through a published events.Lifecycle interface:
// pkg/services/events keeps exactly one published root interface (Service),
// matching the docs/internal/standards service-root-interface convention, so
// this narrow capability is declared locally instead of widening it.
type eventsLifecycle interface {
	Close(context.Context) error
}

// processLifecycleAggregate composes every ProcessLifecycle-participating
// service's own Close into the single ProcessLifecycle slot
// initializerapplication.Process accepts (see
// pkg/initializer/application/process.go). Each participant's Close is
// always attempted; the first non-nil error is returned once every
// participant has been given the chance to shut down.
type processLifecycleAggregate struct {
	providers providers.Lifecycle
	events    eventsLifecycle
}

func (a processLifecycleAggregate) Close(ctx context.Context) error {
	var first error
	if err := a.providers.Close(ctx); err != nil {
		first = err
	}
	if err := a.events.Close(ctx); err != nil && first == nil {
		first = err
	}
	return first
}
