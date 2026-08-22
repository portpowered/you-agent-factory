package service

import (
	"errors"
	"io/fs"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// trackedTemporaryFiles gives one Execute call ownership of only the paths it
// creates. The underlying platform effect remains stateless and is never asked
// to perform an unscoped cleanup.
type trackedTemporaryFiles struct {
	system workers.TemporaryFileSystem

	mu    sync.Mutex
	paths []string
}

func newTrackedTemporaryFiles(system workers.TemporaryFileSystem) *trackedTemporaryFiles {
	if system == nil {
		return nil
	}
	return &trackedTemporaryFiles{system: system}
}

func (files *trackedTemporaryFiles) CreateTemp(
	directory string,
	pattern string,
) (workers.TemporaryFile, error) {
	if files == nil || files.system == nil {
		return nil, errors.New("temporary file system is unavailable")
	}
	file, err := files.system.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, errors.New("temporary file system returned a nil file")
	}
	if path := strings.TrimSpace(file.Name()); path != "" {
		files.mu.Lock()
		files.paths = append(files.paths, path)
		files.mu.Unlock()
	}
	return file, nil
}

func (files *trackedTemporaryFiles) Remove(path string) error {
	if files == nil || files.system == nil {
		return errors.New("temporary file system is unavailable")
	}
	err := files.system.Remove(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		files.forget(path)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (files *trackedTemporaryFiles) Cleanup() error {
	if files == nil || files.system == nil {
		return nil
	}
	files.mu.Lock()
	paths := append([]string(nil), files.paths...)
	files.paths = nil
	files.mu.Unlock()

	var cleanupErr error
	for index := len(paths) - 1; index >= 0; index-- {
		if err := files.system.Remove(paths[index]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func (files *trackedTemporaryFiles) forget(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	files.mu.Lock()
	defer files.mu.Unlock()
	for index := len(files.paths) - 1; index >= 0; index-- {
		if files.paths[index] != path {
			continue
		}
		files.paths = append(files.paths[:index], files.paths[index+1:]...)
		return
	}
}
