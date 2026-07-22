package artifacts

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

type TemporaryFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type MakeDirectories func(string, fs.FileMode) error
type CreateTemporaryFile func(string, string) (TemporaryFile, error)
type RemovePath func(string) error
type RenamePath func(string, string) error

type Writer interface {
	Write(string, Recording) error
}

type AtomicWriter struct {
	makeDirectories     MakeDirectories
	createTemporaryFile CreateTemporaryFile
	removePath          RemovePath
	renamePath          RenamePath
}

func NewAtomicWriter(makeDirectories MakeDirectories, createTemporaryFile CreateTemporaryFile, removePath RemovePath, renamePath RenamePath) (*AtomicWriter, error) {
	if makeDirectories == nil || createTemporaryFile == nil || removePath == nil || renamePath == nil {
		return nil, fmt.Errorf("recording artifact write operations are required")
	}
	return &AtomicWriter{makeDirectories: makeDirectories, createTemporaryFile: createTemporaryFile, removePath: removePath, renamePath: renamePath}, nil
}

// Write validates and atomically persists one portable recording. A failed
// write never leaves a destination that could be mistaken for a complete file.
func (writer *AtomicWriter) Write(path string, value Recording) error {
	if writer == nil {
		return fmt.Errorf("recording artifact writer is required")
	}
	if err := Validate(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recording: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := writer.makeDirectories(dir, 0o700); err != nil {
		return fmt.Errorf("create recording directory: %w", err)
	}
	temporary, err := writer.createTemporaryFile(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary recording: %w", err)
	}
	temporaryPath := temporary.Name()
	defer writer.removePath(temporaryPath)
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
	if err := writer.renamePath(temporaryPath, path); err != nil {
		return fmt.Errorf("publish recording: %w", err)
	}
	return nil
}
