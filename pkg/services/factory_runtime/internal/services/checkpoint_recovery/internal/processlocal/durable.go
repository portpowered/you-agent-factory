package processlocal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

const (
	durableFormatVersion         = 1
	durablePersistedSuffix       = ".json"
	durableOpaqueDirectory       = "opaque-checkpoints"
	durableJavaScriptDirectory   = "javascript-checkpoints"
	durableOpaqueTempPattern     = ".checkpoint-*.tmp"
	durableJavaScriptTempPattern = ".javascript-checkpoint-*.tmp"
)

// DurableCheckpointFileSystem is the exact filesystem effect required by the
// durable checkpoint adapters. Wire selects the policy-free platform
// implementation; adapters receive only this narrow effect.
type DurableCheckpointFileSystem interface {
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]fs.DirEntry, error)
	MkdirAll(string, fs.FileMode) error
	CreateTemp(string, string) (platformfilesystem.TemporaryFile, error)
	Remove(string) error
	Rename(string, string) error
}

// DurableStore persists opaque checkpoint envelopes beneath one explicit
// directory. It owns no recovery policy beyond the CheckpointStore contract.
type DurableStore struct {
	root  string
	files DurableCheckpointFileSystem
	mu    sync.RWMutex
}

var _ checkpointrecovery.CheckpointStore = (*DurableStore)(nil)

type persistedEnvelope struct {
	FormatVersion int    `json:"formatVersion"`
	CheckpointID  string `json:"checkpointId"`
	SchemaVersion int    `json:"schemaVersion"`
	StrategyKind  string `json:"strategyKind"`
	Payload       []byte `json:"payload"`
}

// NewDurableCheckpointStore returns the wire-selectable constructor for the
// filesystem-backed opaque checkpoint adapter.
func NewDurableCheckpointStore(
	files DurableCheckpointFileSystem,
) func(string) (checkpointrecovery.CheckpointStore, error) {
	return func(durableRoot string) (checkpointrecovery.CheckpointStore, error) {
		dir, err := durableDirectory(durableRoot, durableOpaqueDirectory)
		if err != nil {
			return nil, err
		}
		return NewDurable(dir, files)
	}
}

// NewDurable constructs a filesystem-backed opaque checkpoint store rooted at
// dir. The directory is created lazily by Put so an empty root retains the
// existing missing-checkpoint read behavior.
func NewDurable(dir string, files DurableCheckpointFileSystem) (*DurableStore, error) {
	if files == nil {
		return nil, errors.New("durable checkpoint filesystem is required")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("durable checkpoint directory is required")
	}
	return &DurableStore{root: filepath.Clean(dir), files: files}, nil
}

// Put atomically stores or replaces one versioned opaque checkpoint envelope.
func (s *DurableStore) Put(envelope checkpointrecovery.Envelope) error {
	if err := checkpointrecovery.ValidateEnvelope(envelope); err != nil {
		return err
	}
	id := strings.TrimSpace(envelope.CheckpointID)
	encoded, err := json.Marshal(persistedEnvelope{
		FormatVersion: durableFormatVersion,
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
	return writeAtomically(
		s.files,
		s.root,
		s.pathFor(id),
		durableOpaqueTempPattern,
		"durable checkpoint",
		encoded,
	)
}

// Get returns one stored envelope when present and structurally valid.
func (s *DurableStore) Get(checkpointID string) (checkpointrecovery.Envelope, error) {
	id := strings.TrimSpace(checkpointID)
	if id == "" {
		return checkpointrecovery.Envelope{}, checkpointrecovery.ErrCheckpointNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	encoded, err := s.files.ReadFile(s.pathFor(id))
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
	if persisted.FormatVersion != durableFormatVersion {
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

func (s *DurableStore) pathFor(checkpointID string) string {
	digest := sha256.Sum256([]byte(checkpointID))
	return filepath.Join(s.root, hex.EncodeToString(digest[:])+durablePersistedSuffix)
}

// DurableJavaScriptStore persists JavaScript checkpoint records beneath one
// explicit directory. It keeps exact IDs, including surrounding whitespace.
type DurableJavaScriptStore struct {
	root  string
	files DurableCheckpointFileSystem
	mu    sync.RWMutex
}

var _ factoryruntime.JavaScriptCheckpointStore = (*DurableJavaScriptStore)(nil)

type persistedJavaScriptRecord struct {
	FormatVersion int       `json:"formatVersion"`
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	Summary       string    `json:"summary"`
	Timestamp     time.Time `json:"timestamp"`
	ArtifactID    string    `json:"artifactId"`
	ContentHash   string    `json:"contentHash"`
	SizeBytes     int64     `json:"sizeBytes"`
	RawBody       []byte    `json:"rawBody"`
	StoragePath   string    `json:"storagePath"`
}

var persistedJavaScriptFields = [...]string{
	"formatVersion",
	"id",
	"label",
	"summary",
	"timestamp",
	"artifactId",
	"contentHash",
	"sizeBytes",
	"rawBody",
	"storagePath",
}

// NewDurableJavaScriptCheckpointStore returns the wire-selectable constructor
// for the filesystem-backed JavaScript checkpoint adapter.
func NewDurableJavaScriptCheckpointStore(
	files DurableCheckpointFileSystem,
) func(string) (factoryruntime.JavaScriptCheckpointStore, error) {
	return func(durableRoot string) (factoryruntime.JavaScriptCheckpointStore, error) {
		dir, err := durableDirectory(durableRoot, durableJavaScriptDirectory)
		if err != nil {
			return nil, err
		}
		return newDurableJavaScriptStore(dir, files)
	}
}

func newDurableJavaScriptStore(
	dir string,
	files DurableCheckpointFileSystem,
) (*DurableJavaScriptStore, error) {
	if files == nil {
		return nil, errors.New("durable JavaScript checkpoint filesystem is required")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("durable JavaScript checkpoint directory is required")
	}
	return &DurableJavaScriptStore{root: filepath.Clean(dir), files: files}, nil
}

// Put atomically stores or replaces one JavaScript checkpoint record. The
// existing port intentionally has no error result, so filesystem failures are
// treated as an uncommitted write and do not escape through the port.
func (s *DurableJavaScriptStore) Put(record factorydefinitions.JavaScriptCheckpointRecord) {
	if s == nil || strings.TrimSpace(record.ID) == "" {
		return
	}
	encoded, err := json.Marshal(toPersistedJavaScriptRecord(record))
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_ = writeAtomically(
		s.files,
		s.root,
		s.pathFor(record.ID),
		durableJavaScriptTempPattern,
		"durable JavaScript checkpoint",
		encoded,
	)
}

// List returns valid checkpoint records in stable exact-ID order. Missing,
// corrupt, incomplete, temporary, and unrelated files are ignored so one bad
// entry cannot hide neighboring checkpoints.
func (s *DurableJavaScriptStore) List() []factorydefinitions.JavaScriptCheckpointRecord {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := s.files.ReadDir(s.root)
	if err != nil {
		return nil
	}

	records := make([]factorydefinitions.JavaScriptCheckpointRecord, 0, len(entries))
	for _, entry := range entries {
		if !isPersistedJavaScriptEntry(entry) {
			continue
		}
		record, ok := s.readJavaScriptEntry(entry.Name())
		if ok {
			records = append(records, record)
		}
	}
	sort.SliceStable(records, func(left, right int) bool {
		return records[left].ID < records[right].ID
	})
	return records
}

// Get returns one valid checkpoint record when present. Missing and corrupt
// records share the existing port's false result.
func (s *DurableJavaScriptStore) Get(id string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	if s == nil || strings.TrimSpace(id) == "" {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	encoded, err := s.files.ReadFile(s.pathFor(id))
	if err != nil {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	return decodeJavaScriptRecord(encoded, id)
}

func (s *DurableJavaScriptStore) readJavaScriptEntry(name string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	encoded, err := s.files.ReadFile(filepath.Join(s.root, name))
	if err != nil {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	record, ok := decodeJavaScriptRecord(encoded, "")
	if !ok || filepath.Base(s.pathFor(record.ID)) != name {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	return record, true
}

func (s *DurableJavaScriptStore) pathFor(id string) string {
	digest := sha256.Sum256([]byte(id))
	return filepath.Join(s.root, hex.EncodeToString(digest[:])+durablePersistedSuffix)
}

func toPersistedJavaScriptRecord(record factorydefinitions.JavaScriptCheckpointRecord) persistedJavaScriptRecord {
	return persistedJavaScriptRecord{
		FormatVersion: durableFormatVersion,
		ID:            record.ID,
		Label:         record.Label,
		Summary:       record.Summary,
		Timestamp:     record.Timestamp,
		ArtifactID:    record.ArtifactID,
		ContentHash:   record.ContentHash,
		SizeBytes:     record.SizeBytes,
		RawBody:       cloneRawBody(record.RawBody),
		StoragePath:   record.StoragePath,
	}
}

func decodeJavaScriptRecord(encoded []byte, expectedID string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	var persisted persistedJavaScriptRecord
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&persisted); err != nil || !hasAllPersistedJavaScriptFields(encoded) {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	if persisted.FormatVersion != durableFormatVersion || strings.TrimSpace(persisted.ID) == "" {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	if expectedID != "" && persisted.ID != expectedID {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	return fromPersistedJavaScriptRecord(persisted), true
}

func hasAllPersistedJavaScriptFields(encoded []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return false
	}
	for _, field := range persistedJavaScriptFields {
		if _, ok := fields[field]; !ok {
			return false
		}
	}
	return true
}

func fromPersistedJavaScriptRecord(record persistedJavaScriptRecord) factorydefinitions.JavaScriptCheckpointRecord {
	return factorydefinitions.JavaScriptCheckpointRecord{
		ID:          record.ID,
		Label:       record.Label,
		Summary:     record.Summary,
		Timestamp:   record.Timestamp,
		ArtifactID:  record.ArtifactID,
		ContentHash: record.ContentHash,
		SizeBytes:   record.SizeBytes,
		RawBody:     cloneRawBody(record.RawBody),
		StoragePath: record.StoragePath,
	}
}

func cloneRawBody(body []byte) []byte {
	if body == nil {
		return nil
	}
	cloned := make([]byte, len(body))
	copy(cloned, body)
	return cloned
}

func isPersistedJavaScriptEntry(entry fs.DirEntry) bool {
	return entry.Type().IsRegular() && filepath.Ext(entry.Name()) == durablePersistedSuffix
}

func durableDirectory(root, name string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", errors.New("durable checkpoint root is required")
	}
	return filepath.Join(filepath.Clean(trimmed), name), nil
}

func writeAtomically(
	files DurableCheckpointFileSystem,
	root string,
	destination string,
	temporaryPattern string,
	description string,
	encoded []byte,
) error {
	if err := files.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create %s directory: %w", description, err)
	}
	temporary, err := files.CreateTemp(root, temporaryPattern)
	if err != nil {
		return fmt.Errorf("create %s temporary file: %w", description, err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = files.Remove(temporaryPath)
		}
	}()
	written, err := temporary.WriteString(string(encoded))
	if err != nil {
		return fmt.Errorf("write %s temporary file: %w", description, err)
	}
	if written != len(encoded) {
		return fmt.Errorf("write %s temporary file: wrote %d of %d bytes", description, written, len(encoded))
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s temporary file: %w", description, err)
	}
	if err := files.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("commit %s: %w", description, err)
	}
	committed = true
	return nil
}

func corruptCheckpoint(format string, args ...any) error {
	return fmt.Errorf("%w: %s", checkpointrecovery.ErrCorruptCheckpoint, fmt.Sprintf(format, args...))
}
