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
	if page.Catalog.WorkerSessionID != id {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	if req.FactorySessionID != "" && page.Catalog.FactorySessionID != strings.TrimSpace(req.FactorySessionID) {
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	result, err := capturedHistoryIdentity(recordings.WorkerCapturedCatalogItem{
		Catalog: page.Catalog, Opening: page.Opening, Terminal: page.Terminal,
		Health: page.Health, HealthReason: page.HealthReason, OwnerLost: page.OwnerLost,
	}, nil)
	if err != nil {
		return workersessions.Observation{}, err
	}
	s.applyArchivedTerminalCause(ctx, page, result)
	err = s.archivedCapturedFacts(ctx, page, result)
	if err != nil {
		return workersessions.Observation{}, err
	}
	if page.SuccessorWorkerSessionID != "" {
		if result.SuccessorWorkerSessionID != "" && result.SuccessorWorkerSessionID != page.SuccessorWorkerSessionID {
			return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
		}
		result.SuccessorWorkerSessionID = page.SuccessorWorkerSessionID
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
		Provider: draft.Provenance.Provider,
		WorkIDs:  append([]string(nil), opening.WorkIDs...), AttemptID: opening.AttemptID, TurnID: opening.TurnID,
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

func (s *LogReader) archivedCapturedFacts(ctx context.Context, page recordings.WorkerCapturedActivityPage, observation *workersessions.Observation) error {
	// The source draft retains field presence, including explicit zeros.
	// Do not round-trip the page's numeric convenience projection.
	reader, ok := s.reader.(recordings.WorkerRecordingReader)
	if !ok {
		return nil
	}
	snapshot, err := reader.LoadWorkerRecording(ctx, page.Catalog.RecordingID)
	if err != nil || snapshot.RecordingID != page.Catalog.RecordingID {
		return workersessions.ErrObservationProjectionUnavailable
	}
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != observation.WorkerSessionID {
			continue
		}
		if session.RecordingGenerationID != "" && session.RecordingGenerationID != page.Catalog.RecordingGenerationID {
			return workersessions.ErrObservationProjectionUnavailable
		}
		for _, record := range session.Records {
			if uint64(record.ID.Position) > page.Catalog.CommittedPosition {
				continue
			}
			var draft workers.Draft
			if json.Unmarshal(record.Payload, &draft) == nil {
				applyCapturedSessionFacts(observation, draft)
				applyCapturedUsageFacts(observation, draft)
			}
		}
		applyCapturedTiming(observation, page.Terminal, page.Catalog.CommittedPosition, session.CapturedAt)
	}
	return observation.Validate()
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
