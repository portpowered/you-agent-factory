package api

import (
	"errors"
	"fmt"
	"os"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/internal/cursorstorage"
)

const (
	loadableProviderSessionProviderCursor = "cursor"
)

func loadCursorProviderSessionDetails(root cursorstorage.AgentStorageRoot, id string) (factoryapi.ProviderSessionDetailResponse, error) {
	if err := cursorstorage.ValidateSessionID(id); err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, errInvalidProviderSessionIdentifier
	}

	resolved, err := cursorstorage.ResolveStoreDB(root, id)
	if err != nil {
		switch {
		case errors.Is(err, cursorstorage.ErrInvalidSessionID):
			return factoryapi.ProviderSessionDetailResponse{}, errInvalidProviderSessionIdentifier
		case errors.Is(err, cursorstorage.ErrSessionNotFound):
			return factoryapi.ProviderSessionDetailResponse{}, errProviderSessionNotFound
		case errors.Is(err, cursorstorage.ErrAmbiguousSession):
			return factoryapi.ProviderSessionDetailResponse{}, errAmbiguousProviderSessionFile
		default:
			return factoryapi.ProviderSessionDetailResponse{}, fmt.Errorf("resolve cursor session store: %w", err)
		}
	}

	session, err := cursorstorage.LoadSessionData(resolved)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, fmt.Errorf("load cursor session data: %w", err)
	}

	info, err := os.Stat(resolved.AbsolutePath)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, fmt.Errorf("stat cursor session store: %w", err)
	}
	modifiedAt := info.ModTime().UTC()

	parsed := mapCursorSessionToProviderSessionDetail(id, resolved.RelativePath, info.Size(), &modifiedAt, session)
	return parsed, nil
}

func mapCursorSessionToProviderSessionDetail(
	id string,
	relativePath string,
	sizeBytes int64,
	modifiedAt *time.Time,
	session *cursorstorage.SessionData,
) factoryapi.ProviderSessionDetailResponse {
	stats := session.ParseStats
	summary := factoryapi.ProviderSessionParseSummary{
		LineCount:          stats.BlobCount + stats.MetaCount,
		EventCount:         stats.ReadableBlobCount,
		MalformedLineCount: stats.MalformedBlobCount + stats.MalformedMetaCount,
		UnknownEventCount:  stats.UnavailableBlobCount,
		Turns:              []factoryapi.ProviderSessionTurnSummary{},
		FunctionCalls:      []factoryapi.ProviderSessionFunctionCallSummary{},
		Reasoning:          []factoryapi.ProviderSessionReasoningSummary{},
		ParseErrors:        cursorParseErrorsFromStats(stats),
		UnknownEvents:      cursorUnknownEventsFromStats(stats),
		TokenUsage:         mapCursorTokenUsage(session.TokenUsage),
	}

	return factoryapi.ProviderSessionDetailResponse{
		ProviderSession: factoryapi.LoadableProviderSessionRef{
			Provider: factoryapi.LoadableProviderSessionProvider(loadableProviderSessionProviderCursor),
			Kind:     factoryapi.LoadableProviderSessionKind(loadableProviderSessionKind),
			Id:       id,
		},
		Source: factoryapi.ProviderSessionSourceMetadata{
			RelativePath: relativePath,
			SizeBytes:    sizeBytes,
			ModifiedAt:   modifiedAt,
		},
		Parse:      summary,
		Transcript: cursorTranscriptFromSession(session),
	}
}

func mapCursorTokenUsage(usage cursorstorage.SessionTokenUsage) *factoryapi.ProviderSessionTokenUsage {
	if usage.InputTokens == nil && usage.OutputTokens == nil &&
		usage.CacheReadTokens == nil && usage.CacheWriteTokens == nil {
		return nil
	}
	mapped := &factoryapi.ProviderSessionTokenUsage{
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CacheReadTokens,
		CacheWriteTokens:  usage.CacheWriteTokens,
	}
	mapped.TotalTokens = cursorTotalTokens(usage)
	return mapped
}

func cursorTotalTokens(usage cursorstorage.SessionTokenUsage) *int {
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

func cursorTranscriptFromSession(session *cursorstorage.SessionData) []factoryapi.ProviderSessionTranscriptEntry {
	if session == nil {
		return []factoryapi.ProviderSessionTranscriptEntry{}
	}
	ordered := session.OrderedBubbles()
	transcript := make([]factoryapi.ProviderSessionTranscriptEntry, 0, len(ordered))
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
		entryType := factoryapi.ProviderSessionTranscriptEntryType(bubble.TranscriptEntryType())
		transcript = append(transcript, factoryapi.ProviderSessionTranscriptEntry{
			Order:      len(transcript) + 1,
			SourceType: stringPtrIfNotEmpty("cursor_bubble"),
			Text:       stringPtrIfNotEmpty(text),
			Timestamp:  timestamp,
			Type:       entryType,
		})
	}
	return transcript
}

func cursorParseErrorsFromStats(stats cursorstorage.SessionParseStats) []factoryapi.ProviderSessionLineError {
	if stats.MalformedBlobCount+stats.MalformedMetaCount == 0 {
		return []factoryapi.ProviderSessionLineError{}
	}
	return []factoryapi.ProviderSessionLineError{
		{
			LineNumber: 1,
			Message:    "cursor session store contained malformed blob or meta records",
		},
	}
}

func cursorUnknownEventsFromStats(stats cursorstorage.SessionParseStats) []factoryapi.ProviderSessionUnknownEvent {
	if stats.UnavailableBlobCount == 0 {
		return []factoryapi.ProviderSessionUnknownEvent{}
	}
	events := make([]factoryapi.ProviderSessionUnknownEvent, 0, stats.UnavailableBlobCount)
	for range stats.UnavailableBlobCount {
		events = append(events, factoryapi.ProviderSessionUnknownEvent{
			Type:        stringPtrIfNotEmpty("cursor_blob"),
			PayloadType: stringPtrIfNotEmpty("unavailable"),
		})
	}
	return events
}
