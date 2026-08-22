// Package wire constructs the Automations script-poller subservice.
package wire

import (
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollersservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/internal/service"
)

// CursorPersistenceFileSystem is the filesystem effect accepted by the
// script-poller composition boundary.
type CursorPersistenceFileSystem = scriptpollersservice.CursorPersistenceFileSystem

// NewService constructs an inert script-poller service with injected runtime
// dependencies. Construction never invokes the supplied functions.
func NewService(dependencies scriptpollers.Dependencies) scriptpollers.Service {
	return scriptpollersservice.New(dependencies)
}

// NewDurableCursorRecorder constructs the Automations-owned durable cursor
// implementation behind the script-poller wire boundary.
func NewDurableCursorRecorder(
	baseDir string,
	files CursorPersistenceFileSystem,
) (scriptpollers.CursorRecorder, error) {
	return scriptpollersservice.NewDurableCursorRecorder(baseDir, files)
}
