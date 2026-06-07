package inboxgitkeep

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func EnsureInputInboxGitkeep(targetDir, relativePath string) error {
	path := filepath.Join(targetDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s parent directory: %w", relativePath, err)
	}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return fmt.Errorf("%s exists as a directory", relativePath)
		}
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("stat %s: %w", relativePath, err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("create %s: %w", relativePath, err)
	}
	return nil
}
