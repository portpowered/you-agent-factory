// Package wire is the Worker Sessions composition boundary.
//
// Wire performs construction only, returns the singular
// workersessions.Service root interface, and starts no lifecycle
// components. It composes the implementation through direct single
// injection of one workers.WorkstationPoolBoundary and one
// EventsAppender, with no dependency bag, service locator, or alternate
// construction path. Factory Runtime is the production consumer (W4
// dispatch cutover), composed through pkg/services/factory_runtime/internal
// and pkg/wire.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	internalservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// EventsAppender is the narrow Events dependency the constructed Service's
// Start commits its before-handoff opening record through. Any
// events.Service value satisfies this interface structurally.
type EventsAppender = internalservice.EventsAppender

// NewService constructs the Worker Sessions root from the one directly
// injected workers.WorkstationPoolBoundary that Start publishes attempts
// through and the one directly injected EventsAppender Start's before-handoff
// publication barrier commits through. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}.
func NewService(
	boundary workers.WorkstationPoolBoundary,
	eventsAppender EventsAppender,
	logger logging.Logger,
) (workersessions.Service, error) {
	return internalservice.New(boundary, eventsAppender, logger)
}
