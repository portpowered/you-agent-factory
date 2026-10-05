package service

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Capture may lag live publication. Only a durable snapshot grants usage facts;
// a missing or unreadable snapshot leaves usage unknown without hiding identity.
func (r *registry) capturedObservationUsage(ctx context.Context, id string) *workersessions.TokenUsage {
	publication := r.publicationFor(id)
	if publication == nil {
		return nil
	}
	publication.mu.Lock()
	recordingID := publication.recordingID
	publication.mu.Unlock()
	if recordingID == "" {
		return nil
	}
	snapshot, err := r.LoadWorkerRecording(ctx, recordingID)
	if err != nil || snapshot.RecordingID != recordingID {
		return nil
	}
	return capturedSnapshotUsage(snapshot, id, ^uint64(0))
}

func capturedSnapshotUsage(snapshot recordings.WorkerRecordingSnapshot, id string, head uint64) *workersessions.TokenUsage {
	var usage *workersessions.TokenUsage
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != id {
			continue
		}
		for _, record := range session.Records {
			if uint64(record.ID.Position) > head {
				continue
			}
			var draft workers.Draft
			if json.Unmarshal(record.Payload, &draft) != nil {
				continue
			}
			if captured, _, ok := usageProjectionFromDraft(draft); ok {
				usage = captured
			}
		}
		break
	}
	return usage
}
