// Package wire constructs the Automations filesystem_watchers subservice.
package wire

import (
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	fswservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/internal/service"
)

// NewService constructs an inert filesystem watcher service.
func NewService() filesystemwatchers.Service {
	return fswservice.New()
}
