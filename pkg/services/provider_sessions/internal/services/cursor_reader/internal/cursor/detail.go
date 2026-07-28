// Package cursor is the parent-private Cursor Reader implementation that
// discovers, parses, and maps cursor-agent CLI storage without shelling out to
// an external parser.
package cursor

import (
	"context"
	"errors"
	"fmt"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
)

// LoadDetails resolves a Cursor session_id from server-configured cursor-agent storage.
func LoadDetails(ctx context.Context, files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CursorWalkDirectory, resolveSymlinks providersessionsinternal.CursorResolveSymlinks, openSQLDatabase providersessionsinternal.CursorOpenSQLDatabase, root AgentStorageRoot, id string) (providersessions.Detail, error) {
	ins := newInspection(ctx)
	if err := ins.checkCanceled(); err != nil {
		return providersessions.Detail{}, providersessions.ErrOperationCanceled
	}
	if err := ValidateSessionID(id); err != nil {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}

	resolved, err := ResolveStoreDB(ins, files, walkDirectory, resolveSymlinks, root, id)
	if err != nil {
		switch {
		case errors.Is(err, providersessions.ErrOperationCanceled), errors.Is(err, context.Canceled):
			return providersessions.Detail{}, providersessions.ErrOperationCanceled
		case errors.Is(err, providersessions.ErrResourceLimitExceeded):
			return providersessions.Detail{}, limitLookupError(root, err)
		case errors.Is(err, ErrInvalidSessionID):
			return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
		case errors.Is(err, ErrSessionNotFound):
			return providersessions.Detail{}, providersessions.ErrSessionNotFound
		case errors.Is(err, ErrAmbiguousSession):
			return providersessions.Detail{}, providersessions.ErrAmbiguousSessionFile
		default:
			return providersessions.Detail{}, fmt.Errorf("resolve cursor session store: %s", sanitizeStructuralError(err.Error()))
		}
	}

	session, err := LoadSessionData(ins, files, openSQLDatabase, resolved)
	if err != nil {
		switch {
		case errors.Is(err, providersessions.ErrOperationCanceled), errors.Is(err, context.Canceled):
			return providersessions.Detail{}, providersessions.ErrOperationCanceled
		case errors.Is(err, providersessions.ErrResourceLimitExceeded):
			if session != nil {
				info, statErr := files.Stat(resolved.AbsolutePath)
				if statErr == nil {
					modifiedAt := info.ModTime().UTC()
					return mapSessionToProviderSessionDetail(ins, id, resolved.RelativePath, info.Size(), &modifiedAt, session), nil
				}
				return mapSessionToProviderSessionDetail(ins, id, resolved.RelativePath, 0, nil, session), nil
			}
			return providersessions.Detail{}, limitLookupError(root, err)
		default:
			return providersessions.Detail{}, fmt.Errorf("load cursor session data: %s", sanitizeStructuralError(err.Error()))
		}
	}

	info, err := files.Stat(resolved.AbsolutePath)
	if err != nil {
		return providersessions.Detail{}, fmt.Errorf("stat cursor session store: %s", sanitizeStructuralError(err.Error()))
	}
	modifiedAt := info.ModTime().UTC()

	return mapSessionToProviderSessionDetail(ins, id, resolved.RelativePath, info.Size(), &modifiedAt, session), nil
}

func limitLookupError(root AgentStorageRoot, err error) error {
	return &providersessions.LookupError{
		Provider: providersessions.ProviderCursor,
		Root:     string(root),
		Err:      err,
	}
}

func mapSessionToProviderSessionDetail(
	ins *inspection,
	id string,
	relativePath string,
	sizeBytes int64,
	modifiedAt *time.Time,
	session *SessionData,
) providersessions.Detail {
	stats := session.ParseStats
	ins.mergeStats(&stats)
	facts := reconstructSessionFacts(ins, session)
	unknownCount := stats.UnavailableBlobCount + facts.unknownCount + ins.unknownRecords
	summary := providersessions.ParseSummary{
		LineCount:          stats.BlobCount + stats.MetaCount,
		EventCount:         len(facts.transcript),
		MalformedLineCount: stats.MalformedBlobCount + stats.MalformedMetaCount,
		UnknownEventCount:  unknownCount,
		Turns:              facts.turns,
		FunctionCalls:      facts.functionCalls,
		Reasoning:          facts.reasoning,
		ParseErrors:        ins.parseErrors(parseErrorsFromStats(stats)),
		UnknownEvents:      unknownEvents(unknownCount),
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
		Transcript: facts.transcript,
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

func unknownEvents(count int) []providersessions.UnknownEvent {
	if count == 0 {
		return []providersessions.UnknownEvent{}
	}
	events := make([]providersessions.UnknownEvent, 0, count)
	for range count {
		events = append(events, providersessions.UnknownEvent{
			Type:        stringPtrIfNotEmpty("cursor_record"),
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
