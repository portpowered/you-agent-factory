package runtimepersist

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	durableSessionsHomeDir = ".you-agent-factory"
	durableSessionsSubdir  = "durable-sessions"
)

// Store persists durable runtime snapshots. Tests can inject an isolated store
// without coupling runtime construction to the host filesystem.
type Store interface {
	Save(sessionID string, encoded []byte) error
	Load(sessionID string) ([]byte, error)
}

// FileSystem is the exact host-filesystem capability consumed by this private
// persistence implementation. The public Factory Sessions edge is structurally
// compatible without coupling this implementation back to its parent package.
type FileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
}

// DirectoryStore persists snapshots beneath one explicit directory.
type DirectoryStore struct {
	Dir   string
	files FileSystem
}

// NewProjectStore constructs the durable snapshot store for one project root.
// Project path validation and filesystem initialization stay at this persistence
// boundary rather than leaking into application composition.
func NewProjectStore(
	projectRoot string,
	files FileSystem,
) (Store, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, errors.New("durable session persistence project root is required")
	}
	store, err := NewDirectoryStore(DirForProjectRoot(root), files)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// NewDirectoryStore validates and initializes an explicit snapshot directory.
func NewDirectoryStore(
	dir string,
	files FileSystem,
) (DirectoryStore, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return DirectoryStore{}, errors.New("durable session persistence directory is required")
	}
	if files == nil {
		return DirectoryStore{}, errors.New("durable session persistence filesystem is required")
	}
	if err := files.MkdirAll(trimmed, 0o700); err != nil {
		return DirectoryStore{}, fmt.Errorf("initialize durable session persistence directory: %w", err)
	}
	return DirectoryStore{Dir: trimmed, files: files}, nil
}

// Save writes a snapshot to the configured directory.
func (s DirectoryStore) Save(sessionID string, encoded []byte) error {
	return SaveBytes(s.Dir, sessionID, encoded, s.files)
}

// Load reads a snapshot from the configured directory.
func (s DirectoryStore) Load(sessionID string) ([]byte, error) {
	return LoadBytes(s.Dir, sessionID, s.files)
}

var durableSessionIDPattern = regexp.MustCompile(`^(dur-sess-[a-f0-9]{32}|~default|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})$`)

// DirForProjectRoot returns the project-local durable session persistence directory.
func DirForProjectRoot(projectRoot string) string {
	return filepath.Join(strings.TrimSpace(projectRoot), durableSessionsHomeDir, durableSessionsSubdir)
}

// SaveBytes writes one terminal runtime session snapshot payload for later CLI inspection.
func SaveBytes(
	dir, sessionID string,
	encoded []byte,
	files FileSystem,
) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return errors.New("durable session persistence directory is required")
	}
	if files == nil {
		return errors.New("durable session persistence filesystem is required")
	}
	if err := files.MkdirAll(trimmedDir, 0o700); err != nil {
		return fmt.Errorf("create durable session persistence directory: %w", err)
	}
	path := filepath.Join(trimmedDir, sessionID+".json")
	if err := files.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write durable session snapshot: %w", err)
	}
	return nil
}

// LoadBytes reads one persisted runtime session snapshot payload.
func LoadBytes(
	dir, sessionID string,
	files FileSystem,
) ([]byte, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return nil, errors.New("durable session persistence directory is required")
	}
	if files == nil {
		return nil, errors.New("durable session persistence filesystem is required")
	}
	path := filepath.Join(trimmedDir, sessionID+".json")
	encoded, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("read durable session snapshot: %w", err)
	}
	return encoded, nil
}

func validateSessionID(sessionID string) error {
	trimmed := strings.TrimSpace(sessionID)
	if !durableSessionIDPattern.MatchString(trimmed) {
		return fmt.Errorf("invalid durable session id %q", sessionID)
	}
	return nil
}
