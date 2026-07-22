// Package generatedartifacts owns policy-free filesystem operations for
// reviewed generated artifacts. Generators supply deterministic path/payload
// pairs; command adapters select this local store to write or compare them.
package generatedartifacts

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// Artifact is one repository-relative generated output and its expected bytes.
type Artifact struct {
	Path    string
	Payload []byte
	Absent  bool
}

// Drift describes byte-level differences between expected and stored outputs.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
}

// Empty reports whether all expected artifacts match their stored bytes.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// FileSystem is the exact host-filesystem edge required by LocalStore. The
// developer-tool process roots select its production implementation.
type FileSystem interface {
	ReadFile(string) ([]byte, error)
	Remove(string) error
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
}

// LocalStore performs generated-output effects beneath one repository root.
// Its zero value is deliberately inert: process roots must provide the host
// filesystem explicitly through NewLocalStore.
type LocalStore struct {
	files FileSystem
}

// Store is the complete generated-artifact persistence role used by generator
// process roots.
type Store interface {
	SourceStore
	Write(string, []Artifact) error
	Check(string, []Artifact) (Drift, error)
}

// SourceStore is the policy-free repository source read edge selected by a
// generator command or test owner.
type SourceStore interface {
	Read(string) ([]byte, error)
}

// NewLocalStore binds the exact filesystem mechanics required by the store.
func NewLocalStore(files FileSystem) (LocalStore, error) {
	if files == nil {
		return LocalStore{}, fmt.Errorf("create generated artifact store: filesystem is required")
	}
	return LocalStore{files: files}, nil
}

func (store LocalStore) Read(path string) ([]byte, error) {
	if store.files == nil {
		return nil, fmt.Errorf("read generated artifact %s: filesystem is required", path)
	}
	return store.files.ReadFile(path)
}

// Write persists every artifact, creating its parent directory when needed.
func (store LocalStore) Write(repositoryRoot string, artifacts []Artifact) error {
	if store.files == nil {
		return fmt.Errorf("write generated artifacts: filesystem is required")
	}
	for _, artifact := range artifacts {
		target := filepath.Join(repositoryRoot, filepath.FromSlash(artifact.Path))
		if artifact.Absent {
			if err := store.files.Remove(target); err != nil && !isNotExist(err) {
				return fmt.Errorf("remove retired generated artifact %s: %w", artifact.Path, err)
			}
			continue
		}
		if err := store.files.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", artifact.Path, err)
		}
		if err := store.files.WriteFile(target, artifact.Payload, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", artifact.Path, err)
		}
	}
	return nil
}

// Check compares every expected artifact with its stored bytes.
func (store LocalStore) Check(repositoryRoot string, artifacts []Artifact) (Drift, error) {
	if store.files == nil {
		return Drift{}, fmt.Errorf("check generated artifacts: filesystem is required")
	}
	drift := Drift{}
	for _, artifact := range artifacts {
		target := filepath.Join(repositoryRoot, filepath.FromSlash(artifact.Path))
		if artifact.Absent {
			if _, err := store.files.Stat(target); err == nil {
				drift.Unexpected = append(drift.Unexpected, artifact.Path)
			} else if !isNotExist(err) {
				return Drift{}, fmt.Errorf("inspect retired generated artifact %s: %w", artifact.Path, err)
			}
			continue
		}
		got, err := store.files.ReadFile(target)
		if err != nil {
			if isNotExist(err) {
				drift.Missing = append(drift.Missing, artifact.Path)
				continue
			}
			return Drift{}, fmt.Errorf("read %s: %w", artifact.Path, err)
		}
		if !bytes.Equal(normalize(got), normalize(artifact.Payload)) {
			drift.Stale = append(drift.Stale, artifact.Path)
		}
	}
	return drift, nil
}

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

func normalize(payload []byte) []byte {
	return bytes.ReplaceAll(payload, []byte("\r\n"), []byte("\n"))
}
