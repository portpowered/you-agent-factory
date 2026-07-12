package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Write validates and atomically persists one portable recording. A failed
// write never leaves a destination that could be mistaken for a complete file.
func Write(path string, value Recording) error {
	if err := Validate(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recording: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create recording directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary recording: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary recording: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary recording: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary recording: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary recording: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish recording: %w", err)
	}
	return nil
}
