package http_test

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// remoteWorkerRecordingStore is the functional test's replaceable durable
// recording edge. It keeps the public writer/reader contract and the
// Recordings-owned reducer in the test while avoiding direct construction of
// the sibling recordings wire package from a transport test.
type remoteWorkerRecordingStore struct {
	mu        sync.Mutex
	snapshots map[string]recordings.WorkerRecordingSnapshot
}

func newRemoteWorkerRecordingStore() *remoteWorkerRecordingStore {
	return &remoteWorkerRecordingStore{snapshots: make(map[string]recordings.WorkerRecordingSnapshot)}
}

func (store *remoteWorkerRecordingStore) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	if store == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(record.RecordingID) == "" || strings.TrimSpace(record.WorkerSessionID) == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}
	if err := record.Record.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot := cloneRemoteWorkerRecordingSnapshot(store.snapshots[record.RecordingID])
	snapshot.RecordingID = record.RecordingID
	session := remoteWorkerRecordingSession(&snapshot, record.WorkerSessionID)
	for _, accepted := range session.Records {
		if accepted.Identity() != record.Record.Identity() {
			continue
		}
		if reflect.DeepEqual(accepted, record.Record) {
			return nil
		}
		return recordings.ErrWorkerRecordingDuplicate
	}
	history := append([]events.Record(nil), session.Records...)
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
	applyRemoteWorkerRecordingProjection(session, projection)
	store.snapshots[record.RecordingID] = snapshot
	return nil
}

func (store *remoteWorkerRecordingStore) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	if store == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(failure.RecordingID) == "" || strings.TrimSpace(failure.WorkerSessionID) == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot := cloneRemoteWorkerRecordingSnapshot(store.snapshots[failure.RecordingID])
	snapshot.RecordingID = failure.RecordingID
	session := remoteWorkerRecordingSession(&snapshot, failure.WorkerSessionID)
	if failure.Topic != "" {
		session.Topic = failure.Topic
	}
	session.Failure = failure.Code
	session.ExecutionTerminal = cloneRemoteWorkerRecordingTerminal(failure.ExecutionTerminal)
	projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       snapshot.RecordingID,
		WorkerSessionID:   session.WorkerSessionID,
		Topic:             session.Topic,
		Failure:           session.Failure,
		ExecutionTerminal: session.ExecutionTerminal,
		Records:           append([]events.Record(nil), session.Records...),
	})
	if err != nil {
		return err
	}
	applyRemoteWorkerRecordingProjection(session, projection)
	store.snapshots[failure.RecordingID] = snapshot
	return nil
}

func (store *remoteWorkerRecordingStore) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	if store == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	if strings.TrimSpace(recordingID) == "" {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot, ok := store.snapshots[recordingID]
	if !ok {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrWorkerRecordingReplay
	}
	return cloneRemoteWorkerRecordingSnapshot(snapshot), nil
}

func remoteWorkerRecordingSession(
	snapshot *recordings.WorkerRecordingSnapshot,
	workerSessionID string,
) *recordings.WorkerSessionRecordingSnapshot {
	for index := range snapshot.Sessions {
		if snapshot.Sessions[index].WorkerSessionID == workerSessionID {
			return &snapshot.Sessions[index]
		}
	}
	snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{WorkerSessionID: workerSessionID})
	return &snapshot.Sessions[len(snapshot.Sessions)-1]
}

func applyRemoteWorkerRecordingProjection(
	session *recordings.WorkerSessionRecordingSnapshot,
	projection recordings.WorkerRecordingProjection,
) {
	session.Topic = projection.Topic
	session.Status = projection.Status
	session.LastPosition = projection.LastPosition
	session.InterruptionReason = projection.InterruptionReason
	session.ExecutionTerminal = cloneRemoteWorkerRecordingTerminal(projection.ExecutionTerminal)
	session.Records = append([]events.Record(nil), projection.Records...)
}

func cloneRemoteWorkerRecordingSnapshot(
	snapshot recordings.WorkerRecordingSnapshot,
) recordings.WorkerRecordingSnapshot {
	clone := snapshot
	clone.Sessions = make([]recordings.WorkerSessionRecordingSnapshot, len(snapshot.Sessions))
	for index, session := range snapshot.Sessions {
		clone.Sessions[index] = session
		clone.Sessions[index].ExecutionTerminal = cloneRemoteWorkerRecordingTerminal(session.ExecutionTerminal)
		clone.Sessions[index].Records = make([]events.Record, len(session.Records))
		for recordIndex, record := range session.Records {
			clone.Sessions[index].Records[recordIndex] = record.Detached()
		}
	}
	return clone
}

func cloneRemoteWorkerRecordingTerminal(
	terminal *recordings.WorkerRecordingTerminal,
) *recordings.WorkerRecordingTerminal {
	if terminal == nil {
		return nil
	}
	clone := *terminal
	return &clone
}

var _ recordings.WorkerRecordingWriter = (*remoteWorkerRecordingStore)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*remoteWorkerRecordingStore)(nil)
var _ recordings.WorkerRecordingReader = (*remoteWorkerRecordingStore)(nil)
