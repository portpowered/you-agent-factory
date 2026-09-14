//go:build windows

package tts_clean_install

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func rejectReparsePath(path string) error {
	current := filepath.Clean(path)
	for {
		name, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil {
			return err
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("component %q is a Windows reparse point", filepath.Base(current))
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}
