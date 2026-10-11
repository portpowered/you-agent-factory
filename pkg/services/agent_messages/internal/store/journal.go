package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"unicode/utf8"
)

// FileSystem is the exact file effect accepted by the journal. The production
// adapter supplies durable, truncatable descriptors; the store owns the
// format, transaction ordering and rollback policy.
type FileSystem interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, fs.FileMode) error
	OpenFile(string, int, fs.FileMode) (io.WriteCloser, error)
	// ReplaceDurable atomically publishes flushed bytes. Failure preserves the
	// existing file; implementations must never remove it before replacement.
	ReplaceDurable(string, []byte) error
}

type durableFile interface {
	io.WriteCloser
	io.Seeker
	Stat() (fs.FileInfo, error)
	Sync() error
	Truncate(int64) error
}

// Journal serializes transactions for one process-owned profile. Admission
// holds its own lock across policy, quota checks, Commit, and publication.
// Separate processes must not share a writable profile.
type Journal struct {
	mu       sync.Mutex
	fs       FileSystem
	path     string
	opened   bool
	fault    error
	sequence uint64
	records  map[string]bool
	entries  map[string]Entry
	requests map[requestKey]Request
	ordered  []string
	indexes  map[indexKey][]string
}

type requestKey struct{ sender, request string }

func New(files FileSystem, path string) *Journal {
	return &Journal{fs: files, path: path}
}

// Open reconstructs the complete committed journal without modifying bytes.
// Corruption fails closed, including a truncated final line. It is a messaging
// error, not a failure of the application's Factory execution lifecycle.
func (j *Journal) Open() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.opened || j.fault != nil {
		return j.fault
	}
	data, err := j.fs.ReadFile(j.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		j.fault = ErrUnavailable
		return j.fault
	}
	j.entries = make(map[string]Entry)
	j.requests = make(map[requestKey]Request)
	j.records = make(map[string]bool)
	j.indexes = make(map[indexKey][]string)
	if err := j.replay(data); err != nil {
		j.fault = ErrCorrupt
		return j.fault
	}
	j.opened = true
	return nil
}

func (j *Journal) replay(data []byte) error {
	if !utf8.Valid(data) {
		return ErrCorrupt
	}
	if len(data) == 0 {
		return nil
	}
	if data[len(data)-1] != '\n' {
		return ErrCorrupt
	}
	for _, line := range bytes.Split(data[:len(data)-1], []byte{'\n'}) {
		var transaction Transaction
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&transaction); err != nil {
			return ErrCorrupt
		}
		if decoder.Decode(new(any)) != io.EOF {
			return ErrCorrupt
		}
		if err := j.validate(transaction); err != nil {
			return ErrCorrupt
		}
		j.apply(transaction)
	}
	return nil
}

// Commit updates the index only after append and fsync succeed. Failures omit
// filesystem paths and underlying diagnostics which might contain payloads.
func (j *Journal) Commit(transaction Transaction) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.opened || j.fault != nil {
		return j.unavailable()
	}
	if err := j.validate(transaction); err != nil {
		return err
	}
	encoded, err := json.Marshal(transaction)
	if err != nil {
		return ErrInvalidTransaction
	}
	if err := j.append(append(encoded, '\n')); err != nil {
		return err
	}
	j.apply(transaction)
	return nil
}

func (j *Journal) append(line []byte) error {
	if err := j.fs.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return ErrUnavailable
	}
	// Windows append-only handles cannot truncate a failed transaction. The
	// profile has one writer, so seek under the journal lock with a read/write
	// descriptor instead of acquiring an append-only host capability.
	writer, err := j.fs.OpenFile(j.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return ErrUnavailable
	}
	file, ok := writer.(durableFile)
	if !ok {
		_ = writer.Close()
		return ErrUnavailable
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return ErrUnavailable
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		return ErrUnavailable
	}
	written, err := file.Write(line)
	if err != nil || written != len(line) {
		return j.rollback(file, info.Size())
	}
	if err := file.Sync(); err != nil {
		return j.rollback(file, info.Size())
	}
	// A successfully flushed record is committed even if descriptor cleanup
	// reports a late failure. Returning failure would invite a duplicate retry.
	_ = file.Close()
	return nil
}

func (j *Journal) rollback(file durableFile, size int64) error {
	truncateErr := file.Truncate(size)
	syncErr := file.Sync()
	_ = file.Close()
	if truncateErr != nil || syncErr != nil {
		// An uncertain disk tail cannot admit another transaction. Reconstruction
		// validates the bytes; never silently repair or discard committed data.
		j.fault = ErrUnavailable
	}
	return ErrUnavailable
}

func (j *Journal) unavailable() error {
	if j.fault != nil {
		return j.fault
	}
	return ErrUnavailable
}

func (j *Journal) apply(t Transaction) {
	// A compacted snapshot may serialize entries in any order. Build indexes
	// once in original admission order, then append only new admissions.
	if t.Kind == Snapshot {
		t.Messages = append([]Entry{}, t.Messages...)
		sort.Slice(t.Messages, func(a, b int) bool { return t.Messages[a].Sequence < t.Messages[b].Sequence })
	}
	for _, entry := range t.Messages {
		if _, exists := j.entries[entry.Message.MessageID]; !exists {
			j.index(entry)
		}
		j.entries[entry.Message.MessageID] = entry
	}
	for _, request := range t.Requests {
		j.requests[requestKey{request.SenderIdentity, request.RequestID}] = request
	}
	j.records[t.RecordID] = true
	j.sequence = t.Sequence
}
