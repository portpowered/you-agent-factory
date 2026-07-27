package service

import (
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

type service struct{}

var _ filesystemwatchers.Service = (*service)(nil)

// New constructs an inert filesystem watcher service.
func New() filesystemwatchers.Service {
	return &service{}
}

func (*service) NewWatcher(config filesystemwatchers.Config) filesystemwatchers.Watcher {
	return newWatcher(config)
}
