package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// LookupWorkerSessionSummary carries the same activated capture source across
// the runtime observation adapter; it never falls back to a recording load.
func (r *registry) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	if r == nil || r.recording == nil {
		return recordings.WorkerCapturedSummary{}, recordings.ErrMissingWorkerRecordingReader
	}
	reader, ok := r.recording.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return recordings.WorkerCapturedSummary{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LookupWorkerSessionSummary(ctx, id)
}

// CurrentWorkerRecordingHealth preserves the Recordings-owned selected health
// capability for runtime observation readers using this same capture service.
func (r *registry) CurrentWorkerRecordingHealth(ctx context.Context, recordingID string, workerIDs []string) (recordings.WorkerRecordingSnapshot, error) {
	if r == nil || r.recording == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	reader, ok := r.recording.(recordings.WorkerRecordingHealthReader)
	if !ok {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.CurrentWorkerRecordingHealth(ctx, recordingID, workerIDs)
}

// GetObservationByWorkerSessionID reads archived identity without restoring a
// registry, execution authority, or a provider-native transcript reader.
func (s *LogReader) GetObservationByWorkerSessionID(ctx context.Context, req workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Observation{}, err
	}
	if ctx == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	id := strings.TrimSpace(req.WorkerSessionID)
	reader, ok := s.reader.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, id)
	item := summary.Capture
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return workersessions.Observation{}, err
		default:
			return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
		}
	}
	// A historical prefix cannot prove that an execution is still running.
	if item.Catalog.WorkerSessionID != id {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	if req.FactorySessionID != "" && item.Catalog.FactorySessionID != strings.TrimSpace(req.FactorySessionID) {
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	result, err := capturedHistoryIdentity(item, nil)
	if err != nil {
		return workersessions.Observation{}, err
	}
	applySummaryTerminalCause(summary, result)
	return result.Clone(), nil
}

func capturedTerminalIdentity(page recordings.WorkerCapturedActivityPage, id string) (workersessions.Observation, error) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(page.Opening.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil || opening.WorkerSessionID != id {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	metadata, err := decodeSessionMetadata(opening.SessionMetadata)
	if err != nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	result := workersessions.Observation{
		WorkerSessionID: id, Direct: opening.FactorySessionID == "", FactorySessionID: opening.FactorySessionID,
		Provider: draft.Provenance.Provider,
		WorkIDs:  append([]string(nil), opening.WorkIDs...), AttemptID: opening.AttemptID, TurnID: opening.TurnID,
		State: workersessions.State(page.Terminal.Status), StartedAt: opening.StartedAt,
		ConfirmationState: workersessions.ConfirmationStateConfirmed,
		DurationBasis:     workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable,
		RecordingHealth: page.Health, RecordingHealthReason: page.HealthReason,
	}
	if metadata != nil {
		result.Requester, result.Correlation, result.Labels = metadata.Requester, metadata.Correlation, metadata.Labels
	}
	if opening.Model != "" {
		result.Model = &opening.Model
	}
	if opening.ReasoningEffort != "" {
		result.ReasoningEffort = &opening.ReasoningEffort
	}
	if opening.Lineage != nil {
		result.PredecessorWorkerSessionID = opening.Lineage.PredecessorWorkerSessionID
		result.SuccessorWorkerSessionID = opening.Lineage.SuccessorWorkerSessionID
	}
	if !result.State.Terminal() || result.Validate() != nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return result.Clone(), nil
}

// Provider facts enter the live projection only after their lifecycle record
// has been accepted. This never creates a Provider Session association.
func (r *registry) rememberObservationProvider(id, provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if metadata := r.observations[id]; metadata != nil {
		metadata.provider = provider
	}
}
