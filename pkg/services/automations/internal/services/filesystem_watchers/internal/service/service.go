package service

import (
	"github.com/jonboulle/clockwork"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

type service struct {
	clock clockwork.Clock
}

var _ filesystemwatchers.Service = (*service)(nil)

// New constructs an inert filesystem watcher service.
func New(clock clockwork.Clock) filesystemwatchers.Service {
	return &service{clock: clock}
}

func (s *service) NewWatcher(config filesystemwatchers.Config) automations.FilesystemWatcher {
	return newWatcherWithClock(config, s.clock)
}
