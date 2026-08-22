package service

import (
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

// FileWriter durably snapshots source-native Worker records after each
// accepted append. The hash-based filename keeps opaque recording identities
// out of paths while preserving one stable sidecar per recording.
type FileWriter struct {
	storage platformreplay.Storage
	root    string

	mu        sync.Mutex
	snapshots map[string]recordings.WorkerRecordingSnapshot
}

var _ recordings.WorkerRecordingWriter = (*FileWriter)(nil)
var _ recordings.WorkerRecordingReader = (*FileWriter)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*FileWriter)(nil)

// NewFileWriter constructs the default local durable Worker-record sidecar
// writer. The storage port owns atomic replacement and filesystem mechanics.
func NewFileWriter(storage platformreplay.Storage, root string) (recordings.WorkerRecordingWriter, error) {
	if storage == nil {
		return nil, fmt.Errorf("Worker recording file writer: storage is required")
	}
	if root == "" {
		return nil, fmt.Errorf("Worker recording file writer: root is required")
	}
	return &FileWriter{storage: storage, root: root, snapshots: make(map[string]recordings.WorkerRecordingSnapshot)}, nil
}

// PersistWorkerRecord writes a complete detached snapshot before reporting
// success, so the opening barrier cannot observe an in-memory-only acceptance.
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
	if err := record.Record.Validate(); err != nil {
		return fmt.Errorf("validate Worker record: %w", err)
	}
	return writer.persistWorkerRecord(ctx, record)
}

func (writer *FileWriter) persistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	snapshot, err := writer.snapshotForWrite(ctx, record.RecordingID)
	if err != nil {
		return err
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if cached, ok := writer.snapshots[record.RecordingID]; ok {
		snapshot = cloneSnapshot(cached)
	}
	snapshot.RecordingID = record.RecordingID
	var session *recordings.WorkerSessionRecordingSnapshot
	for i := range snapshot.Sessions {
		if snapshot.Sessions[i].WorkerSessionID == record.WorkerSessionID {
			session = &snapshot.Sessions[i]
			break
		}
	}
	if session == nil {
		snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{WorkerSessionID: record.WorkerSessionID})
		session = &snapshot.Sessions[len(snapshot.Sessions)-1]
	}
	for _, accepted := range session.Records {
		if accepted.Identity() != record.Record.Identity() {
			continue
		}
		if sameRecord(accepted, record.Record) {
			return nil
		}
		return fmt.Errorf("%w: source identity %q/%q/%d/%q changed", recordings.ErrWorkerRecordingDuplicate, record.Record.SourceType, record.Record.SourceID, record.Record.SourceSequence, record.Record.SourceEventID)
	}
	history := make([]events.Record, 0, len(session.Records)+1)
	for _, accepted := range session.Records {
		history = append(history, accepted.Detached())
	}
	history = append(history, record.Record.Detached())
	projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       record.RecordingID,
		WorkerSessionID:   record.WorkerSessionID,
		Topic:             session.Topic,
		Failure:           session.Failure,
		ExecutionTerminal: session.ExecutionTerminal,
		Records:           history,
	})
	if err != nil {
		return err
	}
	session.Topic = projection.Topic
	session.Status = projection.Status
	session.LastPosition = projection.LastPosition
	session.InterruptionReason = projection.InterruptionReason
	session.ExecutionTerminal = cloneWorkerRecordingTerminal(projection.ExecutionTerminal)
	session.Records = projection.Records
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode Worker recording snapshot: %w", err)
	}
	if err := writer.storage.WriteFile(writer.path(record.RecordingID), data); err != nil {
		return fmt.Errorf("persist Worker recording snapshot: %w", err)
	}
	if writer.snapshots == nil {
		writer.snapshots = make(map[string]recordings.WorkerRecordingSnapshot)
	}
	writer.snapshots[record.RecordingID] = snapshot
	return nil
}
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

	snapshot, err := writer.snapshotForWrite(ctx, failure.RecordingID)
	if err != nil {
		return err
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if cached, ok := writer.snapshots[failure.RecordingID]; ok {
		snapshot = cloneSnapshot(cached)
	}
	snapshot.RecordingID = failure.RecordingID
	session := findOrCreateSession(&snapshot, failure.WorkerSessionID)
	if failure.Topic != "" {
		session.Topic = failure.Topic
	}
	session.Failure = failure.Code
	session.ExecutionTerminal = cloneWorkerRecordingTerminal(failure.ExecutionTerminal)
	projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       snapshot.RecordingID,
		WorkerSessionID:   session.WorkerSessionID,
		Topic:             session.Topic,
		Failure:           session.Failure,
		ExecutionTerminal: session.ExecutionTerminal,
		Records:           session.Records,
	})
	if err != nil {
		return fmt.Errorf("classify failed Worker recording snapshot: %w", err)
	}
	session.Topic = projection.Topic
	session.Status = projection.Status
	session.LastPosition = projection.LastPosition
	session.InterruptionReason = projection.InterruptionReason
	session.ExecutionTerminal = cloneWorkerRecordingTerminal(projection.ExecutionTerminal)
	session.Records = projection.Records
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode failed Worker recording snapshot: %w", err)
	}
	if err := writer.storage.WriteFile(writer.path(failure.RecordingID), data); err != nil {
		return fmt.Errorf("persist failed Worker recording snapshot: %w", err)
	}
	if writer.snapshots == nil {
		writer.snapshots = make(map[string]recordings.WorkerRecordingSnapshot)
	}
	writer.snapshots[failure.RecordingID] = snapshot
	return nil
}

// LoadWorkerRecording reads one opaque recording identity and derives each
// session's health from its durable evidence before returning. This keeps
// reopened sidecars on the same pure classification path as live capture and
// portable replay.
func (writer *FileWriter) LoadWorkerRecording(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	if writer == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingWriter
	}
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	if strings.TrimSpace(recordingID) == "" {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrInvalidWorkerRecordingRequest
	}

	writer.mu.Lock()
	if snapshot, ok := writer.snapshots[recordingID]; ok {
		clone := cloneSnapshot(snapshot)
		writer.mu.Unlock()
		return clone, nil
	}
	writer.mu.Unlock()

	snapshot, err := writer.readDurableSnapshot(ctx, recordingID)
	if err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	clone := cloneSnapshot(snapshot)
	writer.mu.Lock()
	if writer.snapshots == nil {
		writer.snapshots = make(map[string]recordings.WorkerRecordingSnapshot)
	}
	writer.snapshots[recordingID] = cloneSnapshot(snapshot)
	writer.mu.Unlock()
	return clone, nil
}

func (writer *FileWriter) snapshotForWrite(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	writer.mu.Lock()
	if snapshot, ok := writer.snapshots[recordingID]; ok {
		clone := cloneSnapshot(snapshot)
		writer.mu.Unlock()
		return clone, nil
	}
	writer.mu.Unlock()

	snapshot, err := writer.readDurableSnapshot(ctx, recordingID)
	if errors.Is(err, os.ErrNotExist) {
		return recordings.WorkerRecordingSnapshot{RecordingID: recordingID}, nil
	}
	return snapshot, err
}

func (writer *FileWriter) readDurableSnapshot(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	data, err := writer.storage.ReadFile(writer.path(recordingID))
	if err != nil {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("load Worker recording snapshot: %w", err)
	}
	var snapshot recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: decode Worker recording snapshot: %v", recordings.ErrWorkerRecordingReplay, err)
	}
	if snapshot.RecordingID != recordingID {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: snapshot identity %q does not match %q", recordings.ErrWorkerRecordingReplay, snapshot.RecordingID, recordingID)
	}
	if len(snapshot.Sessions) == 0 {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: Worker Session history is missing", recordings.ErrWorkerRecordingReplay)
	}
	for index := range snapshot.Sessions {
		session := &snapshot.Sessions[index]
		result, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
			Snapshot:        snapshot,
			WorkerSessionID: session.WorkerSessionID,
		})
		if err != nil {
			return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("classify Worker recording snapshot: %w", err)
		}
		projection := result.Projection
		session.Topic = projection.Topic
		session.Status = projection.Status
		session.LastPosition = projection.LastPosition
		session.InterruptionReason = projection.InterruptionReason
		session.ExecutionTerminal = cloneWorkerRecordingTerminal(projection.ExecutionTerminal)
		session.Records = projection.Records
	}
	return snapshot, nil
}

func (writer *FileWriter) path(recordingID string) string {
	digest := sha256.Sum256([]byte(recordingID))
	return filepath.Join(writer.root, hex.EncodeToString(digest[:])+".worker.json")
}

func cloneSnapshot(snapshot recordings.WorkerRecordingSnapshot) recordings.WorkerRecordingSnapshot {
	clone := snapshot
	clone.Sessions = make([]recordings.WorkerSessionRecordingSnapshot, len(snapshot.Sessions))
	for i, session := range snapshot.Sessions {
		clone.Sessions[i] = recordings.WorkerSessionRecordingSnapshot{
			WorkerSessionID:    session.WorkerSessionID,
			Topic:              session.Topic,
			Status:             session.Status,
			LastPosition:       session.LastPosition,
			Failure:            session.Failure,
			InterruptionReason: session.InterruptionReason,
			ExecutionTerminal:  cloneWorkerRecordingTerminal(session.ExecutionTerminal),
			Records:            make([]events.Record, len(session.Records)),
		}
		for j, record := range session.Records {
			clone.Sessions[i].Records[j] = record.Detached()
		}
	}
	return clone
}

func cloneWorkerRecordingTerminal(terminal *recordings.WorkerRecordingTerminal) *recordings.WorkerRecordingTerminal {
	if terminal == nil {
		return nil
	}
	clone := *terminal
	return &clone
}

func findOrCreateSession(
	snapshot *recordings.WorkerRecordingSnapshot,
	workerSessionID string,
) *recordings.WorkerSessionRecordingSnapshot {
	for i := range snapshot.Sessions {
		if snapshot.Sessions[i].WorkerSessionID == workerSessionID {
			return &snapshot.Sessions[i]
		}
	}
	snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{WorkerSessionID: workerSessionID})
	return &snapshot.Sessions[len(snapshot.Sessions)-1]
}
