package runtime

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Canonical-ID summaries retain Factory lifecycle facts, but usage must come
// from the committed Worker capture rather than provider files or diagnostics.
func (s *recordedWorkerSessionObservation) withCapturedWorkerIdentity(ctx context.Context, observation workersessions.Observation) (workersessions.Observation, error) {
	observation.TokenUsage = nil
	if s.factorySessionID != "" {
		observation.FactorySessionID = s.factorySessionID
	}
	if s.recordingReader == nil || s.recordingID == "" {
		return observation, nil
	}
	snapshot, err := s.recordingReader.LoadWorkerRecording(ctx, s.recordingID)
	if err != nil {
		if healthErr := recordingHealthLoadError(err); healthErr != nil {
			return workersessions.Observation{}, healthErr
		}
		return observation, nil
	}
	health, err := workerRecordingHealthMap(snapshot, s.recordingID)
	if err != nil {
		return workersessions.Observation{}, err
	}
	if current, ok := health[observation.WorkerSessionID]; ok {
		observation.RecordingHealth = current.status
		observation.RecordingHealthReason = current.reason
		if current.startedAt != nil {
			started := *current.startedAt
			observation.StartedAt = &started
		}
	}
	observation.TokenUsage = capturedFactoryWorkerUsage(snapshot, observation.WorkerSessionID)
	return observation, nil
}

func capturedFactoryWorkerUsage(snapshot recordings.WorkerRecordingSnapshot, id string) *workersessions.TokenUsage {
	var usage *workersessions.TokenUsage
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != id {
			continue
		}
		for _, record := range session.Records {
			var draft workers.Draft
			if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated {
				continue
			}
			var captured workersessions.TokenUsage
			if json.Unmarshal(draft.Payload, &captured) != nil ||
				(captured.InputTokens == nil && captured.CachedInputTokens == nil && captured.OutputTokens == nil && captured.ReasoningOutputTokens == nil && captured.TotalTokens == nil) {
				continue
			}
			usage = &captured
		}
		break
	}
	return usage
}
