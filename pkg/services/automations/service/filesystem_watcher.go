package service

import (
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

var _ automations.FilesystemWatcherFactory = (*Service)(nil)

func (s *Service) NewFilesystemWatcher(config automations.FilesystemWatcherConfig) automations.FilesystemWatcher {
	if s == nil || s.filesystemWatchers == nil {
		return nil
	}
	return s.filesystemWatchers.NewWatcher(filesystemwatchers.Config{
		Dir:               config.Dir,
		Logger:            config.Logger,
		KnownWorkTypes:    config.KnownWorkTypes,
		ValidStatesByType: config.ValidStatesByType,
		Files:             config.Files,
		WalkDirectory:     filesystemwatchers.DirectoryWalker(config.WalkDirectory),
		WorkRequestIDs:    config.WorkRequestIDs,
		Submitter:         filesystemwatchers.WorkRequestSubmitter(config.Submitter),
	})
}
