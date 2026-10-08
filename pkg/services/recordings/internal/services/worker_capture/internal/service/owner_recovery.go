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
	// Activation and the first catalog read share one scan and the same
	// hydrated, committed prefixes. A request must not start a second rebuild
	// while recovery is still deciding which owners can be fenced.
	writer.rebuildMu.Lock()
	defer writer.rebuildMu.Unlock()
	if writer.ownerDeaths == nil {
		writer.ownerDeaths = make(map[string]bool)
	}
	seen := make(map[string]bool)
	complete := true
	err := writer.directory.ScanDirectory(writer.root, 64, func(files []os.DirEntry) error {
		available, err := writer.recoverOwnerFiles(ctx, files, seen)
		complete = complete && available
		return err
	})
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err == nil {
		writer.ownerRecoveryDone = true
		// Unavailable files grant neither recovery authority nor proof of
		// catalog membership. Ordinary reads retry them without a restart.
		writer.catalogLoaded = complete
	}
	return err
}

func (writer *FileWriter) recoverOwnerFiles(ctx context.Context, files []os.DirEntry, seen map[string]bool) (bool, error) {
	complete := true
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if file.IsDir() || (!strings.HasSuffix(file.Name(), ".worker.jsonl") && !strings.HasSuffix(file.Name(), ".worker.json")) {
			continue
		}
		data, err := writer.storage.ReadFile(filepath.Join(writer.root, file.Name()))
		if err != nil {
			// An unavailable journal grants no recovery authority. Keep hosting
			// unrelated work; public reads retain their typed unavailable result
			// and may recover when storage becomes readable again.
			complete = false
			continue
		}
		id, err := writer.recordingFileIdentity(file.Name(), data)
		if err != nil {
			writer.markCatalogDamaged()
			continue // Unreadable identities grant no recovery authority.
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := writer.recoverRecordingOwner(ctx, id); err != nil {
			return false, err
		}
		// Recovery hydrated this entry behind the append barrier. Reuse it
		// to index ended sessions as well as newly fenced owners.
		if err := writer.rebuildRecordingIndex(ctx, id); err != nil {
			if canceled := ctx.Err(); canceled != nil {
				return false, canceled
			}
			writer.indexUnavailableCapture(file.Name(), data)
			// Hydration may fail at a transient read edge after the identity
			// read succeeds. Leave the catalog retryable in that case too.
			complete = false
		}
	}
	return complete, nil
}

func (writer *FileWriter) markCatalogDamaged() {
	writer.catalogMu.Lock()
	writer.catalogDamaged = true
	writer.catalogMu.Unlock()
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
