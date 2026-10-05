package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// A loss fact describes supervisor ownership, never provider/child death.
type ownerLossFact struct {
	RecoveryOwnerEpoch string `json:"recoveryOwnerEpoch"`
	ChildLiveness      string `json:"childLiveness"`
}

// RecoverWorkerOwners runs only during host activation, before exposing control
// or admission. Construction and ordinary reads never produce loss facts.
func (writer *FileWriter) RecoverWorkerOwners(ctx context.Context) error {
	writer.ownerRecoveryMu.Lock()
	defer writer.ownerRecoveryMu.Unlock()
	if writer.ownerRecoveryDone {
		return nil
	}
	if writer.ownerDeaths == nil {
		writer.ownerDeaths = make(map[string]bool)
	}
	seen := make(map[string]bool)
	err := writer.directory.ScanDirectory(writer.root, 64, func(files []os.DirEntry) error {
		return writer.recoverOwnerFiles(ctx, files, seen)
	})
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err == nil {
		writer.ownerRecoveryDone = true
	}
	return err
}

func (writer *FileWriter) recoverOwnerFiles(ctx context.Context, files []os.DirEntry, seen map[string]bool) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.IsDir() || (!strings.HasSuffix(file.Name(), ".worker.jsonl") && !strings.HasSuffix(file.Name(), ".worker.json")) {
			continue
		}
		data, err := writer.storage.ReadFile(filepath.Join(writer.root, file.Name()))
		if err != nil {
			return recordings.ErrWorkerRecordingReplay
		}
		id, err := writer.recordingFileIdentity(file.Name(), data)
		if err != nil || seen[id] {
			continue // Unreadable identities grant no recovery authority.
		}
		seen[id] = true
		if err := writer.recoverRecordingOwner(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (writer *FileWriter) recoverRecordingOwner(ctx context.Context, id string) error {
	entry := writer.entry(id)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, id, entry); err != nil {
		return nil // Corrupt journals remain unreadable; never append through them.
	}
	if entry.damaged {
		return nil
	}
	for _, session := range entry.sessions {
		if session.projection.ExecutionTerminal != nil || len(session.records) == 0 {
			continue
		}
		dead, queried := writer.ownerDeaths[session.ownerEpoch]
		if !queried {
			dead = writer.ownerDeathWitness(session.ownerEpoch)
			writer.ownerDeaths[session.ownerEpoch] = dead
		}
		if dead {
			if err := writer.persistOwnerLoss(ctx, entry, session); err != nil {
				return err
			}
			writer.indexSession(session)
		}
	}
	return ctx.Err()
}

func (writer *FileWriter) persistOwnerLoss(ctx context.Context, entry *recordingEntry, session *recordingSession) error {
	recoveryEpoch := writer.captureOwnerEpoch()
	if _, valid := decodeCaptureOwner(recoveryEpoch); !valid {
		return recordings.ErrWorkerControlConflict
	}
	stamp := writer.clock.Now().UTC()
	delta := workerJournalEntry{
		Version: 2, Kind: "owner-loss", RecordingID: session.projection.RecordingID,
		WorkerSessionID: session.projection.WorkerSessionID, RecordingGenerationID: session.generation,
		OwnerEpoch: session.ownerEpoch, CapturedAt: &stamp,
		OwnerLoss: &ownerLossFact{RecoveryOwnerEpoch: recoveryEpoch, ChildLiveness: "UNKNOWN"},
	}
	if err := writer.append(ctx, entry, delta); err != nil {
		return err
	}
	return entry.applyOwnerLoss(delta.RecordingID, delta)
}

func (entry *recordingEntry) applyOwnerLoss(id string, delta workerJournalEntry) error {
	if err := delta.validateOwnerLoss(id); err != nil {
		return err
	}
	session := entry.sessions[delta.WorkerSessionID]
	if session == nil || session.generation != delta.RecordingGenerationID || session.ownerEpoch != delta.OwnerEpoch || session.projection.ExecutionTerminal != nil {
		return recordings.ErrWorkerRecordingReplay
	}
	// Keep the captured prefix and its watermark intact: this is not a worker
	// callback, stop acknowledgement or fabricated source-native event.
	p, err := (recordings.WorkerRecordingCodec{}).FailWorkerRecording(session.projection, "OWNER_LOST",
		&recordings.WorkerRecordingTerminal{Phase: workers.PhaseFailed, Status: "FAILED"})
	if err != nil {
		return err
	}
	session.projection = p
	entry.commit(session)
	return nil
}

func (delta workerJournalEntry) validateOwnerLoss(id string) error {
	if delta.Version != 2 || delta.RecordingID != id || delta.CapturedAt == nil || delta.CapturedAt.IsZero() ||
		delta.OwnerLoss == nil || delta.OwnerLoss.ChildLiveness != "UNKNOWN" || delta.Record != nil ||
		delta.ControlOperation != nil || delta.Topic != "" || delta.Code != "" || delta.ExecutionTerminal != nil {
		return recordings.ErrWorkerRecordingReplay
	}
	return delta.validateLossIncarnations()
}

func (delta workerJournalEntry) validateLossIncarnations() error {
	prior, valid := decodeCaptureOwner(delta.OwnerEpoch)
	if !valid {
		return recordings.ErrWorkerRecordingReplay
	}
	recovery, valid := decodeCaptureOwner(delta.OwnerLoss.RecoveryOwnerEpoch)
	if !valid || recovery.Process.Host != prior.Process.Host || recovery.Process == prior.Process {
		return recordings.ErrWorkerRecordingReplay
	}
	return nil
}
