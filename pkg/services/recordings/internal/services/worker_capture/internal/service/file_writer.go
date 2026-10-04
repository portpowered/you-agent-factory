package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// FileWriter persists synced deltas; its map lock never covers disk I/O.
// Each process owns one writer. Multiple processes must not share this store.
type FileWriter struct {
	storage  platformreplay.Storage
	appender platformreplay.Appender
	root     string
	mu       sync.Mutex
	entries  map[string]*recordingEntry
}
type recordingEntry struct {
	mu       sync.Mutex
	loaded   bool
	exists   bool
	damaged  bool
	sessions map[string]*recordingSession
	order    []string
}
type recordingSession struct {
	projection recordings.WorkerRecordingProjection
	records    []events.Record
	identities map[events.AppendIdentity]events.Record
}
type workerJournalEntry struct {
	Version           int                                 `json:"version"`
	Kind              string                              `json:"kind"`
	RecordingID       string                              `json:"recordingId"`
	WorkerSessionID   string                              `json:"workerSessionId"`
	Record            *events.Record                      `json:"record,omitempty"`
	Topic             events.Topic                        `json:"topic,omitempty"`
	Code              string                              `json:"code,omitempty"`
	ExecutionTerminal *recordings.WorkerRecordingTerminal `json:"executionTerminal,omitempty"`
}

var _ recordings.WorkerRecordingWriter = (*FileWriter)(nil)
var _ recordings.WorkerRecordingReader = (*FileWriter)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*FileWriter)(nil)

// NewFileWriter requires the existing append-and-sync storage capability.
func NewFileWriter(storage platformreplay.Storage, root string) (recordings.WorkerRecordingWriter, error) {
	if storage == nil {
		return nil, fmt.Errorf("Worker recording file writer: storage is required")
	}
	appender, ok := storage.(platformreplay.Appender)
	if !ok {
		return nil, fmt.Errorf("Worker recording file writer: append storage is required")
	}
	if root == "" {
		return nil, fmt.Errorf("Worker recording file writer: root is required")
	}
	return &FileWriter{storage: storage, appender: appender, root: root, entries: make(map[string]*recordingEntry)}, nil
}
func (writer *FileWriter) entry(id string) *recordingEntry {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	entry := writer.entries[id]
	if entry == nil {
		entry = &recordingEntry{}
		writer.entries[id] = entry
	}
	return entry
}
func (entry *recordingEntry) session(recordingID, sessionID string) *recordingSession {
	if session := entry.sessions[sessionID]; session != nil {
		return session
	}
	return &recordingSession{projection: recordings.WorkerRecordingProjection{
		RecordingID: recordingID, WorkerSessionID: sessionID, Topic: events.Topic("worker-session/" + sessionID + "/events"),
		Status: recordings.WorkerRecordingStatusIncomplete}, identities: make(map[events.AppendIdentity]events.Record)}
}
func (entry *recordingEntry) commit(session *recordingSession) {
	id := session.projection.WorkerSessionID
	if entry.sessions[id] == nil {
		entry.order = append(entry.order, id)
	}
	entry.sessions[id] = session
	entry.exists = true
}

// PersistWorkerRecord admits a single detached record only after durable sync.
func (writer *FileWriter) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(record.RecordingID) == "" || strings.TrimSpace(record.WorkerSessionID) == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}
	entry := writer.entry(record.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, record.RecordingID, entry); err != nil {
		return err
	}
	if entry.damaged {
		return recordings.ErrWorkerRecordingReplay
	}
	session := entry.session(record.RecordingID, record.WorkerSessionID)
	projection, duplicate, err := session.prepareRecord(record.Record)
	if err != nil || duplicate {
		return err
	}
	delta := workerJournalEntry{Version: 1, Kind: "record", RecordingID: record.RecordingID, WorkerSessionID: record.WorkerSessionID, Record: &record.Record}
	if err := writer.append(ctx, entry, delta); err != nil {
		return err
	}
	session.acceptRecord(projection)
	entry.commit(session)
	return nil
}
func (session *recordingSession) prepareRecord(record events.Record) (recordings.WorkerRecordingProjection, bool, error) {
	if accepted, ok := session.identities[record.Identity()]; ok {
		if sameRecord(accepted, record) {
			return session.projection, true, nil
		}
		return recordings.WorkerRecordingProjection{}, false, recordings.ErrWorkerRecordingDuplicate
	}
	projection, err := (recordings.WorkerRecordingCodec{}).AdvanceWorkerRecording(session.projection, record)
	return projection, false, err
}
func (session *recordingSession) acceptRecord(projection recordings.WorkerRecordingProjection) {
	record := projection.Records[0]
	session.records = append(session.records, record)
	session.identities[record.Identity()] = record
	projection.Records = nil
	session.projection = projection
}

// PersistWorkerRecordingFailure appends a safe capture-loss fact.
func (writer *FileWriter) PersistWorkerRecordingFailure(ctx context.Context, failure recordings.WorkerRecordingFailure) error {
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(failure.RecordingID) == "" || strings.TrimSpace(failure.WorkerSessionID) == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}
	entry := writer.entry(failure.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, failure.RecordingID, entry); err != nil {
		return err
	}
	if entry.damaged {
		return recordings.ErrWorkerRecordingReplay
	}
	session := entry.session(failure.RecordingID, failure.WorkerSessionID)
	delta := workerJournalEntry{Version: 1, Kind: "failure", RecordingID: failure.RecordingID, WorkerSessionID: failure.WorkerSessionID, Topic: failure.Topic, Code: failure.Code, ExecutionTerminal: failure.ExecutionTerminal}
	projection, err := session.prepareFailure(delta)
	if err != nil {
		return err
	}
	if err := writer.append(ctx, entry, delta); err != nil {
		return err
	}
	session.projection = projection
	entry.commit(session)
	return nil
}
func (session *recordingSession) prepareFailure(delta workerJournalEntry) (recordings.WorkerRecordingProjection, error) {
	if delta.Topic != session.projection.Topic {
		return recordings.WorkerRecordingProjection{}, recordings.ErrWorkerRecordingOrder
	}
	if strings.TrimSpace(delta.Code) == "" {
		return recordings.WorkerRecordingProjection{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	return (recordings.WorkerRecordingCodec{}).FailWorkerRecording(session.projection, delta.Code, delta.ExecutionTerminal)
}
func (writer *FileWriter) append(ctx context.Context, entry *recordingEntry, delta workerJournalEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(delta)
	if err != nil {
		return fmt.Errorf("encode Worker recording delta: %w", err)
	}
	if err := writer.appender.AppendFile(writer.path(delta.RecordingID)+"l", append(data, '\n')); err != nil {
		// Even a close-after-sync error may have committed bytes. Rehydrate before
		// retrying instead of risking a second identity or trusting partial rollback.
		entry.loaded = false
		return fmt.Errorf("persist Worker recording delta: %w", err)
	}
	return nil
}

// LoadWorkerRecording returns detached, reducer-derived committed history.
func (writer *FileWriter) LoadWorkerRecording(ctx context.Context, id string) (recordings.WorkerRecordingSnapshot, error) {
	if writer == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingWriter
	}
	if strings.TrimSpace(id) == "" {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	entry := writer.entry(id)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, id, entry); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	if !entry.exists {
		return recordings.WorkerRecordingSnapshot{}, os.ErrNotExist
	}
	snapshot := recordings.WorkerRecordingSnapshot{RecordingID: id}
	for _, sessionID := range entry.order {
		session := entry.sessions[sessionID]
		p, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
			RecordingID: id, WorkerSessionID: sessionID, Topic: session.projection.Topic, Failure: session.projection.Degradation,
			ExecutionTerminal: session.projection.ExecutionTerminal, Records: session.records})
		if err != nil {
			return recordings.WorkerRecordingSnapshot{}, err
		}
		snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{
			WorkerSessionID: sessionID, Topic: p.Topic, Status: p.Status, LastPosition: p.LastPosition, Failure: p.Degradation,
			InterruptionReason: session.projection.InterruptionReason, ExecutionTerminal: cloneWorkerRecordingTerminal(p.ExecutionTerminal), Records: p.Records})
		if p.Status == recordings.WorkerRecordingStatusIncomplete && snapshot.Sessions[len(snapshot.Sessions)-1].InterruptionReason == "" {
			reason := p.Degradation
			if reason == "" {
				reason = recordings.WorkerRecordingInterruptionProcessStopped
			}
			snapshot.Sessions[len(snapshot.Sessions)-1].InterruptionReason = reason
		}
	}
	return snapshot, nil
}
func (writer *FileWriter) hydrate(ctx context.Context, id string, entry *recordingEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.loaded {
		return nil
	}
	// Build privately so malformed input cannot become cached accepted state.
	loaded := &recordingEntry{sessions: make(map[string]*recordingSession)}
	if err := writer.loadLegacy(id, loaded); err != nil {
		return err
	}
	data, err := writer.storage.ReadFile(writer.path(id) + "l")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load Worker journal: %w", err)
	}
	if err == nil {
		for len(data) > 0 {
			end := bytes.IndexByte(data, '\n')
			if end < 0 {
				loaded.damaged = true
				break
			}
			if err := loaded.applyLine(id, data[:end]); err != nil {
				return err
			}
			data = data[end+1:]
		}
	}
	if loaded.damaged {
		for _, session := range loaded.sessions {
			p, err := (recordings.WorkerRecordingCodec{}).FailWorkerRecording(session.projection, "PERSISTENCE_FAILED", session.projection.ExecutionTerminal)
			if err != nil {
				return err
			}
			session.projection = p
		}
	}
	entry.sessions = loaded.sessions
	entry.order = loaded.order
	entry.exists = loaded.exists
	entry.damaged = loaded.damaged
	entry.loaded = true
	return nil
}
func (writer *FileWriter) loadLegacy(id string, entry *recordingEntry) error {
	data, err := writer.storage.ReadFile(writer.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load Worker recording snapshot: %w", err)
	}
	var snapshot recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("%w: %v", recordings.ErrWorkerRecordingReplay, err)
	}
	if snapshot.RecordingID != id {
		return recordings.ErrWorkerRecordingReplay
	}
	if len(snapshot.Sessions) == 0 {
		return recordings.ErrWorkerRecordingReplay
	}
	for _, legacy := range snapshot.Sessions {
		result, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot, WorkerSessionID: legacy.WorkerSessionID})
		if err != nil {
			return err
		}
		session := entry.session(id, legacy.WorkerSessionID)
		session.records = result.Projection.Records
		session.projection = result.Projection
		session.projection.Records = nil
		for _, record := range session.records {
			session.identities[record.Identity()] = record
		}
		entry.commit(session)
	}
	return nil
}
func (entry *recordingEntry) applyLine(id string, line []byte) error {
	var delta workerJournalEntry
	if err := json.Unmarshal(line, &delta); err != nil {
		return fmt.Errorf("%w: %v", recordings.ErrWorkerRecordingReplay, err)
	}
	if delta.Version != 1 {
		return recordings.ErrWorkerRecordingCompatibility
	}
	if delta.RecordingID != id || strings.TrimSpace(delta.WorkerSessionID) == "" {
		return recordings.ErrWorkerRecordingReplay
	}
	session := entry.session(id, delta.WorkerSessionID)
	switch delta.Kind {
	case "record":
		if err := session.applyRecordDelta(delta); err != nil {
			return err
		}
	case "failure":
		if delta.Record != nil {
			return recordings.ErrWorkerRecordingReplay
		}
		projection, err := session.prepareFailure(delta)
		if err != nil {
			return err
		}
		session.projection = projection
	default:
		return recordings.ErrWorkerRecordingCompatibility
	}
	entry.commit(session)
	return nil
}

func (session *recordingSession) applyRecordDelta(delta workerJournalEntry) error {
	if delta.Record == nil || delta.Topic != "" || delta.Code != "" || delta.ExecutionTerminal != nil {
		return recordings.ErrWorkerRecordingReplay
	}
	projection, duplicate, err := session.prepareRecord(*delta.Record)
	if err != nil {
		return err
	}
	if duplicate {
		return recordings.ErrWorkerRecordingDuplicate
	}
	session.acceptRecord(projection)
	return nil
}

func (writer *FileWriter) path(recordingID string) string {
	digest := sha256.Sum256([]byte(recordingID))
	return filepath.Join(writer.root, hex.EncodeToString(digest[:])+".worker.json")
}

func cloneWorkerRecordingTerminal(terminal *recordings.WorkerRecordingTerminal) *recordings.WorkerRecordingTerminal {
	if terminal == nil {
		return nil
	}
	clone := *terminal
	return &clone
}
