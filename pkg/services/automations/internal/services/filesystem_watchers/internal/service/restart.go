package service

import (
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

func (s *service) NewWatcherWithResume(
	req filesystemwatchers.RestartRequest,
) (automations.FilesystemWatcher, filesystemwatchers.WatcherFacts, error) {
	facts, err := s.ResumeWatcherFacts(req.Identity, req.Authoritative, req.Resume)
	if err != nil {
		return nil, filesystemwatchers.WatcherFacts{}, err
	}
	handled, err := s.newHandledIdentities(facts, req.Persist)
	if err != nil {
		return nil, filesystemwatchers.WatcherFacts{}, err
	}
	return newWatcherWithClockAndHandled(req.Config, s.clock, handled), facts, nil
}
