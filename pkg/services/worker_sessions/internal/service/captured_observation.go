package service

import (
	"context"
	"encoding/json"

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
	var usage *workersessions.TokenUsage
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != id {
			continue
		}
		for _, record := range session.Records {
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
