// Package wire is the Worker Sessions composition boundary.
//
// Wire performs construction only, returns the singular
// workersessions.Service root interface, and starts no lifecycle
// components. It composes the implementation through direct single
// injection of one workers.WorkstationExecutionService, with no dependency
// bag, service locator, or alternate construction path. There is no
// production consumer of workersessions.Service yet (Runtime cutover is
// W4), so canonical pkg/wire composition is not forced for this
// constructor; callers that need the service call NewService directly.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	service "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewService constructs the Worker Sessions root from the one directly
// injected workers.WorkstationExecutionService that Start hands attempts to.
// logger is the direct, required operation-logging abstraction; callers with
// no operation logging pass logging.NoopLogger{}.
func NewService(
	execution workers.WorkstationExecutionService,
	logger logging.Logger,
) (workersessions.Service, error) {
	return service.New(execution, logger)
}
