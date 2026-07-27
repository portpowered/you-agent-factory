package service

import (
	"fmt"
	"path/filepath"
	"sync"

	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

func observationIdentity(watchRoot, path string) (filesystemwatchers.ObservationIdentity, error) {
	rel, err := filepath.Rel(watchRoot, path)
	if err != nil {
		return "", fmt.Errorf("derive observation identity for %s: %w", path, err)
	}
	return filesystemwatchers.ObservationIdentity(filepath.ToSlash(rel)), nil
}

type memoryHandledIdentities struct {
	mu       sync.Mutex
	recorded map[filesystemwatchers.ObservationIdentity]struct{}
}

func newMemoryHandledIdentities() *memoryHandledIdentities {
	return &memoryHandledIdentities{
		recorded: make(map[filesystemwatchers.ObservationIdentity]struct{}),
	}
}

func (s *memoryHandledIdentities) Contains(identity filesystemwatchers.ObservationIdentity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.recorded[identity]
	return ok
}

func (s *memoryHandledIdentities) Record(identity filesystemwatchers.ObservationIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded[identity] = struct{}{}
	return nil
}
