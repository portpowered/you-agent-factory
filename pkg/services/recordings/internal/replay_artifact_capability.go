package internal

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

var _ recordings.RecordingReplayArtifacts = (*combinedService)(nil)

// LoadReplay implements recordings.RecordingReplayArtifacts by adapting the
// existing LoadReplayRecording operation to the root-owned replay/artifact
// vocabulary.
func (service *combinedService) LoadReplay(
	request recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	result, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.LoadReplayResult{}, translateReplayArtifactError(err)
	}
	return recordings.LoadReplayResult{Replay: toReplayFacts(result.Recording)}, nil
}

// BuildArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing BuildPortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) BuildArtifact(
	request recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	result, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.BuildArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.BuildArtifactResult{Artifact: toArtifactEnvelope(result.Artifact)}, nil
}

// ValidateArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing ValidatePortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) ValidateArtifact(
	request recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	result, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: fromArtifactEnvelope(request.Artifact),
	})
	if err != nil {
		return recordings.ValidateArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.ValidateArtifactResult{Summary: toArtifactSummary(result.Summary)}, nil
}

// EncodeArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing EncodePortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) EncodeArtifact(
	request recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	result, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: fromArtifactEnvelope(request.Artifact),
	})
	if err != nil {
		return recordings.EncodeArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.EncodeArtifactResult{Payload: result.Payload}, nil
}

// DecodeArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing DecodePortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) DecodeArtifact(
	request recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	result, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: request.Payload,
	})
	if err != nil {
		return recordings.DecodeArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.DecodeArtifactResult{Artifact: toArtifactEnvelope(result.Artifact)}, nil
}

// SummarizeArtifact implements recordings.RecordingReplayArtifacts by
// adapting the existing SummarizePortableArtifact operation to the
// root-owned replay/artifact vocabulary.
func (service *combinedService) SummarizeArtifact(
	request recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	result, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: fromArtifactEnvelope(request.Artifact),
	})
	if err != nil {
		return recordings.SummarizeArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.SummarizeArtifactResult{Summary: toArtifactSummary(result.Summary)}, nil
}

// ExportArtifact implements recordings.RecordingReplayArtifacts by adapting
// the existing ExportPortableArtifact operation to the root-owned
// replay/artifact vocabulary.
func (service *combinedService) ExportArtifact(
	ctx context.Context,
	request recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	result, err := service.ExportPortableArtifact(ctx, recordings.ExportPortableArtifactRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.ExportArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.ExportArtifactResult{
		Reference: recordings.ArtifactReference(result.Reference),
		Artifact:  toArtifactEnvelope(result.Artifact),
	}, nil
}

// ReadArtifact implements recordings.RecordingReplayArtifacts by adapting the
// existing ReadPortableArtifact operation to the root-owned replay/artifact
// vocabulary.
func (service *combinedService) ReadArtifact(
	ctx context.Context,
	request recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	result, err := service.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Reference:   recordings.RecordingArtifactReference(request.Reference),
	})
	if err != nil {
		return recordings.ReadArtifactResult{}, translateReplayArtifactError(err)
	}
	return recordings.ReadArtifactResult{Artifact: toArtifactEnvelope(result.Artifact)}, nil
}

func toReplayScope(scope recordings.CanonicalEventScope) recordings.ReplayScope {
	return recordings.ReplayScope{FactorySessionID: scope.FactorySessionID}
}

func fromReplayScope(scope recordings.ReplayScope) recordings.CanonicalEventScope {
	return recordings.CanonicalEventScope{FactorySessionID: scope.FactorySessionID}
}

func toReplayEventCursor(cursor *recordings.CanonicalEventCursor) *recordings.ReplayEventCursor {
	if cursor == nil {
		return nil
	}
	detached := recordings.ReplayEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           int64(cursor.Sequence),
	}
	return &detached
}

func fromReplayEventCursor(cursor recordings.ReplayEventCursor) recordings.CanonicalEventCursor {
	return recordings.CanonicalEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           recordings.CanonicalEventSequence(cursor.Sequence),
	}
}

func toReplayEventCursorValue(cursor recordings.CanonicalEventCursor) recordings.ReplayEventCursor {
	return recordings.ReplayEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           int64(cursor.Sequence),
	}
}

func toReplayEvents(events []recordings.CanonicalEvent) []recordings.ReplayEvent {
	if len(events) == 0 {
		return nil
	}
	detached := make([]recordings.ReplayEvent, len(events))
	for index, event := range events {
		detached[index] = recordings.ReplayEvent{
			ID:            string(event.ID),
			Sequence:      int64(event.Sequence),
			FactoryTick:   event.FactoryTick,
			Scope:         toReplayScope(event.Scope),
			Cursor:        toReplayEventCursorValue(event.Cursor),
			RecordedAt:    event.RecordedAt,
			Kind:          string(event.Kind),
			Payload:       event.Payload,
			SourceContext: event.SourceContext,
		}
	}
	return detached
}

func fromReplayEvents(events []recordings.ReplayEvent) []recordings.CanonicalEvent {
	if len(events) == 0 {
		return nil
	}
	detached := make([]recordings.CanonicalEvent, len(events))
	for index, event := range events {
		detached[index] = recordings.CanonicalEvent{
			ID:            recordings.CanonicalEventID(event.ID),
			Sequence:      recordings.CanonicalEventSequence(event.Sequence),
			FactoryTick:   event.FactoryTick,
			Scope:         fromReplayScope(event.Scope),
			Cursor:        fromReplayEventCursor(event.Cursor),
			RecordedAt:    event.RecordedAt,
			Kind:          recordings.CanonicalEventKind(event.Kind),
			Payload:       event.Payload,
			SourceContext: event.SourceContext,
		}
	}
	return detached
}

func toReplayFacts(facts recordings.ReplayRecordingFacts) recordings.ReplayFacts {
	return recordings.ReplayFacts{
		RecordingID: recordings.ReplayRecordingID(facts.RecordingID),
		Scope:       toReplayScope(facts.Scope),
		Events:      toReplayEvents(facts.Events),
	}
}

func toArtifactFailures(failures []recordings.RecordingFailure) []recordings.ArtifactFailure {
	if len(failures) == 0 {
		return nil
	}
	detached := make([]recordings.ArtifactFailure, len(failures))
	for index, failure := range failures {
		detached[index] = recordings.ArtifactFailure{
			Code:       failure.Code,
			Message:    failure.Message,
			RecordedAt: failure.RecordedAt,
		}
	}
	return detached
}

func fromArtifactFailures(failures []recordings.ArtifactFailure) []recordings.RecordingFailure {
	if len(failures) == 0 {
		return nil
	}
	detached := make([]recordings.RecordingFailure, len(failures))
	for index, failure := range failures {
		detached[index] = recordings.RecordingFailure{
			Code:       failure.Code,
			Message:    failure.Message,
			RecordedAt: failure.RecordedAt,
		}
	}
	return detached
}

func toArtifactSummary(summary recordings.PortableArtifactSummary) recordings.ArtifactSummary {
	return recordings.ArtifactSummary{
		RecordingID: recordings.ReplayRecordingID(summary.RecordingID),
		Reference:   recordings.ArtifactReference(summary.Reference),
		Scope:       toReplayScope(summary.Scope),
		State:       recordings.ArtifactState(summary.State),
		EventCount:  summary.EventCount,
		FirstCursor: toReplayEventCursor(summary.FirstCursor),
		LastCursor:  toReplayEventCursor(summary.LastCursor),
		Failures:    toArtifactFailures(summary.Failures),
		Available:   summary.Available,
	}
}

func fromArtifactSummary(summary recordings.ArtifactSummary) recordings.PortableArtifactSummary {
	var firstCursor, lastCursor *recordings.CanonicalEventCursor
	if summary.FirstCursor != nil {
		cursor := fromReplayEventCursor(*summary.FirstCursor)
		firstCursor = &cursor
	}
	if summary.LastCursor != nil {
		cursor := fromReplayEventCursor(*summary.LastCursor)
		lastCursor = &cursor
	}
	return recordings.PortableArtifactSummary{
		RecordingID: recordings.RecordingID(summary.RecordingID),
		Reference:   recordings.RecordingArtifactReference(summary.Reference),
		Scope:       fromReplayScope(summary.Scope),
		State:       recordings.RecordingLifecycleState(summary.State),
		EventCount:  summary.EventCount,
		FirstCursor: firstCursor,
		LastCursor:  lastCursor,
		Failures:    fromArtifactFailures(summary.Failures),
		Available:   summary.Available,
	}
}

func toArtifactEnvelope(artifact recordings.PortableArtifact) recordings.ArtifactEnvelope {
	return recordings.ArtifactEnvelope{
		SchemaVersion: recordings.ArtifactSchemaVersion(artifact.SchemaVersion),
		Summary:       toArtifactSummary(artifact.Summary),
		Events:        toReplayEvents(artifact.Events),
		Integrity: recordings.ArtifactIntegrity{
			Algorithm: artifact.Integrity.Algorithm,
			Digest:    artifact.Integrity.Digest,
		},
	}
}

func fromArtifactEnvelope(envelope recordings.ArtifactEnvelope) recordings.PortableArtifact {
	return recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaVersion(envelope.SchemaVersion),
		Summary:       fromArtifactSummary(envelope.Summary),
		Events:        fromReplayEvents(envelope.Events),
		Integrity: recordings.PortableArtifactIntegrity{
			Algorithm: envelope.Integrity.Algorithm,
			Digest:    envelope.Integrity.Digest,
		},
	}
}

func translateReplayArtifactError(err error) error {
	if err == nil {
		return nil
	}
	kind := recordings.ReplayArtifactErrorInvalid
	switch {
	case errors.Is(err, recordings.ErrReplayRecordingNotFound):
		kind = recordings.ReplayArtifactErrorNotFound
	case errors.Is(err, recordings.ErrReplayRecordingNotFinalized):
		kind = recordings.ReplayArtifactErrorNotFinalized
	case errors.Is(err, recordings.ErrCorruptReplayInput):
		kind = recordings.ReplayArtifactErrorCorruptInput
	case errors.Is(err, recordings.ErrPortableArtifactUnavailable):
		kind = recordings.ReplayArtifactErrorUnavailable
	case errors.Is(err, recordings.ErrUnsupportedPortableArtifactSchema):
		kind = recordings.ReplayArtifactErrorUnsupportedSchema
	case errors.Is(err, recordings.ErrInvalidPortableArtifactIntegrity):
		kind = recordings.ReplayArtifactErrorInvalidIntegrity
	case errors.Is(err, recordings.ErrInvalidPortableArtifactOrder):
		kind = recordings.ReplayArtifactErrorInvalidOrder
	case errors.Is(err, recordings.ErrPortableArtifactExportFailed):
		kind = recordings.ReplayArtifactErrorExportFailed
	case errors.Is(err, recordings.ErrForeignPortableArtifact):
		kind = recordings.ReplayArtifactErrorForeign
	case errors.Is(err, recordings.ErrPortableArtifactCancelled):
		kind = recordings.ReplayArtifactErrorCancelled
	case errors.Is(err, recordings.ErrInvalidPortableArtifact):
		kind = recordings.ReplayArtifactErrorInvalid
	}
	return &recordings.ReplayArtifactError{Kind: kind, Message: err.Error(), Cause: err}
}
