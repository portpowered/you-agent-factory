// Package filesystem implements the durable filesystem-backed checkpoint
// adapter for checkpoint_recovery.
package filesystem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

const persistedFormatVersion = 1

// Store persists versioned opaque checkpoint envelopes beneath one explicit
// directory. It owns no recovery policy beyond the CheckpointStore contract.
type Store struct {
	root string
	mu   sync.RWMutex
}

var _ checkpointrecovery.CheckpointStore = (*Store)(nil)

type persistedEnvelope struct {
	FormatVersion int    `json:"formatVersion"`
	CheckpointID  string `json:"checkpointId"`
	SchemaVersion int    `json:"schemaVersion"`
	StrategyKind  string `json:"strategyKind"`
	Payload       []byte `json:"payload"`
}

// New constructs a filesystem-backed checkpoint store rooted at dir. The
// directory is created lazily by Put so an empty root still has the existing
// missing-checkpoint read behavior.
func New(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("durable checkpoint directory is required")
	}
	return &Store{root: filepath.Clean(dir)}, nil
}

// Put atomically stores or replaces one versioned opaque checkpoint envelope.
func (s *Store) Put(envelope checkpointrecovery.Envelope) error {
	if err := checkpointrecovery.ValidateEnvelope(envelope); err != nil {
		return err
	}
	id := strings.TrimSpace(envelope.CheckpointID)
	encoded, err := json.Marshal(persistedEnvelope{
		FormatVersion: persistedFormatVersion,
		CheckpointID:  id,
		SchemaVersion: envelope.SchemaVersion,
		StrategyKind:  envelope.StrategyKind,
		Payload:       envelope.Payload,
	})
	if err != nil {
		return fmt.Errorf("encode durable checkpoint %q: %w", id, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create durable checkpoint directory: %w", err)
	}
	temporary, err := os.CreateTemp(s.root, ".checkpoint-*.tmp")
	if err != nil {
		return fmt.Errorf("create durable checkpoint temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure durable checkpoint temporary file: %w", err)
	}
	written, err := temporary.Write(encoded)
	if err != nil {
		return fmt.Errorf("write durable checkpoint temporary file: %w", err)
	}
	if written != len(encoded) {
		return fmt.Errorf("write durable checkpoint temporary file: wrote %d of %d bytes", written, len(encoded))
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("flush durable checkpoint temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close durable checkpoint temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.pathFor(id)); err != nil {
		return fmt.Errorf("commit durable checkpoint %q: %w", id, err)
	}
	committed = true
	return nil
}

// Get returns one stored envelope when present and structurally valid.
func (s *Store) Get(checkpointID string) (checkpointrecovery.Envelope, error) {
	id := strings.TrimSpace(checkpointID)
	if id == "" {
		return checkpointrecovery.Envelope{}, checkpointrecovery.ErrCheckpointNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	encoded, err := os.ReadFile(s.pathFor(id))
	if errors.Is(err, fs.ErrNotExist) {
		return checkpointrecovery.Envelope{}, checkpointrecovery.ErrCheckpointNotFound
	}
	if err != nil {
		return checkpointrecovery.Envelope{}, fmt.Errorf("read durable checkpoint %q: %w", id, err)
	}

	var persisted persistedEnvelope
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&persisted); err != nil {
		return checkpointrecovery.Envelope{}, corruptCheckpoint("decode checkpoint %q: %v", id, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return checkpointrecovery.Envelope{}, corruptCheckpoint("checkpoint %q contains trailing data", id)
		}
		return checkpointrecovery.Envelope{}, corruptCheckpoint("decode trailing data for checkpoint %q: %v", id, err)
	}
	if persisted.FormatVersion != persistedFormatVersion {
		return checkpointrecovery.Envelope{}, corruptCheckpoint("checkpoint %q has unsupported format version %d", id, persisted.FormatVersion)
	}
	if persisted.CheckpointID != id {
		return checkpointrecovery.Envelope{}, corruptCheckpoint("checkpoint %q has mismatched identity %q", id, persisted.CheckpointID)
	}
	envelope := checkpointrecovery.Envelope{
		CheckpointID:  persisted.CheckpointID,
		SchemaVersion: persisted.SchemaVersion,
		StrategyKind:  persisted.StrategyKind,
		Payload:       persisted.Payload,
	}
	if err := checkpointrecovery.ValidateEnvelope(envelope); err != nil {
		return checkpointrecovery.Envelope{}, corruptCheckpoint("validate checkpoint %q: %v", id, err)
	}
	return envelope, nil
}

func (s *Store) pathFor(checkpointID string) string {
	digest := sha256.Sum256([]byte(checkpointID))
	return filepath.Join(s.root, hex.EncodeToString(digest[:])+".json")
}

func corruptCheckpoint(format string, args ...any) error {
	return fmt.Errorf("%w: %s", checkpointrecovery.ErrCorruptCheckpoint, fmt.Sprintf(format, args...))
}
