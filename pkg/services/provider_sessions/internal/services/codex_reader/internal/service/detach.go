package service

import (
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func detachDetail(detail providersessions.Detail) providersessions.Detail {
	return providersessions.Detail{
		ProviderSession: detail.ProviderSession,
		Source:          detachSourceMetadata(detail.Source),
		Parse:           detachParseSummary(detail.Parse),
		Transcript:      detachTranscript(detail.Transcript),
	}
}

func detachParsedDetails(parsed ParsedDetails) ParsedDetails {
	return ParsedDetails{
		Summary:    detachParseSummary(parsed.Summary),
		Transcript: detachTranscript(parsed.Transcript),
	}
}

func detachSourceMetadata(source providersessions.SourceMetadata) providersessions.SourceMetadata {
	return providersessions.SourceMetadata{
		ModifiedAt:   cloneTimePtr(source.ModifiedAt),
		RelativePath: source.RelativePath,
		SizeBytes:    source.SizeBytes,
	}
}

func detachParseSummary(summary providersessions.ParseSummary) providersessions.ParseSummary {
	return providersessions.ParseSummary{
		EventCount:         summary.EventCount,
		FunctionCalls:      detachFunctionCalls(summary.FunctionCalls),
		LineCount:          summary.LineCount,
		MalformedLineCount: summary.MalformedLineCount,
		ParseErrors:        detachLineErrors(summary.ParseErrors),
		Reasoning:          detachReasoningSummaries(summary.Reasoning),
		TokenUsage:         detachTokenUsage(summary.TokenUsage),
		Turns:              detachTurnSummaries(summary.Turns),
		UnknownEventCount:  summary.UnknownEventCount,
		UnknownEvents:      detachUnknownEvents(summary.UnknownEvents),
	}
}

func detachTranscript(entries []providersessions.TranscriptEntry) []providersessions.TranscriptEntry {
	if len(entries) == 0 {
		return []providersessions.TranscriptEntry{}
	}
	cloned := make([]providersessions.TranscriptEntry, len(entries))
	for i := range entries {
		cloned[i] = detachTranscriptEntry(entries[i])
	}
	return cloned
}

func detachTranscriptEntry(entry providersessions.TranscriptEntry) providersessions.TranscriptEntry {
	return providersessions.TranscriptEntry{
		Arguments:        cloneStringPtr(entry.Arguments),
		CallID:           cloneStringPtr(entry.CallID),
		Encrypted:        cloneBoolPtr(entry.Encrypted),
		EncryptedContent: cloneStringPtr(entry.EncryptedContent),
		LineNumber:       cloneIntPtr(entry.LineNumber),
		Name:             cloneStringPtr(entry.Name),
		Order:            entry.Order,
		Output:           cloneStringPtr(entry.Output),
		SourceType:       cloneStringPtr(entry.SourceType),
		Status:           cloneStringPtr(entry.Status),
		Summary:          cloneStringPtr(entry.Summary),
		Text:             cloneStringPtr(entry.Text),
		Timestamp:        cloneTimePtr(entry.Timestamp),
		TurnIndex:        cloneIntPtr(entry.TurnIndex),
		Type:             entry.Type,
	}
}

func detachFunctionCalls(calls []providersessions.FunctionCallSummary) []providersessions.FunctionCallSummary {
	if len(calls) == 0 {
		return []providersessions.FunctionCallSummary{}
	}
	cloned := make([]providersessions.FunctionCallSummary, len(calls))
	for i := range calls {
		cloned[i] = providersessions.FunctionCallSummary{
			Arguments: cloneStringPtr(calls[i].Arguments),
			CallID:    cloneStringPtr(calls[i].CallID),
			Name:      cloneStringPtr(calls[i].Name),
			Order:     calls[i].Order,
			Output:    cloneStringPtr(calls[i].Output),
			Status:    cloneStringPtr(calls[i].Status),
			TurnIndex: cloneIntPtr(calls[i].TurnIndex),
			Type:      calls[i].Type,
		}
	}
	return cloned
}

func detachReasoningSummaries(reasoning []providersessions.ReasoningSummary) []providersessions.ReasoningSummary {
	if len(reasoning) == 0 {
		return []providersessions.ReasoningSummary{}
	}
	cloned := make([]providersessions.ReasoningSummary, len(reasoning))
	for i := range reasoning {
		cloned[i] = providersessions.ReasoningSummary{
			Encrypted:        cloneBoolPtr(reasoning[i].Encrypted),
			EncryptedContent: cloneStringPtr(reasoning[i].EncryptedContent),
			Order:            reasoning[i].Order,
			SourceType:       reasoning[i].SourceType,
			Summary:          cloneStringPtr(reasoning[i].Summary),
			Text:             cloneStringPtr(reasoning[i].Text),
			TurnIndex:        cloneIntPtr(reasoning[i].TurnIndex),
		}
	}
	return cloned
}

func detachTurnSummaries(turns []providersessions.TurnSummary) []providersessions.TurnSummary {
	if len(turns) == 0 {
		return []providersessions.TurnSummary{}
	}
	cloned := make([]providersessions.TurnSummary, len(turns))
	for i := range turns {
		cloned[i] = providersessions.TurnSummary{
			EventCount:        turns[i].EventCount,
			FunctionCallCount: turns[i].FunctionCallCount,
			Index:             turns[i].Index,
			ReasoningCount:    turns[i].ReasoningCount,
			ResponseItemCount: turns[i].ResponseItemCount,
			StartedAt:         cloneTimePtr(turns[i].StartedAt),
		}
	}
	return cloned
}

func detachLineErrors(errors []providersessions.LineError) []providersessions.LineError {
	if len(errors) == 0 {
		return []providersessions.LineError{}
	}
	cloned := make([]providersessions.LineError, len(errors))
	copy(cloned, errors)
	return cloned
}

func detachUnknownEvents(events []providersessions.UnknownEvent) []providersessions.UnknownEvent {
	if len(events) == 0 {
		return []providersessions.UnknownEvent{}
	}
	cloned := make([]providersessions.UnknownEvent, len(events))
	for i := range events {
		cloned[i] = providersessions.UnknownEvent{
			LineNumber:  events[i].LineNumber,
			PayloadType: cloneStringPtr(events[i].PayloadType),
			Type:        cloneStringPtr(events[i].Type),
		}
	}
	return cloned
}

func detachTokenUsage(usage *providersessions.TokenUsage) *providersessions.TokenUsage {
	if usage == nil {
		return nil
	}
	return &providersessions.TokenUsage{
		CacheWriteTokens:      cloneIntPtr(usage.CacheWriteTokens),
		CachedInputTokens:     cloneIntPtr(usage.CachedInputTokens),
		InputTokens:           cloneIntPtr(usage.InputTokens),
		OutputTokens:          cloneIntPtr(usage.OutputTokens),
		ReasoningOutputTokens: cloneIntPtr(usage.ReasoningOutputTokens),
		TotalTokens:           cloneIntPtr(usage.TotalTokens),
	}
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
