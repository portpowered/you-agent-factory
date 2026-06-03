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
		ParseErrors:        []factoryapi.ProviderSessionLineError{},
		UnknownEvents:      []factoryapi.ProviderSessionUnknownEvent{},
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
		Transcript: []factoryapi.ProviderSessionTranscriptEntry{},
	}
}
