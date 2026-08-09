package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// provideWorkerSessionsFactory constructs the one canonical per-session
// Worker Sessions (W4 Runtime dispatch cutover) construction path over the
// already-composed events.Service and logging.Logger singletons. Factory
// Runtime consumes only the returned factoryruntime.WorkerSessionsFactory
// function value, never worker_sessions/wire directly, so composition of the
// peer worker_sessions service stays inside pkg/wire.
func provideWorkerSessionsFactory(
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
) factoryruntime.WorkerSessionsFactory {
	return func(boundary workers.WorkstationPoolBoundary, clock platformclock.Source) (workersessions.Service, error) {
		return workersessionswire.NewService(boundary, eventsService, logger, clock, providerSessions)
	}
}
