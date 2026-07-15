package providersessioncursor

import (
	"errors"
	"fmt"
	"os"
	"time"

	cursorstorage "github.com/portpowered/infinite-you/pkg/platform/cursors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	loadableProviderSessionProvider = "cursor"
	loadableProviderSessionKind     = "session_id"
)

// LoadDetails resolves a Cursor session_id from server-configured cursor-agent storage.
func LoadDetails(root cursorstorage.AgentStorageRoot, id string) (factoryapi.ProviderSessionDetailResponse, error) {
	if err := cursorstorage.ValidateSessionID(id); err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, ErrInvalidProviderSessionIdentifier
	}

	resolved, err := cursorstorage.ResolveStoreDB(root, id)
	if err != nil {
		switch {
		case errors.Is(err, cursorstorage.ErrInvalidSessionID):
			return factoryapi.ProviderSessionDetailResponse{}, ErrInvalidProviderSessionIdentifier
		case errors.Is(err, cursorstorage.ErrSessionNotFound):
			return factoryapi.ProviderSessionDetailResponse{}, ErrProviderSessionNotFound
		case errors.Is(err, cursorstorage.ErrAmbiguousSession):
			return factoryapi.ProviderSessionDetailResponse{}, ErrAmbiguousProviderSessionFile
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

	return mapSessionToProviderSessionDetail(id, resolved.RelativePath, info.Size(), &modifiedAt, session), nil
}

func mapSessionToProviderSessionDetail(
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
		ParseErrors:        parseErrorsFromStats(stats),
		UnknownEvents:      unknownEventsFromStats(stats),
		TokenUsage:         mapTokenUsage(session.TokenUsage),
	}

	return factoryapi.ProviderSessionDetailResponse{
		ProviderSession: factoryapi.LoadableProviderSessionRef{
			Provider: factoryapi.LoadableProviderSessionProvider(loadableProviderSessionProvider),
			Kind:     factoryapi.LoadableProviderSessionKind(loadableProviderSessionKind),
			Id:       id,
		},
		Source: factoryapi.ProviderSessionSourceMetadata{
			RelativePath: relativePath,
			SizeBytes:    sizeBytes,
			ModifiedAt:   modifiedAt,
		},
		Parse:      summary,
		Transcript: transcriptFromSession(session),
	}
}

func mapTokenUsage(usage cursorstorage.SessionTokenUsage) *factoryapi.ProviderSessionTokenUsage {
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
	mapped.TotalTokens = totalTokens(usage)
	return mapped
}

func totalTokens(usage cursorstorage.SessionTokenUsage) *int {
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

func transcriptFromSession(session *cursorstorage.SessionData) []factoryapi.ProviderSessionTranscriptEntry {
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

func parseErrorsFromStats(stats cursorstorage.SessionParseStats) []factoryapi.ProviderSessionLineError {
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

func unknownEventsFromStats(stats cursorstorage.SessionParseStats) []factoryapi.ProviderSessionUnknownEvent {
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
