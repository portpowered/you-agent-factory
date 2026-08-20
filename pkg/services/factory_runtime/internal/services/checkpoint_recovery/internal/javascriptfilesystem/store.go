// Package javascriptfilesystem implements the durable filesystem-backed
// JavaScript checkpoint adapter inside the parent-private checkpoint recovery
// layout.
package javascriptfilesystem

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
	"sort"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	persistedFormatVersion = 1
	persistedSuffix        = ".json"
	temporaryFilePattern   = ".javascript-checkpoint-*.tmp"
)

var _ factoryruntime.JavaScriptCheckpointStore = (*Store)(nil)

// Store persists JavaScript checkpoint records beneath one explicit directory.
// It owns no checkpoint or resume policy beyond the JavaScript checkpoint store
// contract.
type Store struct {
	root string
	mu   sync.RWMutex
}

type persistedRecord struct {
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

var persistedFields = [...]string{
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

// New constructs a filesystem-backed JavaScript checkpoint store rooted at
// dir. The directory is created lazily by Put so an empty root retains the
// existing missing-record behavior.
func New(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("durable JavaScript checkpoint directory is required")
	}
	return &Store{root: filepath.Clean(dir)}, nil
}

// Put atomically stores or replaces one JavaScript checkpoint record. The
// existing port intentionally has no error result, so filesystem failures are
// treated as an uncommitted write and do not escape through the port.
func (s *Store) Put(record factorydefinitions.JavaScriptCheckpointRecord) {
	if s == nil || strings.TrimSpace(record.ID) == "" {
		return
	}
	encoded, err := json.Marshal(toPersistedRecord(record))
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_ = writeAtomically(s.root, s.pathFor(record.ID), encoded)
}

// List returns valid checkpoint records in stable ID order. Missing, corrupt,
// incomplete, temporary, and unrelated files are ignored so one bad entry
// cannot hide neighboring checkpoints.
func (s *Store) List() []factorydefinitions.JavaScriptCheckpointRecord {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}

	records := make([]factorydefinitions.JavaScriptCheckpointRecord, 0, len(entries))
	for _, entry := range entries {
		if !isPersistedEntry(entry) {
			continue
		}
		record, ok := s.readEntry(entry.Name())
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
func (s *Store) Get(id string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	if s == nil {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	encoded, err := os.ReadFile(s.pathFor(id))
	if errors.Is(err, fs.ErrNotExist) {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	if err != nil {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	return decodeRecord(encoded, id)
}

func (s *Store) readEntry(name string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	encoded, err := os.ReadFile(filepath.Join(s.root, name))
	if err != nil {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	record, ok := decodeRecord(encoded, "")
	if !ok || filepath.Base(s.pathFor(record.ID)) != name {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	return record, true
}

func toPersistedRecord(record factorydefinitions.JavaScriptCheckpointRecord) persistedRecord {
	return persistedRecord{
		FormatVersion: persistedFormatVersion,
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

func decodeRecord(encoded []byte, expectedID string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	var persisted persistedRecord
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&persisted); err != nil || !hasAllPersistedFields(encoded) {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	if persisted.FormatVersion != persistedFormatVersion || strings.TrimSpace(persisted.ID) == "" {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	if expectedID != "" && strings.TrimSpace(persisted.ID) != expectedID {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	return fromPersistedRecord(persisted), true
}

func hasAllPersistedFields(encoded []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return false
	}
	for _, field := range persistedFields {
		if _, ok := fields[field]; !ok {
			return false
		}
	}
	return true
}

func fromPersistedRecord(record persistedRecord) factorydefinitions.JavaScriptCheckpointRecord {
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

func isPersistedEntry(entry os.DirEntry) bool {
	return entry.Type().IsRegular() && filepath.Ext(entry.Name()) == persistedSuffix
}

func writeAtomically(root, destination string, encoded []byte) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create durable JavaScript checkpoint directory: %w", err)
	}
	temporary, err := os.CreateTemp(root, temporaryFilePattern)
	if err != nil {
		return fmt.Errorf("create durable JavaScript checkpoint temporary file: %w", err)
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
		return fmt.Errorf("secure durable JavaScript checkpoint temporary file: %w", err)
	}
	written, err := temporary.Write(encoded)
	if err != nil {
		return fmt.Errorf("write durable JavaScript checkpoint temporary file: %w", err)
	}
	if written != len(encoded) {
		return fmt.Errorf("write durable JavaScript checkpoint temporary file: wrote %d of %d bytes", written, len(encoded))
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("flush durable JavaScript checkpoint temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close durable JavaScript checkpoint temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("commit durable JavaScript checkpoint: %w", err)
	}
	committed = true
	return nil
}

func (s *Store) pathFor(id string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(id)))
	return filepath.Join(s.root, hex.EncodeToString(digest[:])+persistedSuffix)
}
