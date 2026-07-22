// Package cursor discovers, parses, and maps Cursor provider sessions from
// cursor-agent CLI storage without shelling out to an external parser.
package cursor

import (
	"errors"
	"fmt"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// LoadDetails resolves a Cursor session_id from server-configured cursor-agent storage.
func LoadDetails(files providersessions.FileSystem, walkDirectory providersessions.CursorWalkDirectory, resolveSymlinks providersessions.CursorResolveSymlinks, openSQLDatabase providersessions.CursorOpenSQLDatabase, root AgentStorageRoot, id string) (providersessions.Detail, error) {
	if err := ValidateSessionID(id); err != nil {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}

	resolved, err := ResolveStoreDB(files, walkDirectory, resolveSymlinks, root, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidSessionID):
			return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
		case errors.Is(err, ErrSessionNotFound):
			return providersessions.Detail{}, providersessions.ErrSessionNotFound
		case errors.Is(err, ErrAmbiguousSession):
			return providersessions.Detail{}, providersessions.ErrAmbiguousSessionFile
		default:
			return providersessions.Detail{}, fmt.Errorf("resolve cursor session store: %w", err)
		}
	}

	session, err := LoadSessionData(files, openSQLDatabase, resolved)
	if err != nil {
		return providersessions.Detail{}, fmt.Errorf("load cursor session data: %w", err)
	}

	info, err := files.Stat(resolved.AbsolutePath)
	if err != nil {
		return providersessions.Detail{}, fmt.Errorf("stat cursor session store: %w", err)
	}
	modifiedAt := info.ModTime().UTC()

	return mapSessionToProviderSessionDetail(id, resolved.RelativePath, info.Size(), &modifiedAt, session), nil
}

func mapSessionToProviderSessionDetail(
	id string,
	relativePath string,
	sizeBytes int64,
	modifiedAt *time.Time,
	session *SessionData,
) providersessions.Detail {
	stats := session.ParseStats
	summary := providersessions.ParseSummary{
		LineCount:          stats.BlobCount + stats.MetaCount,
		EventCount:         stats.ReadableBlobCount,
		MalformedLineCount: stats.MalformedBlobCount + stats.MalformedMetaCount,
		UnknownEventCount:  stats.UnavailableBlobCount,
		Turns:              []providersessions.TurnSummary{},
		FunctionCalls:      []providersessions.FunctionCallSummary{},
		Reasoning:          []providersessions.ReasoningSummary{},
		ParseErrors:        parseErrorsFromStats(stats),
		UnknownEvents:      unknownEventsFromStats(stats),
		TokenUsage:         mapTokenUsage(session.TokenUsage),
	}

	return providersessions.Detail{
		ProviderSession: providersessions.Ref{
			Provider: providersessions.ProviderCursor,
			Kind:     providersessions.SessionIDKind,
			ID:       id,
		},
		Source: providersessions.SourceMetadata{
			RelativePath: relativePath,
			SizeBytes:    sizeBytes,
			ModifiedAt:   modifiedAt,
		},
		Parse:      summary,
		Transcript: transcriptFromSession(session),
	}
}

func mapTokenUsage(usage SessionTokenUsage) *providersessions.TokenUsage {
	if usage.InputTokens == nil && usage.OutputTokens == nil &&
		usage.CacheReadTokens == nil && usage.CacheWriteTokens == nil {
		return nil
	}
	mapped := &providersessions.TokenUsage{
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CacheReadTokens,
		CacheWriteTokens:  usage.CacheWriteTokens,
	}
	mapped.TotalTokens = totalTokens(usage)
	return mapped
}

func totalTokens(usage SessionTokenUsage) *int {
	total := 0
	present := false
	if usage.InputTokens != nil {
		total += *usage.InputTokens
		present = true
	}
	if usage.OutputTokens != nil {
		total += *usage.OutputTokens
		present = true
	}
	if usage.CacheReadTokens != nil {
		total += *usage.CacheReadTokens
		present = true
	}
	if usage.CacheWriteTokens != nil {
		total += *usage.CacheWriteTokens
		present = true
	}
	if !present {
		return nil
	}
	return &total
}

func transcriptFromSession(session *SessionData) []providersessions.TranscriptEntry {
	if session == nil {
		return []providersessions.TranscriptEntry{}
	}
	ordered := session.OrderedBubbles()
	transcript := make([]providersessions.TranscriptEntry, 0, len(ordered))
	for _, bubble := range ordered {
		text := truncateSessionText(bubble.DisplayText())
		if text == "" {
			continue
		}
		var timestamp *time.Time
		if ts := bubble.GetTimestamp(); !ts.IsZero() {
			utc := ts.UTC()
			timestamp = &utc
		}
		entryType := providersessions.TranscriptEntryType(bubble.TranscriptEntryType())
		transcript = append(transcript, providersessions.TranscriptEntry{
			Order:      len(transcript) + 1,
			SourceType: stringPtrIfNotEmpty("cursor_bubble"),
			Text:       stringPtrIfNotEmpty(text),
			Timestamp:  timestamp,
			Type:       entryType,
		})
	}
	return transcript
}

func parseErrorsFromStats(stats SessionParseStats) []providersessions.LineError {
	if stats.MalformedBlobCount+stats.MalformedMetaCount == 0 {
		return []providersessions.LineError{}
	}
	return []providersessions.LineError{
		{
			LineNumber: 1,
			Message:    "cursor session store contained malformed blob or meta records",
		},
	}
}

func unknownEventsFromStats(stats SessionParseStats) []providersessions.UnknownEvent {
	if stats.UnavailableBlobCount == 0 {
		return []providersessions.UnknownEvent{}
	}
	events := make([]providersessions.UnknownEvent, 0, stats.UnavailableBlobCount)
	for range stats.UnavailableBlobCount {
		events = append(events, providersessions.UnknownEvent{
			Type:        stringPtrIfNotEmpty("cursor_blob"),
			PayloadType: stringPtrIfNotEmpty("unavailable"),
		})
	}
	return events
}

func truncateSessionText(value string) string {
	const maxSessionSummaryTextLength = 1000
	if len(value) <= maxSessionSummaryTextLength {
		return value
	}
	return value[:maxSessionSummaryTextLength] + "..."
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
