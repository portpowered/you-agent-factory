package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Group only calls already waiting for the same journal. There is no timer or
// asynchronous acknowledgement. A caller still returns only after durable sync.
const (
	workerCommitGroupLimit = 128
	workerCommitGroupBytes = 8 << 20
)

type pendingWorkerRecord struct {
	ctx          context.Context
	record       recordings.WorkerRecordingRecord
	complete     bool
	err          error
	requiresSync bool
}

type stagedWorkerRecord struct {
	session    *recordingSession
	projection recordings.WorkerRecordingProjection
	delta      workerJournalEntry
}

func (entry *recordingEntry) pendingGroup() []*pendingWorkerRecord {
	entry.pendingMu.Lock()
	count := min(len(entry.pending), workerCommitGroupLimit)
	bytes := 0
	for index, request := range entry.pending[:count] {
		size := len(request.record.Record.Payload) + 4096
		if index > 0 && bytes+size > workerCommitGroupBytes {
			count = index
			break
		}
		bytes += size
	}
	group := append([]*pendingWorkerRecord(nil), entry.pending[:count]...)
	clear(entry.pending[:count])
	entry.pending = entry.pending[count:]
	entry.pendingMu.Unlock()
	return group
}

func (writer *FileWriter) persistPendingRecords(ctx context.Context, entry *recordingEntry, id string) {
	group := entry.pendingGroup()
	defer func() {
		for _, request := range group {
			request.complete = true
		}
	}()
	// Hydration remains cancellable under the journal admission barrier.
	if err := writer.hydrate(ctx, id, entry); err != nil {
		for _, request := range group {
			request.err = err
		}
		return
	}
	if entry.damaged {
		for _, request := range group {
			request.err = recordings.ErrWorkerRecordingReplay
		}
		return
	}
	staged := writer.stagePendingRecords(entry, group)
	var data []byte
	for _, record := range staged {
		encoded, err := json.Marshal(record.delta)
		if err != nil {
			for _, request := range group {
				request.err = err
			}
			return
		}
		data = append(append(data, encoded...), '\n')
	}
	if len(data) == 0 {
		return
	}
	if err := writer.appender.AppendFile(writer.path(id)+"l", data); err != nil {
		// A close-after-sync failure can leave all bytes committed. Reload before
		// a retry, just like single-record admission; publish no uncertain prefix.
		entry.loaded = false
		for _, request := range group {
			if request.err == nil && request.requiresSync {
				request.err = fmt.Errorf("persist Worker recording group: %w", err)
			}
		}
		return
	}
	for _, record := range staged {
		record.session.acceptRecord(record.projection)
		record.session.acceptMetadata(record.delta)
		entry.commit(record.session)
		writer.indexSession(record.session)
	}
}

func (writer *FileWriter) stagePendingRecords(entry *recordingEntry, group []*pendingWorkerRecord) []stagedWorkerRecord {
	sessions := make(map[string]*recordingSession)
	projections := make(map[string]recordings.WorkerRecordingProjection)
	identities := make(map[string]map[events.AppendIdentity]events.Record)
	staged := make([]stagedWorkerRecord, 0, len(group))
	for _, request := range group {
		if request.err = request.ctx.Err(); request.err != nil {
			continue
		}
		record := request.record
		session := sessions[record.WorkerSessionID]
		if session == nil {
			session = entry.session(record.RecordingID, record.WorkerSessionID, record.Record.ID.Topic)
			sessions[record.WorkerSessionID] = session
			projections[record.WorkerSessionID] = session.projection
			identities[record.WorkerSessionID] = make(map[events.AppendIdentity]events.Record)
		}
		accepted, duplicate := session.identities[record.Record.Identity()]
		committedDuplicate := duplicate
		if !duplicate {
			accepted, duplicate = identities[record.WorkerSessionID][record.Record.Identity()]
		}
		if duplicate {
			if !sameRecord(accepted, record.Record) {
				request.err = recordings.ErrWorkerRecordingDuplicate
			} else if committedDuplicate {
				writer.indexSession(session)
			} else {
				request.requiresSync = true
			}
			continue
		}
		previous := projections[record.WorkerSessionID]
		if previous.Degradation == "OWNER_LOST" {
			request.err = recordings.ErrWorkerRecordingTerminal
			continue
		}
		projection, err := (recordings.WorkerRecordingCodec{}).AdvanceWorkerRecording(previous, record.Record)
		if err != nil {
			request.err = err
			continue
		}
		delta := workerJournalEntry{Version: 1, Kind: "record", RecordingID: record.RecordingID, WorkerSessionID: record.WorkerSessionID, Record: &record.Record}
		if previous.LastPosition == 0 {
			ownerEpoch := writer.captureOwnerEpoch()
			identity, _ := json.Marshal([]string{ownerEpoch, record.RecordingID, record.WorkerSessionID, string(record.Record.SourceEventID)})
			generation := sha256.Sum256(identity)
			delta.RecordingGenerationID = hex.EncodeToString(generation[:])
			delta.OwnerEpoch = ownerEpoch
		}
		stamp := writer.clock.Now().UTC()
		delta.CapturedAt = &stamp
		request.requiresSync = true
		staged = append(staged, stagedWorkerRecord{session, projection, delta})
		projections[record.WorkerSessionID] = projection
		identities[record.WorkerSessionID][record.Record.Identity()] = record.Record
	}
	return staged
}
