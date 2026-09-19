//go:build !windows

package tts_clean_install

import (
	"fmt"
	"os"
	"path/filepath"
)

func rejectReparsePath(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("component %q is a symbolic link", filepath.Base(current))
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}
