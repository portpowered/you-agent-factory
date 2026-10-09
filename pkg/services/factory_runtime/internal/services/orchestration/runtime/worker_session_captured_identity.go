package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Scoped lists select only matching capture health from the activated store.
// An empty selection still checks the selected recording's prepared health.
func (s *recordedWorkerSessionObservation) selectedRecordingHealth(ctx context.Context, recorded, live []workersessions.Observation) (map[string]workerRecordingHealth, error) {
	if s.recordingReader == nil || s.recordingID == "" {
		return nil, nil
	}
	reader, ok := s.recordingReader.(recordings.WorkerRecordingHealthReader)
	if !ok {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	ids := make([]string, 0, len(recorded)+len(live))
	seen := make(map[string]bool, cap(ids))
	for _, rows := range [][]workersessions.Observation{recorded, live} {
		for _, row := range rows {
			if !seen[row.WorkerSessionID] {
				seen[row.WorkerSessionID] = true
				ids = append(ids, row.WorkerSessionID)
			}
		}
	}
	snapshot, err := reader.CurrentWorkerRecordingHealth(ctx, s.recordingID, ids)
	if err != nil {
		return nil, recordingHealthLoadError(err)
	}
	if err := observationContextError(ctx); err != nil {
		return nil, err
	}
	for _, session := range snapshot.Sessions {
		if !seen[session.WorkerSessionID] {
			return nil, workersessions.ErrObservationRecordingCorrupt
		}
	}
	return workerRecordingHealthMap(snapshot, s.recordingID)
}

// Canonical-ID summaries retain Factory lifecycle facts, but usage must come
// from the committed Worker capture rather than provider files or diagnostics.
func (s *recordedWorkerSessionObservation) withCapturedWorkerIdentity(ctx context.Context, observation workersessions.Observation) (workersessions.Observation, error) {
	observation.TokenUsage = nil
	if s.factorySessionID != "" {
		observation.FactorySessionID = s.factorySessionID
	}
	if observation.State == workersessions.StateCanceled && s.Service != nil {
		archived, found, err := archivedFactoryWorker(ctx, s.Service, observation.FactorySessionID, observation.WorkerSessionID)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if found && archived.State == workersessions.StateTerminated && archived.TerminalCause != nil && *archived.TerminalCause == "OPERATOR_KILL" {
			observation = archived
		}
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

// Replay never reacquires the recorded owner. Recover only the exact committed
// kill disposition from Worker Sessions' archived observation boundary.
func (cfg *runtimeConfig) recoverReplayForce(ctx context.Context, dispatch work.WorkDispatch) error {
	resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver)
	if !ok || cfg.workerSessions == nil {
		return nil
	}
	id, found := resolver.WorkerSessionIDForDispatch(dispatch)
	if !found {
		return nil
	}
	observation, found, err := archivedFactoryWorker(ctx, cfg.workerSessions, canonicalSessionIDFromFactoryConfig(cfg), id)
	if err != nil {
		return fmt.Errorf("recover recorded force disposition for dispatch %s: %w", dispatch.DispatchID, err)
	}
	if found && observation.State == workersessions.StateTerminated && observation.TerminalCause != nil && *observation.TerminalCause == "OPERATOR_KILL" {
		cfg.attempts.recordConfirmedForce(dispatch.DispatchID)
	}
	return nil
}

func archivedFactoryWorker(ctx context.Context, service workersessions.Service, sessionID, workerID string) (workersessions.Observation, bool, error) {
	observation, err := service.GetCapturedObservation(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerID, FactorySessionID: sessionID})
	if errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		return workersessions.Observation{}, false, nil
	}
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	if observation.WorkerSessionID != workerID || observation.FactorySessionID != sessionID {
		return workersessions.Observation{}, false, workersessions.ErrObservationProjectionUnavailable
	}
	return observation.Clone(), true, nil
}
