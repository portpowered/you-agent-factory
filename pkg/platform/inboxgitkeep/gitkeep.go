package inboxgitkeep

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// FileSystem is the exact policy-free filesystem role needed to materialize
// one inbox sentinel. Domain owners select the path and supply the adapter.
type FileSystem interface {
	MkdirAll(string, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
	WriteFile(string, []byte, fs.FileMode) error
}

// Local is the policy-free local inbox-sentinel adapter selected by Wire. The
// Factory Definitions service supplies both the relative path policy and the
// exact filesystem implementation.
type Local struct {
	fileSystem FileSystem
}

func NewLocal(fileSystem FileSystem) Local {
	return Local{fileSystem: fileSystem}
}

func (local Local) EnsureInputInboxGitkeep(targetDir, relativePath string) error {
	if local.fileSystem == nil {
		return fmt.Errorf("input inbox sentinel filesystem is required")
	}
	path := filepath.Join(targetDir, relativePath)
	if err := local.fileSystem.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s parent directory: %w", relativePath, err)
	}
	info, err := local.fileSystem.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return fmt.Errorf("%s exists as a directory", relativePath)
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("stat %s: %w", relativePath, err)
	}
	if err := local.fileSystem.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("create %s: %w", relativePath, err)
	}
	return nil
}
