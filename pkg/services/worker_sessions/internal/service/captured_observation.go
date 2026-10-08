package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// The terminal stamp measures host capture completion, not provider completion.
// Missing legacy stamps and terminals outside the committed prefix stay unknown.
func applyCapturedTiming(observation *workersessions.Observation, terminal *recordings.WorkerRecordingTerminal, head uint64, stamps map[string]time.Time) {
	if terminal == nil || terminal.Position < 1 || uint64(terminal.Position) > head || observation.StartedAt == nil {
		return
	}
	ended, ok := stamps[strconv.FormatInt(int64(terminal.Position), 10)]
	if !ok || ended.IsZero() || ended.Before(*observation.StartedAt) {
		return
	}
	duration := ended.Sub(*observation.StartedAt)
	observation.EndedAt = &ended
	observation.Duration = &duration
	observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
}

func applyCapturedUsageFacts(observation *workersessions.Observation, draft workers.Draft) {
	if draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated ||
		(draft.DispatchID != "" && draft.DispatchID != observation.AttemptID) {
		return
	}
	if usage, _, ok := usageProjectionFromDraft(draft); ok {
		observation.TokenUsage = usage
	}
	var payload workers.UsagePayload
	if json.Unmarshal(draft.Payload, &payload) == nil && payload.Model != "" {
		observation.Model = &payload.Model
	}
}

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
	return capturedSnapshotUsage(snapshot, publicWorkerID(id), ^uint64(0))
}

func capturedSnapshotUsage(snapshot recordings.WorkerRecordingSnapshot, id string, head uint64) *workersessions.TokenUsage {
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID == id {
			return capturedSessionUsage(session, head)
		}
	}
	return nil
}

func capturedSessionUsage(session recordings.WorkerSessionRecordingSnapshot, head uint64) *workersessions.TokenUsage {
	var usage *workersessions.TokenUsage
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
	return usage
}

// A list owns its detached snapshot for this request only. Loading the entire
// recording for each row repeats copies of every sibling's accumulated history.
// Group by recording, reduce each selected worker once, and never reuse the
// result across requests: the next list must observe new committed facts.
func (r *registry) capturedListUsage(ctx context.Context, ids []observationOrder) map[string]*workersessions.TokenUsage {
	groups := make(map[string]map[string]string)
	usage := make(map[string]*workersessions.TokenUsage)
	for _, item := range ids {
		pub := r.publicationFor(item.id)
		if pub == nil {
			continue
		}
		usage[item.id] = nil // An unavailable capture must clear live usage.
		pub.mu.Lock()
		recordingID := pub.recordingID
		pub.mu.Unlock()
		if recordingID == "" {
			continue
		}
		if groups[recordingID] == nil {
			groups[recordingID] = make(map[string]string)
		}
		groups[recordingID][publicWorkerID(item.id)] = item.id
	}
	for recordingID, selected := range groups {
		snapshot, err := r.LoadWorkerRecording(ctx, recordingID)
		if err != nil || snapshot.RecordingID != recordingID {
			continue
		}
		for _, session := range snapshot.Sessions {
			id, exists := selected[session.WorkerSessionID]
			if !exists {
				continue
			}
			usage[id] = capturedSessionUsage(session, ^uint64(0))
			delete(selected, session.WorkerSessionID) // Preserve first-match semantics.
		}
	}
	return usage
}
