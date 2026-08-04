package internal

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

var _ recordings.RecordingLifecycle = (*combinedService)(nil)

// Begin implements recordings.RecordingLifecycle by adapting the existing
// StartRecording lifecycle operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Begin(
	request recordings.BeginRecordingRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.StartRecording(recordings.StartRecordingRequest{
		Enabled:     request.Enabled,
		RecordingID: recordings.RecordingID(request.RecordingID),
		Scope:       fromLifecycleScope(request.Scope),
		Target: recordings.RecordingTargetRequest{
			Artifact:          recordings.RecordingArtifactReference(request.Artifact),
			HomeDir:           request.HomeDir,
			ReportedSessionID: request.ReportedSessionID,
		},
		FlushInterval: request.FlushInterval,
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	if !result.Enabled {
		return recordings.RecordingLifecycleResult{}, nil
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// Bind implements recordings.RecordingLifecycle by adapting the existing
// BindRecording lifecycle operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Bind(
	request recordings.BindLifecycleRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Artifact:    recordings.RecordingArtifactReference(request.Artifact),
		Scope:       fromLifecycleScope(request.Scope),
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// AppendEvent implements recordings.RecordingLifecycle by adapting the
// existing RecordRecordingEvent operation to the root-owned lifecycle
// vocabulary.
func (service *combinedService) AppendEvent(
	request recordings.AppendLifecycleEventRequest,
) (recordings.RecordingLifecycleResult, error) {
	event := request.Event
	result, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Event: recordings.CanonicalEvent{
			ID:          recordings.CanonicalEventID(event.ID),
			Sequence:    recordings.CanonicalEventSequence(event.Sequence),
			FactoryTick: event.FactoryTick,
			Scope:       fromLifecycleScope(event.Scope),
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: event.Cursor.StreamGenerationID,
				Sequence:           recordings.CanonicalEventSequence(event.Cursor.Sequence),
			},
			RecordedAt:    event.RecordedAt,
			Kind:          recordings.CanonicalEventKind(event.Kind),
			Payload:       event.Payload,
			SourceContext: event.SourceContext,
		},
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// RecordFailure implements recordings.RecordingLifecycle by adapting the
// existing RecordRecordingError operation to the root-owned lifecycle
// vocabulary.
func (service *combinedService) RecordFailure(
	request recordings.RecordLifecycleFailureRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Failure: recordings.RecordingFailure{
			Code:       request.Failure.Code,
			Message:    request.Failure.Message,
			RecordedAt: request.Failure.RecordedAt,
		},
		Cause: request.Cause,
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// Flush implements recordings.RecordingLifecycle by adapting the existing
// FlushRecording operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Flush(
	request recordings.FlushLifecycleRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// Stop implements recordings.RecordingLifecycle by adapting the existing
// StopRecording operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Stop(request recordings.StopLifecycleRequest) error {
	_, err := service.StopRecording(recordings.StopRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	return translateLifecycleError(err)
}

// Finish implements recordings.RecordingLifecycle by adapting the existing
// FinishRecording operation to the root-owned lifecycle vocabulary. The
// detached terminal status is returned alongside a translated error so
// callers can observe finalized-with-failures status.
func (service *combinedService) Finish(
	request recordings.FinishLifecycleRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		FinishedAt:  request.FinishedAt,
	})
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, translateLifecycleError(err)
}

// Status implements recordings.RecordingLifecycle by adapting the existing
// QueryRecordingStatus operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Status(
	request recordings.LifecycleStatusRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

func fromLifecycleScope(scope recordings.LifecycleScope) recordings.CanonicalEventScope {
	return recordings.CanonicalEventScope{FactorySessionID: scope.FactorySessionID}
}

func toLifecycleCursor(cursor *recordings.CanonicalEventCursor) *recordings.LifecycleEventCursor {
	if cursor == nil {
		return nil
	}
	detached := recordings.LifecycleEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           int64(cursor.Sequence),
	}
	return &detached
}

func toLifecycleFailures(failures []recordings.RecordingFailure) []recordings.LifecycleFailure {
	if len(failures) == 0 {
		return nil
	}
	detached := make([]recordings.LifecycleFailure, len(failures))
	for index, failure := range failures {
		detached[index] = recordings.LifecycleFailure{
			Code:       failure.Code,
			Message:    failure.Message,
			RecordedAt: failure.RecordedAt,
		}
	}
	return detached
}

func toLifecycleStatus(status recordings.RecordingStatusFacts) recordings.LifecycleStatus {
	return recordings.LifecycleStatus{
		RecordingID:    recordings.LifecycleRecordingID(status.RecordingID),
		Artifact:       recordings.LifecycleArtifactReference(status.Artifact),
		Scope:          recordings.LifecycleScope{FactorySessionID: status.Scope.FactorySessionID},
		State:          recordings.LifecycleState(status.State),
		AcceptedEvents: status.AcceptedEvents,
		LastEvent:      toLifecycleCursor(status.LastEvent),
		FlushedThrough: toLifecycleCursor(status.FlushedThrough),
		Failures:       toLifecycleFailures(status.Failures),
		FinalizedAt:    status.FinalizedAt,
	}
}

func translateLifecycleError(err error) error {
	if err == nil {
		return nil
	}
	kind := recordings.LifecycleErrorWriteFailed
	switch {
	case errors.Is(err, recordings.ErrMissingRecordingTarget):
		kind = recordings.LifecycleErrorInvalidTarget
	case errors.Is(err, recordings.ErrInvalidRecordingScope):
		kind = recordings.LifecycleErrorInvalidScope
	case errors.Is(err, recordings.ErrRecordingBindingConflict):
		kind = recordings.LifecycleErrorBindingConflict
	case errors.Is(err, recordings.ErrInvalidRecordingEvent):
		kind = recordings.LifecycleErrorInvalidEvent
	case errors.Is(err, recordings.ErrInvalidRecordingFailure):
		kind = recordings.LifecycleErrorInvalidFailure
	case errors.Is(err, recordings.ErrRecordingWriteRejected):
		kind = recordings.LifecycleErrorTerminal
	case errors.Is(err, recordings.ErrInvalidRecordingTerminalMetadata):
		kind = recordings.LifecycleErrorInvalidTerminalMetadata
	}
	return &recordings.LifecycleError{Kind: kind, Message: err.Error(), Cause: err}
}
