// Package persistence provides policy-free filesystem persistence for
// Factory Session reconnect cursors.
package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
)

const schemaVersion = "factory-session-reconnect-cursor/v1"

type persistedCheckpoint struct {
	SchemaVersion string                                `json:"schemaVersion"`
	Identity      factorysessioncursors.StorageIdentity `json:"identity"`
	Checkpoint    factorysessioncursors.Checkpoint      `json:"checkpoint"`
}

// FileStore atomically persists cursor checkpoints beneath one explicit
// directory. It owns no recovery, remapping, or event-delivery policy.
type FileStore struct {
	dir                 string
	files               factorysessions.CursorPersistenceFileSystem
	createTemporaryFile factorysessions.CursorPersistenceCreateTemporaryFile

	mu     sync.RWMutex
	closed bool
}

// NewFileStore initializes an explicit cursor persistence directory.
func NewFileStore(
	dir string,
	files factorysessions.CursorPersistenceFileSystem,
	createTemporaryFile factorysessions.CursorPersistenceCreateTemporaryFile,
) (*FileStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("factory session reconnect cursor persistence directory is required")
	}
	if files == nil {
		return nil, errors.New("factory session reconnect cursor filesystem is required")
	}
	if createTemporaryFile == nil {
		return nil, errors.New("factory session reconnect cursor temporary-file creator is required")
	}
	if err := files.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("initialize factory session reconnect cursor persistence directory: %w", err)
	}
	return &FileStore{
		dir:                 dir,
		files:               files,
		createTemporaryFile: createTemporaryFile,
	}, nil
}

// Load reads and validates one persisted checkpoint. A missing file is the
// defined empty-start result.
func (s *FileStore) Load(
	ctx context.Context,
	identity factorysessioncursors.StorageIdentity,
) (factorysessioncursors.Checkpoint, bool, error) {
	if err := validateRequest(ctx, identity); err != nil {
		return factorysessioncursors.Checkpoint{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return factorysessioncursors.Checkpoint{}, false, factorysessioncursors.ErrStoreClosed
	}
	identity = factorysessioncursors.NormalizeStorageIdentity(identity)
	encoded, err := s.files.ReadFile(s.checkpointPath(identity))
	if errors.Is(err, fs.ErrNotExist) {
		return factorysessioncursors.Checkpoint{}, false, nil
	}
	if err != nil {
		return factorysessioncursors.Checkpoint{}, false, fmt.Errorf("read factory session reconnect cursor: %w", err)
	}
	var persisted persistedCheckpoint
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		return factorysessioncursors.Checkpoint{}, false, fmt.Errorf("decode factory session reconnect cursor: %w", err)
	}
	if persisted.SchemaVersion != schemaVersion {
		return factorysessioncursors.Checkpoint{}, false, fmt.Errorf(
			"unsupported factory session reconnect cursor schema version %q",
			persisted.SchemaVersion,
		)
	}
	if factorysessioncursors.NormalizeStorageIdentity(persisted.Identity) != identity {
		return factorysessioncursors.Checkpoint{}, false, errors.New("factory session reconnect cursor storage identity mismatch")
	}
	checkpoint := factorysessioncursors.NormalizeCheckpoint(persisted.Checkpoint)
	if err := factorysessioncursors.ValidateCheckpoint(checkpoint); err != nil {
		return factorysessioncursors.Checkpoint{}, false, fmt.Errorf("validate persisted factory session reconnect cursor: %w", err)
	}
	return checkpoint, true, nil
}

// Save atomically replaces one complete persisted checkpoint.
func (s *FileStore) Save(
	ctx context.Context,
	identity factorysessioncursors.StorageIdentity,
	checkpoint factorysessioncursors.Checkpoint,
) error {
	if err := validateRequest(ctx, identity); err != nil {
		return err
	}
	checkpoint = factorysessioncursors.NormalizeCheckpoint(checkpoint)
	if err := factorysessioncursors.ValidateCheckpoint(checkpoint); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return factorysessioncursors.ErrStoreClosed
	}
	identity = factorysessioncursors.NormalizeStorageIdentity(identity)
	encoded, err := json.Marshal(persistedCheckpoint{
		SchemaVersion: schemaVersion,
		Identity:      identity,
		Checkpoint:    checkpoint,
	})
	if err != nil {
		return fmt.Errorf("encode factory session reconnect cursor: %w", err)
	}
	if err := s.files.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create factory session reconnect cursor persistence directory: %w", err)
	}
	temporary, err := s.createTemporaryFile(s.dir, ".cursor-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary factory session reconnect cursor: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = s.files.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary factory session reconnect cursor: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		return fmt.Errorf("write temporary factory session reconnect cursor: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("flush temporary factory session reconnect cursor: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary factory session reconnect cursor: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.files.Rename(temporaryPath, s.checkpointPath(identity)); err != nil {
		return fmt.Errorf("commit factory session reconnect cursor: %w", err)
	}
	committed = true
	return nil
}

// Close ends the store lifecycle. It is idempotent.
func (s *FileStore) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func (s *FileStore) checkpointPath(identity factorysessioncursors.StorageIdentity) string {
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return filepath.Join(s.dir, hex.EncodeToString(digest[:])+".json")
}

func validateRequest(ctx context.Context, identity factorysessioncursors.StorageIdentity) error {
	if ctx == nil {
		return errors.New("factory session reconnect cursor context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return factorysessioncursors.ValidateStorageIdentity(identity)
}
