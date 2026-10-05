package support

import (
	"path/filepath"
	"sync"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// CostsSettingsFiles injects a query-time read failure for one owned settings
// document while delegating setup, writes and other documents to the real edge.
type CostsSettingsFiles struct {
	operatorsettings.FileSystem
	mu       sync.RWMutex
	failures map[string]error
}

func NewCostsSettingsFiles(files operatorsettings.FileSystem) *CostsSettingsFiles {
	return &CostsSettingsFiles{FileSystem: files, failures: make(map[string]error)}
}

func (files *CostsSettingsFiles) SetReadFailure(path string, cause error) {
	files.mu.Lock()
	defer files.mu.Unlock()
	if cause == nil {
		delete(files.failures, filepath.Clean(path))
	} else {
		files.failures[filepath.Clean(path)] = cause
	}
}

func (files *CostsSettingsFiles) ReadFile(path string) ([]byte, error) {
	files.mu.RLock()
	cause := files.failures[filepath.Clean(path)]
	files.mu.RUnlock()
	if cause != nil {
		return nil, cause
	}
	return files.FileSystem.ReadFile(path)
}
