package runtimepersist

import (
	"errors"
	"fmt"
	"os"
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

// DirectoryStore persists snapshots beneath one explicit directory.
type DirectoryStore struct{ Dir string }

// NewProjectStore constructs the durable snapshot store for one project root.
// Project path validation and filesystem initialization stay at this persistence
// boundary rather than leaking into application composition.
func NewProjectStore(projectRoot string) (Store, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, errors.New("durable session persistence project root is required")
	}
	store, err := NewDirectoryStore(DirForProjectRoot(root))
	if err != nil {
		return nil, err
	}
	return store, nil
}

// NewDirectoryStore validates and initializes an explicit snapshot directory.
func NewDirectoryStore(dir string) (DirectoryStore, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return DirectoryStore{}, errors.New("durable session persistence directory is required")
	}
	if err := os.MkdirAll(trimmed, 0o700); err != nil {
		return DirectoryStore{}, fmt.Errorf("initialize durable session persistence directory: %w", err)
	}
	return DirectoryStore{Dir: trimmed}, nil
}

// Save writes a snapshot to the configured directory.
func (s DirectoryStore) Save(sessionID string, encoded []byte) error {
	return SaveBytes(s.Dir, sessionID, encoded)
}

// Load reads a snapshot from the configured directory.
func (s DirectoryStore) Load(sessionID string) ([]byte, error) {
	return LoadBytes(s.Dir, sessionID)
}

var durableSessionIDPattern = regexp.MustCompile(`^(dur-sess-[a-f0-9]{32}|~default|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})$`)

// DirForProjectRoot returns the project-local durable session persistence directory.
func DirForProjectRoot(projectRoot string) string {
	return filepath.Join(strings.TrimSpace(projectRoot), durableSessionsHomeDir, durableSessionsSubdir)
}

// SaveBytes writes one terminal runtime session snapshot payload for later CLI inspection.
func SaveBytes(dir, sessionID string, encoded []byte) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return errors.New("durable session persistence directory is required")
	}
	if err := os.MkdirAll(trimmedDir, 0o700); err != nil {
		return fmt.Errorf("create durable session persistence directory: %w", err)
	}
	path := filepath.Join(trimmedDir, sessionID+".json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write durable session snapshot: %w", err)
	}
	return nil
}

// LoadBytes reads one persisted runtime session snapshot payload.
func LoadBytes(dir, sessionID string) ([]byte, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return nil, errors.New("durable session persistence directory is required")
	}
	path := filepath.Join(trimmedDir, sessionID+".json")
	encoded, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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
