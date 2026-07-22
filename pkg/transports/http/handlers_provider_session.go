package http

import (
	"errors"
	"net/http"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func (s *Server) GetProviderSessionDetails(
	w http.ResponseWriter,
	_ *http.Request,
	params factoryapi.GetProviderSessionDetailsParams,
) {
	detail, err := s.providerSessions.Details(
		string(params.Provider),
		string(params.Kind),
		string(params.Id),
	)
	if err != nil {
		s.writeProviderSessionError(w, params, err)
		return
	}
	s.writeJSON(w, http.StatusOK, providerSessionDetailToAPI(detail))
}

func (s *Server) writeProviderSessionError(
	w http.ResponseWriter,
	params factoryapi.GetProviderSessionDetailsParams,
	err error,
) {
	var lookupErr *providersessions.LookupError
	switch {
	case errors.Is(err, providersessions.ErrUnsupportedProvider),
		errors.Is(err, providersessions.ErrUnsupportedKind):
		s.writeError(w, http.StatusBadRequest, "invalid request parameter", "BAD_REQUEST")
	case errors.Is(err, providersessions.ErrInvalidIdentifier):
		_ = errors.As(err, &lookupErr)
		s.writeError(w, http.StatusBadRequest, invalidProviderSessionIdentifierMessage(lookupErr), "BAD_REQUEST")
	case errors.Is(err, providersessions.ErrSessionNotFound):
		if errors.As(err, &lookupErr) && lookupErr.Provider == providersessions.ProviderCursor {
			s.logCursorProviderSessionLookupNotFound(params.Kind, string(params.Id), lookupErr.Root)
		}
		s.writeError(w, http.StatusNotFound, "provider session not found", "NOT_FOUND")
	case errors.Is(err, providersessions.ErrAmbiguousSessionFile):
		s.writeError(w, http.StatusInternalServerError, "multiple provider session files match session identifier", "INTERNAL_ERROR")
	default:
		s.logger.Error("load provider session details failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to load provider session details", "INTERNAL_ERROR")
	}
}

func invalidProviderSessionIdentifierMessage(lookupErr *providersessions.LookupError) string {
	if lookupErr != nil && lookupErr.Provider == providersessions.ProviderCursor {
		return "provider session must be a cursor session_id identifier without path separators"
	}
	return "provider session must be a codex session_id identifier without path separators"
}

func (s *Server) logCursorProviderSessionLookupNotFound(
	kind factoryapi.LoadableProviderSessionKind,
	requestedID string,
	root string,
) {
	fields := []zap.Field{
		zap.String("provider", string(providersessions.ProviderCursor)),
		zap.String("lookup_kind", string(kind)),
		zap.String("requested_id", requestedID),
	}
	if root == "" {
		fields = append(fields, zap.Bool("root_configured", false))
	} else {
		fields = append(fields,
			zap.Bool("root_configured", true),
			zap.String("searched_root", root),
		)
	}
	s.logger.Info("cursor provider session lookup not found", fields...)
}

func providerSessionDetailToAPI(detail providersessions.Detail) factoryapi.ProviderSessionDetailResponse {
	return factoryapi.ProviderSessionDetailResponse{
		ProviderSession: factoryapi.LoadableProviderSessionRef{
			Provider: factoryapi.LoadableProviderSessionProvider(detail.ProviderSession.Provider),
			Kind:     factoryapi.LoadableProviderSessionKind(detail.ProviderSession.Kind),
			Id:       detail.ProviderSession.ID,
		},
		Source: factoryapi.ProviderSessionSourceMetadata{
			ModifiedAt:   detail.Source.ModifiedAt,
			RelativePath: detail.Source.RelativePath,
			SizeBytes:    detail.Source.SizeBytes,
		},
		Parse:      providerSessionParseSummaryToAPI(detail.Parse),
		Transcript: providerSessionTranscriptToAPI(detail.Transcript),
	}
}

func providerSessionParseSummaryToAPI(summary providersessions.ParseSummary) factoryapi.ProviderSessionParseSummary {
	return factoryapi.ProviderSessionParseSummary{
		EventCount:         summary.EventCount,
		FunctionCalls:      providerSessionFunctionCallsToAPI(summary.FunctionCalls),
		LineCount:          summary.LineCount,
		MalformedLineCount: summary.MalformedLineCount,
		ParseErrors:        providerSessionParseErrorsToAPI(summary.ParseErrors),
		Reasoning:          providerSessionReasoningToAPI(summary.Reasoning),
		TokenUsage:         providerSessionTokenUsageToAPI(summary.TokenUsage),
		Turns:              providerSessionTurnsToAPI(summary.Turns),
		UnknownEventCount:  summary.UnknownEventCount,
		UnknownEvents:      providerSessionUnknownEventsToAPI(summary.UnknownEvents),
	}
}

func providerSessionFunctionCallsToAPI(values []providersessions.FunctionCallSummary) []factoryapi.ProviderSessionFunctionCallSummary {
	mapped := make([]factoryapi.ProviderSessionFunctionCallSummary, len(values))
	for i, value := range values {
		mapped[i] = factoryapi.ProviderSessionFunctionCallSummary{
			Arguments: value.Arguments,
			CallId:    value.CallID,
			Name:      value.Name,
			Order:     value.Order,
			Output:    value.Output,
			Status:    value.Status,
			TurnIndex: value.TurnIndex,
			Type:      value.Type,
		}
	}
	return mapped
}

func providerSessionParseErrorsToAPI(values []providersessions.LineError) []factoryapi.ProviderSessionLineError {
	mapped := make([]factoryapi.ProviderSessionLineError, len(values))
	for i, value := range values {
		mapped[i] = factoryapi.ProviderSessionLineError{
			LineNumber: value.LineNumber,
			Message:    value.Message,
		}
	}
	return mapped
}

func providerSessionReasoningToAPI(values []providersessions.ReasoningSummary) []factoryapi.ProviderSessionReasoningSummary {
	mapped := make([]factoryapi.ProviderSessionReasoningSummary, len(values))
	for i, value := range values {
		mapped[i] = factoryapi.ProviderSessionReasoningSummary{
			Encrypted:        value.Encrypted,
			EncryptedContent: value.EncryptedContent,
			Order:            value.Order,
			SourceType:       value.SourceType,
			Summary:          value.Summary,
			Text:             value.Text,
			TurnIndex:        value.TurnIndex,
		}
	}
	return mapped
}

func providerSessionTokenUsageToAPI(value *providersessions.TokenUsage) *factoryapi.ProviderSessionTokenUsage {
	if value == nil {
		return nil
	}
	return &factoryapi.ProviderSessionTokenUsage{
		CacheWriteTokens:      value.CacheWriteTokens,
		CachedInputTokens:     value.CachedInputTokens,
		InputTokens:           value.InputTokens,
		OutputTokens:          value.OutputTokens,
		ReasoningOutputTokens: value.ReasoningOutputTokens,
		TotalTokens:           value.TotalTokens,
	}
}

func providerSessionTurnsToAPI(values []providersessions.TurnSummary) []factoryapi.ProviderSessionTurnSummary {
	mapped := make([]factoryapi.ProviderSessionTurnSummary, len(values))
	for i, value := range values {
		mapped[i] = factoryapi.ProviderSessionTurnSummary{
			EventCount:        value.EventCount,
			FunctionCallCount: value.FunctionCallCount,
			Index:             value.Index,
			ReasoningCount:    value.ReasoningCount,
			ResponseItemCount: value.ResponseItemCount,
			StartedAt:         value.StartedAt,
		}
	}
	return mapped
}

func providerSessionUnknownEventsToAPI(values []providersessions.UnknownEvent) []factoryapi.ProviderSessionUnknownEvent {
	mapped := make([]factoryapi.ProviderSessionUnknownEvent, len(values))
	for i, value := range values {
		mapped[i] = factoryapi.ProviderSessionUnknownEvent{
			LineNumber:  value.LineNumber,
			PayloadType: value.PayloadType,
			Type:        value.Type,
		}
	}
	return mapped
}

func providerSessionTranscriptToAPI(values []providersessions.TranscriptEntry) []factoryapi.ProviderSessionTranscriptEntry {
	mapped := make([]factoryapi.ProviderSessionTranscriptEntry, len(values))
	for i, value := range values {
		mapped[i] = factoryapi.ProviderSessionTranscriptEntry{
			Arguments:        value.Arguments,
			CallId:           value.CallID,
			Encrypted:        value.Encrypted,
			EncryptedContent: value.EncryptedContent,
			LineNumber:       value.LineNumber,
			Name:             value.Name,
			Order:            value.Order,
			Output:           value.Output,
			SourceType:       value.SourceType,
			Status:           value.Status,
			Summary:          value.Summary,
			Text:             value.Text,
			Timestamp:        value.Timestamp,
			TurnIndex:        value.TurnIndex,
			Type:             factoryapi.ProviderSessionTranscriptEntryType(value.Type),
		}
	}
	return mapped
}
