package service

import (
	"context"
	"maps"
	"time"
)

// Read commit metadata once for a finite replay. Source records retain their
// existing Events identity; missing legacy times and uncommitted records stay
// unknown. This optional metadata read does not change stream availability.
func (r *registry) replayCaptureTimes(ctx context.Context, id string) map[string]time.Time {
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
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID == id {
			return maps.Clone(session.CapturedAt)
		}
	}
	return nil
}
