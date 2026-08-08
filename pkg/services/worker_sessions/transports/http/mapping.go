package http

import (
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListWorkerSessionsResponseToAPI maps a detached Worker Sessions result to
// the generated API representation while preserving absent optional values.
func ListWorkerSessionsResponseToAPI(result workersessions.ListObservationsResult) factoryapi.ListWorkerSessionsResponse {
	sessions := make([]factoryapi.WorkerSessionObservation, 0, len(result.Observations))
	for _, observation := range result.Observations {
		sessions = append(sessions, WorkerSessionObservationToAPI(observation))
	}
	return factoryapi.ListWorkerSessionsResponse{Sessions: sessions}
}

// WorkerSessionObservationToAPI maps one detached observation.
func WorkerSessionObservationToAPI(observation workersessions.Observation) factoryapi.WorkerSessionObservation {
	result := factoryapi.WorkerSessionObservation{
		WorkerSessionId:          observation.WorkerSessionID,
		ProviderSessionAvailable: observation.ProviderSessionAvailable,
		WorkIds:                  append([]string(nil), observation.WorkIDs...),
		AttemptId:                observation.AttemptID,
		State:                    factoryapi.WorkerSessionObservationState(observation.State),
		DurationBasis:            factoryapi.WorkerSessionObservationDurationBasis(observation.DurationBasis),
		Transcript:               factoryapi.WorkerSessionObservationTranscript(observation.Transcript),
		Parse:                    workerSessionParseDiagnosticsToAPI(observation.Parse),
	}
	if observation.TurnID != "" {
		result.TurnId = stringPtr(observation.TurnID)
	}
	if observation.ProviderSessionAvailable {
		result.ProviderSession = &factoryapi.WorkerSessionProviderSessionRef{
			Provider: string(observation.ProviderSession.Provider),
			Kind:     observation.ProviderSession.Kind,
			Id:       observation.ProviderSession.ID,
		}
	}
	if observation.StartedAt != nil {
		started := observation.StartedAt.UTC()
		result.StartedAt = &started
	}
	if observation.EndedAt != nil {
		ended := observation.EndedAt.UTC()
		result.EndedAt = &ended
	}
	if observation.Duration != nil {
		millis := observation.Duration.Milliseconds()
		result.DurationMillis = &millis
	}
	if observation.TokenUsage != nil {
		result.TokenUsage = &factoryapi.ProviderSessionTokenUsage{
			CacheWriteTokens:      cloneInt(observation.TokenUsage.CacheWriteTokens),
			CachedInputTokens:     cloneInt(observation.TokenUsage.CachedInputTokens),
			InputTokens:           cloneInt(observation.TokenUsage.InputTokens),
			OutputTokens:          cloneInt(observation.TokenUsage.OutputTokens),
			ReasoningOutputTokens: cloneInt(observation.TokenUsage.ReasoningOutputTokens),
			TotalTokens:           cloneInt(observation.TokenUsage.TotalTokens),
		}
	}
	if observation.Failure != nil {
		result.Failure = &factoryapi.WorkerSessionFailure{
			Kind:                            string(observation.Failure.Kind),
			Detail:                          observation.Failure.Detail,
			ProviderFailureKind:             stringPtrIfNonEmpty(string(observation.Failure.ProviderFailureKind)),
			ProviderContinuationFailureKind: stringPtrIfNonEmpty(string(observation.Failure.ProviderContinuationFailureKind)),
			ProviderContinuationOutcome:     stringPtrIfNonEmpty(string(observation.Failure.ProviderContinuationOutcome)),
		}
	}
	return result
}

func workerSessionParseDiagnosticsToAPI(value workersessions.ParseDiagnostics) factoryapi.WorkerSessionParseDiagnostics {
	errors := make([]factoryapi.WorkerSessionParseDiagnostic, 0, len(value.Errors))
	for _, item := range value.Errors {
		errors = append(errors, factoryapi.WorkerSessionParseDiagnostic{
			Code: item.Code, LineNumber: item.LineNumber, Message: item.Message,
		})
	}
	return factoryapi.WorkerSessionParseDiagnostics{
		EventCount:         value.EventCount,
		MalformedLineCount: value.MalformedLineCount,
		UnknownEventCount:  value.UnknownEventCount,
		Errors:             errors,
	}
}

func stringPtr(value string) *string { return &value }

func stringPtrIfNonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
