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
	page, err := s.reader.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: id, Limit: 1})
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
	if page.Catalog.WorkerSessionID != id || page.Terminal == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	result, err := capturedTerminalIdentity(page, id)
	if err != nil {
		return workersessions.Observation{}, err
	}
	result.TokenUsage, err = s.archivedCapturedUsage(ctx, page, id)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return result.Clone(), nil
}

func capturedTerminalIdentity(page recordings.WorkerCapturedActivityPage, id string) (workersessions.Observation, error) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(page.Opening.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil || opening.WorkerSessionID != id {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	result := workersessions.Observation{
		WorkerSessionID: id, Direct: opening.FactorySessionID == "", FactorySessionID: opening.FactorySessionID,
		WorkIDs: append([]string(nil), opening.WorkIDs...), AttemptID: opening.AttemptID, TurnID: opening.TurnID,
		State: workersessions.State(page.Terminal.Status), StartedAt: opening.StartedAt,
		ConfirmationState: workersessions.ConfirmationStateConfirmed,
		DurationBasis:     workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable,
		RecordingHealth: page.Health, RecordingHealthReason: page.HealthReason,
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

func (s *LogReader) archivedCapturedUsage(ctx context.Context, page recordings.WorkerCapturedActivityPage, id string) (*workersessions.TokenUsage, error) {
	if page.TokenUsage == nil {
		return nil, nil
	}
	// The source draft retains field presence, including explicit zeros.
	// Do not round-trip the page's numeric convenience projection.
	reader, ok := s.reader.(recordings.WorkerRecordingReader)
	if !ok {
		return nil, nil
	}
	snapshot, err := reader.LoadWorkerRecording(ctx, page.Catalog.RecordingID)
	if err != nil || snapshot.RecordingID != page.Catalog.RecordingID {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	return capturedSnapshotUsage(snapshot, id, page.Catalog.CommittedPosition), nil
}
