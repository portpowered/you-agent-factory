package internal

import (
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

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
		WalkDirectory:     config.WalkDirectory,
		WorkRequestIDs:    config.WorkRequestIDs,
		Submitter:         config.Submitter,
		DebounceWindow:    config.DebounceWindow,
	})
}
