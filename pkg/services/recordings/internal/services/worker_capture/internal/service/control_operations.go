package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Each request key serializes only its own sync/CAS work. The index is rebuilt
// once from the same journals; no process-wide lock covers an ordinary append.
type controlKeySlot struct {
	mu  sync.Mutex
	key recordings.WorkerControlOperationKey
}

func operationKey(record recordings.WorkerControlOperationRecord) recordings.WorkerControlOperationKey {
	return recordings.WorkerControlOperationKey{RecordingID: record.Target.RecordingID, WorkerSessionID: record.Target.WorkerSessionID, FactorySessionID: record.Target.FactorySessionID, RequestID: record.Operation.RequestID}
}

func (writer *FileWriter) controlSlot(id string) *controlKeySlot {
	writer.controlIndexMu.Lock()
	defer writer.controlIndexMu.Unlock()
	if writer.controlIndex == nil {
		writer.controlIndex = make(map[string]*controlKeySlot)
	}
	slot := writer.controlIndex[id]
	if slot == nil {
		slot = &controlKeySlot{}
		writer.controlIndex[id] = slot
	}
	return slot
}

func (writer *FileWriter) rebuildControlIndex(ctx context.Context) error {
	writer.controlRebuildMu.Lock()
	defer writer.controlRebuildMu.Unlock()
	if writer.controlIndexLoaded {
		return ctx.Err()
	}
	err := writer.directory.ScanDirectory(writer.root, 64, func(files []os.DirEntry) error {
		return writer.indexControlFiles(ctx, files)
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	writer.controlIndexLoaded = true
	return ctx.Err()
}

func (writer *FileWriter) indexControlFiles(ctx context.Context, files []os.DirEntry) error {
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".worker.jsonl") {
			continue
		}
		if file.Type()&os.ModeSymlink != 0 {
			return recordings.ErrWorkerRecordingReplay
		}
		data, err := writer.storage.ReadFile(filepath.Join(writer.root, file.Name()))
		if err != nil {
			return err
		}
		id, err := writer.recordingFileIdentity(file.Name(), data)
		if err != nil {
			return err
		}
		if err := writer.indexRecordingControls(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (writer *FileWriter) indexRecordingControls(ctx context.Context, id string) error {
	entry := writer.entry(id)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrateControls(ctx, id, entry); err != nil {
		return err
	}
	for requestID, records := range entry.operations {
		slot := writer.controlSlot(requestID)
		key := operationKey(records[0])
		slot.mu.Lock()
		conflict := slot.key.RequestID != "" && slot.key != key
		if !conflict {
			slot.key = key
		}
		slot.mu.Unlock()
		if conflict {
			return recordings.ErrWorkerControlConflict
		}
	}
	return nil
}

// BeginWorkerControlOperation returns acceptance only after journal sync.
func (writer *FileWriter) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	if err := record.ValidateIntent(); err != nil {
		return recordings.WorkerControlOperationRecord{}, false, err
	}
	if err := writer.rebuildControlIndex(ctx); err != nil {
		return recordings.WorkerControlOperationRecord{}, false, err
	}
	key := operationKey(record)
	slot := writer.controlSlot(key.RequestID)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.key.RequestID != "" && slot.key != key {
		return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerControlConflict
	}
	entry := writer.entry(key.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrateControls(ctx, key.RecordingID, entry); err != nil {
		return recordings.WorkerControlOperationRecord{}, false, err
	}
	if previous := entry.operations[key.RequestID]; len(previous) > 0 {
		latest := previous[len(previous)-1]
		if !latest.SameIntent(record) {
			return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerControlConflict
		}
		return latest.Detached(), false, nil
	}
	if err := entry.validateControlTarget(record.Target); err != nil {
		return recordings.WorkerControlOperationRecord{}, false, err
	}
	// A new intent cannot acquire authority over an affirmatively dead
	// supervisor. Existing keyed results above remain readable and replayable.
	if session := entry.sessions[key.WorkerSessionID]; session.projection.Degradation == "OWNER_LOST" || (session.projection.ExecutionTerminal == nil && writer.ownerDeathWitness(session.ownerEpoch)) {
		return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerControlConflict
	}
	if record.InputArtifactRef != "" {
		if err := writer.validateControlInput(ctx, entry, record); err != nil {
			return recordings.WorkerControlOperationRecord{}, false, err
		}
	}
	// Retain the key reservation on uncertain append failure until this exact
	// journal is reconciled. It must not be reused on another source.
	slot.key = key
	if err := writer.appendControl(ctx, entry, record); err != nil {
		return recordings.WorkerControlOperationRecord{}, false, err
	}
	return record.Detached(), true, nil
}

func (writer *FileWriter) AdvanceWorkerControlOperation(ctx context.Context, next recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	if err := next.Validate(); err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	entry := writer.entry(next.Target.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrateControls(ctx, next.Target.RecordingID, entry); err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	history := entry.operations[next.Operation.RequestID]
	if len(history) == 0 {
		return recordings.WorkerControlOperationRecord{}, os.ErrNotExist
	}
	if !history[len(history)-1].CanAdvance(next, expected) {
		return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerControlConflict
	}
	if err := entry.validateControlTarget(next.Target); err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	if err := writer.appendControl(ctx, entry, next); err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	return next.Detached(), nil
}

func (writer *FileWriter) LoadWorkerControlOperation(ctx context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	entry := writer.entry(key.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrateControls(ctx, key.RecordingID, entry); err != nil {
		return recordings.WorkerControlOperationRecord{}, err
	}
	history := entry.operations[key.RequestID]
	if len(history) == 0 || operationKey(history[0]) != key {
		return recordings.WorkerControlOperationRecord{}, os.ErrNotExist
	}
	return history[len(history)-1].Detached(), nil
}

// ListWorkerControlOperations keeps phase facts preceding a later failed result.
func (writer *FileWriter) ListWorkerControlOperations(ctx context.Context, target recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	entry := writer.entry(target.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrateControls(ctx, target.RecordingID, entry); err != nil {
		return nil, err
	}
	var result []recordings.WorkerControlOperationRecord
	for _, history := range entry.operations {
		for _, record := range history {
			if record.Target == target {
				result = append(result, record.Detached())
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Operation.RequestID == result[j].Operation.RequestID {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Operation.RequestID < result[j].Operation.RequestID
	})
	return result, nil
}

func (entry *recordingEntry) validateControlTarget(target recordings.WorkerControlTarget) error {
	if entry.damaged {
		return recordings.ErrWorkerRecordingReplay
	}
	session := entry.sessions[target.WorkerSessionID]
	if session == nil || len(session.records) == 0 {
		return recordings.ErrInvalidWorkerControlOperation
	}
	if session.generation != target.RecordingGenerationID || session.ownerEpoch != target.OwnerEpoch {
		return recordings.ErrWorkerControlConflict
	}
	// Scope comes from the committed opening, not from a caller-supplied ID.
	var opening struct {
		Payload json.RawMessage `json:"payload"`
	}
	var payload struct {
		FactorySessionID string `json:"factorySessionId"`
	}
	if json.Unmarshal(session.records[0].Payload, &opening) != nil || json.Unmarshal(opening.Payload, &payload) != nil || payload.FactorySessionID != target.FactorySessionID {
		return recordings.ErrWorkerControlConflict
	}
	return nil
}

func (writer *FileWriter) appendControl(ctx context.Context, entry *recordingEntry, record recordings.WorkerControlOperationRecord) error {
	stamp := writer.clock.Now().UTC()
	delta := workerJournalEntry{Version: 2, Kind: "control-operation", RecordingID: record.Target.RecordingID, WorkerSessionID: record.Target.WorkerSessionID, RecordingGenerationID: record.Target.RecordingGenerationID, OwnerEpoch: record.Target.OwnerEpoch, CapturedAt: &stamp, ControlOperation: &record}
	if err := writer.append(ctx, entry, delta); err != nil {
		entry.controlUnsynced = true
		return err
	}
	entry.acceptControl(record)
	return nil
}

// An error after writing may leave complete but unsynchronized bytes. Reading
// them is insufficient acknowledgement: synchronize without appending a row.
func (writer *FileWriter) hydrateControls(ctx context.Context, id string, entry *recordingEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.controlUnsynced {
		if err := writer.appender.AppendFile(writer.path(id)+"l", nil); err != nil {
			return err
		}
		entry.controlUnsynced = false
	}
	return writer.hydrate(ctx, id, entry)
}

func (entry *recordingEntry) acceptControl(record recordings.WorkerControlOperationRecord) {
	if entry.operations == nil {
		entry.operations = make(map[string][]recordings.WorkerControlOperationRecord)
	}
	// Both indexes reference the same immutable admitted snapshot. Readers
	// detach it at the boundary, so indexing does not duplicate result payloads.
	record = record.Detached()
	entry.operations[record.Operation.RequestID] = append(entry.operations[record.Operation.RequestID], record)
	if entry.summaryOperations == nil {
		entry.summaryOperations = make(map[recordings.WorkerControlTarget][]recordings.WorkerControlOperationRecord)
	}
	entry.summaryOperations[record.Target] = append(entry.summaryOperations[record.Target], record)
}

func (delta workerJournalEntry) validateControlEnvelope(id string) error {
	if delta.Version != 2 {
		return recordings.ErrWorkerRecordingCompatibility
	}
	if delta.OwnerLoss != nil {
		return recordings.ErrWorkerRecordingReplay
	}
	return delta.validateControlPayload(id)
}

func (delta workerJournalEntry) validateControlPayload(id string) error {
	if delta.ControlOperation == nil || delta.Record != nil || delta.Topic != "" || delta.Code != "" || delta.ExecutionTerminal != nil || delta.CapturedAt == nil || delta.CapturedAt.IsZero() {
		return recordings.ErrWorkerRecordingReplay
	}
	record := *delta.ControlOperation
	if err := record.Validate(); err != nil {
		return err
	}
	if id != record.Target.RecordingID || delta.RecordingID != id || delta.WorkerSessionID != record.Target.WorkerSessionID || delta.RecordingGenerationID != record.Target.RecordingGenerationID || delta.OwnerEpoch != record.Target.OwnerEpoch {
		return recordings.ErrWorkerRecordingReplay
	}
	return nil
}

func (entry *recordingEntry) applyControlDelta(id string, delta workerJournalEntry) error {
	if err := delta.validateControlEnvelope(id); err != nil {
		return err
	}
	record := *delta.ControlOperation
	if err := entry.validateControlTarget(record.Target); err != nil {
		return err
	}
	history := entry.operations[record.Operation.RequestID]
	if len(history) == 0 {
		if err := record.ValidateIntent(); err != nil {
			return recordings.ErrWorkerRecordingReplay
		}
	} else if !history[len(history)-1].CanAdvance(record, history[len(history)-1].Revision) {
		return recordings.ErrWorkerRecordingReplay
	}
	entry.acceptControl(record)
	return nil
}
