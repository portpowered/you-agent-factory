package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// ReadTranscript returns the final normalized Provider Sessions transcript for
// one exact Worker Session association. The lifecycle check happens before the
// provider projection so an active session has a stable active outcome even
// when its provider source is not yet readable.
func (r *registry) ReadTranscript(ctx context.Context, req workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session transcript read rejected", "outcome", "invalid")
		return workersessions.ReadTranscriptResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}

	session, metadata, err := r.transcriptSession(req.ProviderSession)
	if err != nil {
		r.logger.Info("worker session transcript read", "outcome", "not_found")
		return workersessions.ReadTranscriptResult{}, err
	}
	if !session.State.Terminal() {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "active")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptActive
	}
	if session.ProviderSessionAssociation == nil {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "unavailable")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	if r.providerSessions == nil {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "projection_unavailable")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}

	projected, projectErr := r.providerSessions.Project(providersessions.ProjectRequest{
		Session: req.ProviderSession.Clone(),
		Context: ctx,
	})
	if projectErr != nil {
		if errors.Is(projectErr, context.Canceled) || errors.Is(projectErr, providersessions.ErrOperationCanceled) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationCanceled
		}
		if transcriptSourceUnavailable(projectErr) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("%w: %v", workersessions.ErrObservationTranscriptProjectionUnavailable, projectErr)
	}

	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: session.ID,
		ProviderSession: req.ProviderSession.Clone(),
		WorkIDs:         append([]string(nil), metadata.workIDs...),
		AttemptID:       metadata.attemptID,
		State:           session.State,
		Entries:         transcriptEntries(projected.Detail.Transcript),
	}
	if session.ProviderSessionAssociation != nil {
		result.TurnID = session.ProviderSessionAssociation.TurnID
		result.AttemptID = session.ProviderSessionAssociation.AttemptID
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate Worker Session transcript: %w", err)
	}
	r.logger.Info("worker session transcript read", "workerSessionID", result.WorkerSessionID, "outcome", "success", "result_count", len(result.Entries))
	return result, nil
}

func (r *registry) transcriptSession(ref providers.SessionRef) (workersessions.Session, *observation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, 1)
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil && session.ProviderSessionAssociation.Reference == ref {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return workersessions.Session{}, nil, workersessions.ErrObservationSessionNotFound
	}
	sortStrings(ids)
	session := cloneSession(r.sessions[ids[0]])
	metadata := cloneObservation(r.observations[ids[0]])
	if metadata == nil {
		return workersessions.Session{}, nil, workersessions.ErrObservationSessionNotFound
	}
	return session, metadata, nil
}

func transcriptSourceUnavailable(err error) bool {
	return errors.Is(err, providersessions.ErrSessionNotFound) ||
		errors.Is(err, providersessions.ErrAmbiguousSessionFile) ||
		errors.Is(err, providersessions.ErrSessionSourceNotRegularFile) ||
		errors.Is(err, providersessions.ErrSessionStorageUnavailable) ||
		errors.Is(err, providersessions.ErrSessionOutsideRoot)
}

func transcriptEntries(values []providersessions.TranscriptEntry) []workersessions.TranscriptEntry {
	entries := make([]workersessions.TranscriptEntry, len(values))
	for index, value := range values {
		entries[index] = workersessions.TranscriptEntry{
			Arguments:        cloneTranscriptString(value.Arguments),
			CallID:           cloneTranscriptString(value.CallID),
			Encrypted:        cloneTranscriptBool(value.Encrypted),
			EncryptedContent: cloneTranscriptString(value.EncryptedContent),
			LineNumber:       cloneTranscriptInt(value.LineNumber),
			Name:             cloneTranscriptString(value.Name),
			Order:            value.Order,
			Output:           cloneTranscriptString(value.Output),
			SourceType:       cloneTranscriptString(value.SourceType),
			Status:           cloneTranscriptString(value.Status),
			Summary:          cloneTranscriptString(value.Summary),
			Text:             cloneTranscriptString(value.Text),
			Timestamp:        cloneTranscriptTime(value.Timestamp),
			TurnIndex:        cloneTranscriptInt(value.TurnIndex),
			Type:             workersessions.TranscriptEntryType(value.Type),
		}
	}
	return entries
}

func cloneTranscriptBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTranscriptInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTranscriptString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTranscriptTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
