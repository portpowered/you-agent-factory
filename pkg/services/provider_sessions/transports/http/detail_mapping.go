package http

import (
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
