package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/events"
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

// Canonical associations establish physical membership, including restored
// attempts rebound to the current Factory Session. Capture is selected by that
// identity and recording, and contributes only committed usage/kill facts.
// Health and confirmation retain their request-owned samples in ListObservations.
func (s *recordedWorkerSessionObservation) withSelectedCapturedIdentity(ctx context.Context, observation workersessions.Observation) (workersessions.Observation, error) {
	observation.TokenUsage = nil
	if s.factorySessionID != "" {
		observation.FactorySessionID = s.factorySessionID
	}
	if observation.State == workersessions.StateCanceled && s.Service != nil {
		archived, found, err := s.selectedCapturedCancellation(ctx, observation)
		if err != nil || found {
			return archived, err
		}
	}
	if s.recordingReader == nil || s.recordingID == "" {
		return observation, nil
	}
	reader, ok := s.recordingReader.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, observation.WorkerSessionID)
	if failure := observationContextError(ctx); failure != nil {
		return workersessions.Observation{}, failure
	}
	if err != nil {
		if failure := recordingHealthLoadError(err); failure != nil {
			return workersessions.Observation{}, failure
		}
		return observation, nil
	}
	item := summary.Capture
	restoredScope := s.restoredWorkerScopes[observation.WorkerSessionID]
	if !s.selectedCaptureMatches(observation.WorkerSessionID, item.Catalog) {
		return workersessions.Observation{}, workersessions.ErrObservationRecordingCorrupt
	}
	if restoredScope != "" {
		observation, err = s.withRestoredCaptureHealth(observation, item)
		if err != nil {
			return workersessions.Observation{}, err
		}
	}
	observation.TokenUsage = capturedWorkerUsageRecords(item.MetadataRecords, item.Catalog.CommittedPosition)
	return observation, nil
}

func (s *recordedWorkerSessionObservation) selectedCaptureMatches(workerID string, catalog recordings.WorkerSessionCatalogEntry) bool {
	if catalog.WorkerSessionID != workerID {
		return false
	}
	if restoredScope := s.restoredWorkerScopes[workerID]; restoredScope != "" {
		return catalog.FactorySessionID == restoredScope
	}
	return catalog.RecordingID == s.recordingID
}

// A restored canonical association authorizes this exact physical capture in
// its original scope. The summary has already validated its committed opening.
func (s *recordedWorkerSessionObservation) withRestoredCaptureHealth(observation workersessions.Observation, item recordings.WorkerCapturedCatalogItem) (workersessions.Observation, error) {
	health, err := workerRecordingHealthMap(recordings.WorkerRecordingSnapshot{
		RecordingID: item.Catalog.RecordingID,
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: observation.WorkerSessionID, Status: item.Health,
			Failure: item.HealthReason, InterruptionReason: item.HealthReason,
			Records: []events.Record{item.Opening},
		}},
	}, item.Catalog.RecordingID)
	if err != nil {
		return workersessions.Observation{}, err
	}
	rows := []workersessions.Observation{observation}
	s.decorateRecordingHealth(rows, health)
	return rows[0], nil
}

func (s *recordedWorkerSessionObservation) selectedCapturedCancellation(ctx context.Context, observation workersessions.Observation) (workersessions.Observation, bool, error) {
	scope := observation.FactorySessionID
	if restored := s.restoredWorkerScopes[observation.WorkerSessionID]; restored != "" {
		scope = restored
	}
	archived, found, err := archivedFactoryWorker(ctx, s.Service, scope, observation.WorkerSessionID)
	if failure := observationContextError(ctx); failure != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return workersessions.Observation{}, false, workersessions.ErrObservationCanceled
	}
	if err != nil || !found {
		return workersessions.Observation{}, false, err
	}
	observation.TokenUsage = cloneRecordedTokenUsage(archived.TokenUsage)
	if archived.State == workersessions.StateTerminated && archived.TerminalCause != nil && *archived.TerminalCause == "OPERATOR_KILL" {
		archived.FactorySessionID = observation.FactorySessionID
		return archived, true, nil
	}
	return observation, true, nil
}

// Optional activity reads use exact physical identity and the same list budget.
// Missing, failed or slow transcript capture leaves committed facts intact.
func (s *recordedWorkerSessionObservation) withSelectedCapturedTranscript(ctx, optionalCtx context.Context, observation workersessions.Observation) (workersessions.Observation, error) {
	if s.Service == nil || !observation.State.Terminal() || !observation.ProviderSessionAvailable || optionalCtx.Err() != nil {
		return observation, observationContextError(ctx)
	}
	scope := s.executionFactorySessionID
	if restored := s.restoredWorkerScopes[observation.WorkerSessionID]; restored != "" {
		scope = restored
	}
	if scope == "" {
		scope = observation.FactorySessionID
	}
	transcript, err := s.ReadTranscriptByWorkerSessionID(optionalCtx, workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: observation.WorkerSessionID, FactorySessionID: scope})
	if failure := observationContextError(ctx); failure != nil {
		return workersessions.Observation{}, failure
	}
	if optionalCtx.Err() == nil && (errors.Is(err, workersessions.ErrObservationCanceled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return workersessions.Observation{}, workersessions.ErrObservationCanceled
	}
	if err == nil && optionalCtx.Err() == nil && selectedCapturedTranscriptMatches(transcript, observation) {
		observation.Transcript = workersessions.TranscriptAvailabilityAvailable
	}
	return observation, nil
}

func selectedCapturedTranscriptMatches(transcript workersessions.ReadTranscriptResult, observation workersessions.Observation) bool {
	return transcript.WorkerSessionID == observation.WorkerSessionID && transcript.ProviderSession == observation.ProviderSession && transcript.AttemptID == observation.AttemptID && transcript.State == observation.State
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
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID == id {
			return capturedWorkerUsageRecords(session.Records, ^uint64(0))
		}
	}
	return nil
}

func capturedWorkerUsageRecords(records []events.Record, head uint64) *workersessions.TokenUsage {
	var usage *workersessions.TokenUsage
	for _, record := range records {
		if uint64(record.ID.Position) > head || (head != ^uint64(0) && record.ID.Position < 1) {
			continue
		}
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
