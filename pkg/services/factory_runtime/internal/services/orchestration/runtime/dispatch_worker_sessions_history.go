package runtime

import (
	"context"
	"errors"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// GetObservationByWorkerSessionID resolves the Worker Session against this
// Factory Session's durable history before consulting the process-local registry.
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
	if observation, handled, err := s.readWorkerSessionHistory(ctx, req); handled {
		return observation, err
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.readLiveWorkerSessionByID(ctx, req)
}

func (s *recordedWorkerSessionObservation) readWorkerSessionHistory(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, bool, error) {
	if s.hasDurableWorkerHistory() && s.Service != nil {
		observation, err := s.Service.GetObservationByWorkerSessionID(ctx, req)
		if err == nil {
			if recorded, handled, recordedErr := s.recordedWorkerObservationIfAvailable(ctx, observation, req.WorkerSessionID); handled {
				return recorded, true, recordedErr
			}
			return observation, true, nil
		}
		if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			return workersessions.Observation{}, true, err
		}
	}
	if observation, handled, err := s.readRecordedWorkerHistory(ctx, req.WorkerSessionID); handled {
		return observation, true, err
	}
	return workersessions.Observation{}, false, nil
}

func (s *recordedWorkerSessionObservation) readRecordedWorkerHistory(
	ctx context.Context,
	workerSessionID string,
) (workersessions.Observation, bool, error) {
	if s == nil || s.ledger == nil || s.projector == nil {
		return workersessions.Observation{}, false, nil
	}
	observation, found, err := s.readRecordedWorkerSessionByID(ctx, workerSessionID)
	if err != nil || found {
		return observation, true, err
	}
	if s.Service == nil {
		return workersessions.Observation{}, true, workersessions.ErrObservationSessionNotFound
	}
	return workersessions.Observation{}, false, nil
}

func (s *recordedWorkerSessionObservation) recordedWorkerObservationIfAvailable(
	ctx context.Context,
	observation workersessions.Observation,
	workerSessionID string,
) (workersessions.Observation, bool, error) {
	if !observation.ProviderSessionAvailable || s.ledger == nil || s.projector == nil {
		return workersessions.Observation{}, false, nil
	}
	recorded, found, err := s.readRecordedWorkerSessionByID(ctx, workerSessionID)
	if err != nil || found {
		return recorded, true, err
	}
	return workersessions.Observation{}, false, nil
}

func (s *recordedWorkerSessionObservation) readRecordedWorkerSessionByID(
	ctx context.Context,
	workerSessionID string,
) (workersessions.Observation, bool, error) {
	fact, found, err := s.recordedObservationForWorkerSessionID(ctx, workerSessionID)
	if err != nil || !found {
		return workersessions.Observation{}, found, err
	}
	observation := recordedObservationFromFact(fact, s.clock)
	if fact.provider != nil {
		observation, err = s.enrichRecordedObservation(ctx, observation, providerSessionRef(*fact.provider))
		if err != nil {
			return workersessions.Observation{}, false, err
		}
	}
	observation, err = s.withRecordingHealth(ctx, observation)
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	return s.confirmedObservation(observation), true, nil
}

func (s *recordedWorkerSessionObservation) readLiveWorkerSessionByID(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	observation, err := s.Service.GetObservationByWorkerSessionID(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	observation, err = s.withRecordingHealth(ctx, observation)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.confirmedObservation(observation), nil
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
	if result, handled, err := s.readTranscriptHistory(ctx, req); handled {
		return result, err
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

func (s *recordedWorkerSessionObservation) readTranscriptHistory(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, bool, error) {
	if req.WorkerSessionID != "" && s.hasDurableWorkerHistory() && s.Service != nil {
		result, err := s.Service.ReadTranscript(ctx, req)
		if err == nil {
			if recorded, handled, recordedErr := s.recordedTranscriptIfAvailable(ctx, req, result); handled {
				return recorded, true, recordedErr
			}
			return result, true, nil
		}
		if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			return workersessions.ReadTranscriptResult{}, true, err
		}
	}
	if result, handled, err := s.readRecordedTranscriptHistory(ctx, req); handled {
		return result, true, err
	}
	return workersessions.ReadTranscriptResult{}, false, nil
}

func (s *recordedWorkerSessionObservation) readRecordedTranscriptHistory(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, bool, error) {
	if s == nil || s.ledger == nil || s.projector == nil {
		return workersessions.ReadTranscriptResult{}, false, nil
	}
	result, handled, err := s.readRecordedTranscriptForRequest(ctx, req)
	if handled || err != nil {
		return result, true, err
	}
	return workersessions.ReadTranscriptResult{}, false, nil
}

func (s *recordedWorkerSessionObservation) recordedTranscriptIfAvailable(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
	result workersessions.ReadTranscriptResult,
) (workersessions.ReadTranscriptResult, bool, error) {
	if !transcriptProviderSessionAvailable(result) || s.ledger == nil || s.projector == nil {
		return workersessions.ReadTranscriptResult{}, false, nil
	}
	recorded, handled, err := s.readRecordedTranscriptForRequest(ctx, req)
	if err != nil || handled {
		return recorded, true, err
	}
	return workersessions.ReadTranscriptResult{}, false, nil
}

func transcriptProviderSessionAvailable(result workersessions.ReadTranscriptResult) bool {
	return strings.TrimSpace(string(result.ProviderSession.Provider)) != "" ||
		strings.TrimSpace(result.ProviderSession.Kind) != "" ||
		strings.TrimSpace(result.ProviderSession.ID) != ""
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
