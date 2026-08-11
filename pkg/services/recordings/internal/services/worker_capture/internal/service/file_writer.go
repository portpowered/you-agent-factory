package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
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
func (writer *FileWriter) PersistWorkerRecord(_ context.Context, record recordings.WorkerRecordingRecord) error {
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	if err := record.Record.Validate(); err != nil {
		return fmt.Errorf("validate Worker record: %w", err)
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	snapshot := cloneSnapshot(writer.snapshots[record.RecordingID])
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
	session.Records = append(session.Records, record.Record.Detached())
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode Worker recording snapshot: %w", err)
	}
	if err := writer.storage.WriteFile(writer.path(record.RecordingID), data); err != nil {
		return fmt.Errorf("persist Worker recording snapshot: %w", err)
	}
	writer.snapshots[record.RecordingID] = snapshot
	return nil
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
			WorkerSessionID: session.WorkerSessionID,
			Records:         make([]events.Record, len(session.Records)),
		}
		for j, record := range session.Records {
			clone.Sessions[i].Records[j] = record.Detached()
		}
	}
	return clone
}
