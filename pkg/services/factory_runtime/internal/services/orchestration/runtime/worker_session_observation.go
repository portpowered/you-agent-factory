package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func newRecordedWorkerSessionObservation(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
) workersessions.Service {
	return newRecordedWorkerSessionObservationWithRecording(live, ledger, projector, clock, providerSessions, "", nil)
}

func (s *recordedWorkerSessionObservation) ListObservations(
	ctx context.Context,
	req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if s == nil || s.ledger == nil || s.projector == nil {
		return s.listLive(ctx, req)
	}

	events := s.ledger.CanonicalEvents()
	recorded, knownWork, err := s.projectRecorded(ctx, events, req.WorkID)
	if err != nil {
		return workersessions.ListObservationsResult{}, err
	}

	live, liveErr := s.listLive(ctx, req)
	if liveErr == nil {
		recorded = mergeRecordedObservations(recorded, live.Observations)
	}
	if !acceptableLiveObservationError(liveErr) {
		return workersessions.ListObservationsResult{}, liveErr
	}
	if err := s.applyRecordingHealth(ctx, recorded); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	return recordedObservationListResult(recorded, knownWork, live, liveErr)
}

func acceptableLiveObservationError(err error) bool {
	return err == nil || isObservationNotFound(err) || isObservationProjectionUnavailable(err)
}

func recordedObservationListResult(
	recorded []workersessions.Observation,
	knownWork bool,
	live workersessions.ListObservationsResult,
	liveErr error,
) (workersessions.ListObservationsResult, error) {
	if !knownWork && len(recorded) == 0 {
		if liveErr == nil && len(live.Observations) > 0 {
			return live, nil
		}
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}
	if len(recorded) == 0 && liveErr == nil && len(live.Observations) > 0 {
		return live, nil
	}
	sortObservationAttempts(recorded)
	return workersessions.ListObservationsResult{Observations: recorded}, nil
}

func (s *recordedWorkerSessionObservation) listLive(
	ctx context.Context,
	req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	if s == nil || s.Service == nil {
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.Service.ListObservations(ctx, req)
}

type workerRecordingHealth struct {
	status recordings.WorkerRecordingStatus
	reason string
}

func (s *recordedWorkerSessionObservation) recordingHealth(
	ctx context.Context,
) (map[string]workerRecordingHealth, error) {
	if s == nil || s.recordingReader == nil || s.recordingID == "" {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := observationContextError(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.recordingReader.LoadWorkerRecording(ctx, s.recordingID)
	if err != nil {
		return nil, recordingHealthLoadError(err)
	}
	return workerRecordingHealthMap(snapshot, s.recordingID)
}

func recordingHealthLoadError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return workersessions.ErrObservationCanceled
	case errors.Is(err, os.ErrNotExist):
		return nil
	case isCorruptWorkerRecordingError(err):
		return fmt.Errorf("%w: %v", workersessions.ErrObservationRecordingCorrupt, err)
	default:
		return fmt.Errorf("%w: %v", workersessions.ErrObservationRecordingUnavailable, err)
	}
}

func workerRecordingHealthMap(
	snapshot recordings.WorkerRecordingSnapshot,
	recordingID string,
) (map[string]workerRecordingHealth, error) {
	if snapshot.RecordingID != recordingID {
		return nil, fmt.Errorf("%w: recording identity %q does not match %q", workersessions.ErrObservationRecordingCorrupt, snapshot.RecordingID, recordingID)
	}
	if len(snapshot.Sessions) == 0 {
		return nil, fmt.Errorf("%w: recording contains no Worker Session history", workersessions.ErrObservationRecordingCorrupt)
	}
	health := make(map[string]workerRecordingHealth, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		workerSessionID := strings.TrimSpace(session.WorkerSessionID)
		if workerSessionID == "" {
			return nil, fmt.Errorf("%w: recording contains an empty Worker Session identity", workersessions.ErrObservationRecordingCorrupt)
		}
		if _, exists := health[workerSessionID]; exists {
			return nil, fmt.Errorf("%w: recording contains duplicate Worker Session %q", workersessions.ErrObservationRecordingCorrupt, workerSessionID)
		}
		if !validWorkerRecordingHealth(session.Status) {
			return nil, fmt.Errorf("%w: Worker Session %q has invalid health %q", workersessions.ErrObservationRecordingCorrupt, workerSessionID, session.Status)
		}
		health[workerSessionID] = workerRecordingHealth{
			status: session.Status,
			reason: recordingHealthReason(session.Status, session.Failure, session.InterruptionReason),
		}
	}
	return health, nil
}

func validWorkerRecordingHealth(status recordings.WorkerRecordingStatus) bool {
	switch status {
	case recordings.WorkerRecordingStatusComplete,
		recordings.WorkerRecordingStatusDegraded,
		recordings.WorkerRecordingStatusIncomplete:
		return true
	}
	return false
}

func recordingHealthReason(status recordings.WorkerRecordingStatus, failure, interruption string) string {
	if status == recordings.WorkerRecordingStatusDegraded {
		return strings.TrimSpace(failure)
	}
	if status == recordings.WorkerRecordingStatusIncomplete {
		return strings.TrimSpace(interruption)
	}
	return ""
}

func isCorruptWorkerRecordingError(err error) bool {
	return errors.Is(err, recordings.ErrWorkerRecordingReplay) ||
		errors.Is(err, recordings.ErrWorkerRecordingCompatibility) ||
		errors.Is(err, recordings.ErrWorkerRecordingOrder) ||
		errors.Is(err, recordings.ErrWorkerRecordingDuplicate) ||
		errors.Is(err, recordings.ErrWorkerRecordingTerminal) ||
		errors.Is(err, recordings.ErrWorkerRecordingOpening) ||
		errors.Is(err, recordings.ErrWorkerRecordingDelivery)
}

func (s *recordedWorkerSessionObservation) withRecordingHealth(
	ctx context.Context,
	observation workersessions.Observation,
) (workersessions.Observation, error) {
	health, err := s.recordingHealth(ctx)
	if err != nil {
		return workersessions.Observation{}, err
	}
	if current, ok := health[observation.WorkerSessionID]; ok {
		observation.RecordingHealth = current.status
		observation.RecordingHealthReason = current.reason
	}
	return observation, nil
}

func (s *recordedWorkerSessionObservation) applyRecordingHealth(
	ctx context.Context,
	observations []workersessions.Observation,
) error {
	health, err := s.recordingHealth(ctx)
	if err != nil {
		return err
	}
	for index := range observations {
		if current, ok := health[observations[index].WorkerSessionID]; ok {
			observations[index].RecordingHealth = current.status
			observations[index].RecordingHealthReason = current.reason
		}
	}
	return nil
}

func (s *recordedWorkerSessionObservation) validateRecordingHealth(ctx context.Context) error {
	_, err := s.recordingHealth(ctx)
	return err
}

func (s *recordedWorkerSessionObservation) GetObservation(
	ctx context.Context,
	req workersessions.GetObservationRequest,
) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Observation{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		fact, found, err := s.recordedObservationForProvider(ctx, req.ProviderSession)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if found {
			observation, enrichErr := s.enrichRecordedObservation(ctx, recordedObservationFromFact(fact, s.clock), req.ProviderSession)
			if enrichErr != nil {
				return workersessions.Observation{}, enrichErr
			}
			return s.withRecordingHealth(ctx, observation)
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	observation, err := s.Service.GetObservation(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.withRecordingHealth(ctx, observation)
}

// GetObservationByWorkerSessionID resolves the canonical Worker Session
// identity against this Factory Session's durable event history before
// consulting the process-local registry. The wrapper is deliberately
// Factory-Session scoped, so an identical Worker Session ID in another
// session cannot leak into this response.
func (s *recordedWorkerSessionObservation) GetObservationByWorkerSessionID(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Observation{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		fact, found, err := s.recordedObservationForWorkerSessionID(ctx, req.WorkerSessionID)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if found {
			observation := recordedObservationFromFact(fact, s.clock)
			if fact.provider == nil {
				return s.withRecordingHealth(ctx, observation)
			}
			observation, enrichErr := s.enrichRecordedObservation(ctx, observation, providerSessionRef(*fact.provider))
			if enrichErr != nil {
				return workersessions.Observation{}, enrichErr
			}
			return s.withRecordingHealth(ctx, observation)
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	observation, err := s.Service.GetObservationByWorkerSessionID(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.withRecordingHealth(ctx, observation)
}

func (s *recordedWorkerSessionObservation) ReadTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		result, handled, err := s.readRecordedTranscriptForRequest(ctx, req)
		if handled || err != nil {
			return result, err
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	result, err := s.Service.ReadTranscript(ctx, req)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if err := s.validateRecordingHealth(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	return result, nil
}

func (s *recordedWorkerSessionObservation) readRecordedTranscriptForRequest(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, bool, error) {
	var fact recordedDispatchObservation
	var found bool
	var err error
	if req.WorkerSessionID != "" {
		fact, found, err = s.recordedObservationForWorkerSessionID(ctx, req.WorkerSessionID)
	} else {
		fact, found, err = s.recordedObservationForProvider(ctx, req.ProviderSession)
	}
	if err != nil {
		return workersessions.ReadTranscriptResult{}, true, err
	}
	if !found {
		if s.Service == nil {
			return workersessions.ReadTranscriptResult{}, true, workersessions.ErrObservationSessionNotFound
		}
		return workersessions.ReadTranscriptResult{}, false, nil
	}
	if err := s.validateRecordingHealth(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, true, err
	}
	if !fact.state.Terminal() {
		return workersessions.ReadTranscriptResult{}, true, workersessions.ErrObservationTranscriptActive
	}
	if fact.provider == nil {
		return workersessions.ReadTranscriptResult{}, true, workersessions.ErrObservationTranscriptUnavailable
	}
	readRequest := req
	if readRequest.WorkerSessionID != "" {
		readRequest = workersessions.ReadTranscriptRequest{ProviderSession: providerSessionRef(*fact.provider)}
	}
	result, err := s.readRecordedTranscript(ctx, readRequest, fact)
	return result, true, err
}

func (s *recordedWorkerSessionObservation) enrichRecordedObservation(
	ctx context.Context,
	observation workersessions.Observation,
	ref providers.SessionRef,
) (workersessions.Observation, error) {
	if s.Service != nil {
		live, err := s.Service.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: ref})
		if err == nil {
			merged := mergeRecordedObservations([]workersessions.Observation{observation}, []workersessions.Observation{live})
			if len(merged) == 1 {
				return merged[0], nil
			}
		}
		if errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.Observation{}, err
		}
	}
	if s.providerSessions == nil || !observation.ProviderSessionAvailable {
		return observation, nil
	}
	projected, err := s.providerSessions.Project(providersessions.ProjectRequest{
		Session: ref.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.Observation{}, workersessions.ErrObservationCanceled
		}
		return observation, nil
	}
	applyRecordedProviderDetail(&observation, projected.Detail)
	return observation, nil
}

func (s *recordedWorkerSessionObservation) readRecordedTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
	fact recordedDispatchObservation,
) (workersessions.ReadTranscriptResult, error) {
	if fact.provider == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	if s.Service != nil {
		live, err := s.Service.ReadTranscript(ctx, req)
		if err == nil {
			return historicalTranscriptResult(fact, live.Entries, req.ProviderSession)
		}
		if errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.ReadTranscriptResult{}, err
		}
	}
	if s.providerSessions == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	projected, err := s.providerSessions.Project(providersessions.ProjectRequest{
		Session: req.ProviderSession.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationCanceled
		}
		if recordedTranscriptSourceUnavailable(err) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("%w: %v", workersessions.ErrObservationTranscriptProjectionUnavailable, err)
	}
	return historicalTranscriptResult(fact, recordedTranscriptEntries(projected.Detail.Transcript), req.ProviderSession)
}

func historicalTranscriptResult(
	fact recordedDispatchObservation,
	entries []workersessions.TranscriptEntry,
	ref providers.SessionRef,
) (workersessions.ReadTranscriptResult, error) {
	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: fact.workerSessionID,
		ProviderSession: ref.Clone(),
		WorkIDs:         append([]string(nil), fact.workIDs...),
		TurnID:          fact.turnID,
		AttemptID:       fact.dispatchID,
		State:           fact.state,
		Entries:         entries,
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate historical Worker Session transcript: %w", err)
	}
	return result, nil
}
func newRecordedWorkerSessionObservationWithRecording(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
	recordingID string,
	recordingReader recordings.WorkerRecordingReader,
) workersessions.Service {
	return &recordedWorkerSessionObservation{
		Service: live, ledger: ledger, projector: projector, clock: clock,
		providerSessions: providerSessions, recordingID: strings.TrimSpace(recordingID),
		recordingReader: recordingReader,
	}
}
