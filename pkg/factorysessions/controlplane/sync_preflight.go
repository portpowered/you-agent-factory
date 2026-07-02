package controlplane

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// SyncPreflightTarget resolves one live session for sync-preflight reads.
type SyncPreflightTarget struct {
	Session    *factorysessions.LiveSession
	Remapped   bool
	Unresolved bool
}

// SyncPreflightHost exposes composition-root seams for live session sync preflight.
type SyncPreflightHost interface {
	ResolveSyncPreflightTarget(
		sessionID string,
		logicalResolve *interfaces.FactorySessionLogicalResolveHint,
	) (SyncPreflightTarget, error)
	BackendScopeID() string
	StreamGenerationID(session *factorysessions.LiveSession) string
	LiveSessionEvents(session *factorysessions.LiveSession) []factoryapi.FactoryEvent
}

// GetLiveFactorySessionSyncPreflight validates reconnect cursors and session identity
// before live event-stream recovery.
func GetLiveFactorySessionSyncPreflight(
	_ context.Context,
	host SyncPreflightHost,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	response := newSyncPreflightResponse(sessionID, reconnect)
	if IsDurableExecutionSessionID(sessionID) {
		response.ReasonCode = factoryapi.SessionNotFound
		return response, nil
	}
	if host == nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, errors.New("factory session gateway is required")
	}

	resolved, err := host.ResolveSyncPreflightTarget(sessionID, logicalResolve)
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	if resolved.Unresolved {
		response.ReasonCode = factoryapi.LogicalSessionUnresolved
		if logicalResolve != nil {
			response.BackendScopeId = stringPointer(logicalResolve.BackendScopeID)
			response.LogicalSessionKeyId = stringPointer(logicalResolve.LogicalSessionKeyID)
		}
		return response, nil
	}
	if resolved.Session == nil {
		response.ReasonCode = factoryapi.SessionNotFound
		return response, nil
	}
	session := resolved.Session

	response.BackendScopeId = stringPointer(host.BackendScopeID())
	response.LogicalSessionKeyId = stringPointer(logicalSessionKeyID(session))
	response.FactorySessionId = stringPointer(session.ID)
	response.StreamGenerationId = stringPointer(host.StreamGenerationID(session))
	if resolved.Remapped {
		response.ReasonCode = factoryapi.LogicalSessionRemap
		return response, nil
	}

	if !response.ReconnectCursor.Provided {
		response.ReasonCode = factoryapi.Ok
		response.CheckpointReusable = true
		return response, nil
	}

	eventsSnapshot := host.LiveSessionEvents(session)
	_, err = events.BuildReconnectReplay(
		eventsSnapshot,
		*reconnect,
		interfaces.FactoryEventReconnectScope{SessionID: session.ID},
	)
	if err != nil {
		if errors.Is(err, events.ErrReconnectCursorNotFound) {
			response.ReasonCode = factoryapi.CursorStale
			return response, nil
		}
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}

	response.ReasonCode = factoryapi.Ok
	response.CheckpointReusable = true
	response.ReconnectCursor.ValidForStreamGeneration = true
	return response, nil
}

func newSyncPreflightResponse(
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) factoryapi.FactorySessionSyncPreflightResponse {
	response := factoryapi.FactorySessionSyncPreflightResponse{
		RequestedSessionId: strings.TrimSpace(sessionID),
		ReasonCode:         factoryapi.SessionNotFound,
		ReconnectCursor: factoryapi.FactorySessionSyncPreflightReconnectCursor{
			Provided: reconnect != nil && (strings.TrimSpace(reconnect.AfterEventID) != "" || reconnect.AfterSequence != nil),
		},
	}
	if reconnect == nil {
		return response
	}
	if afterEventID := strings.TrimSpace(reconnect.AfterEventID); afterEventID != "" {
		response.ReconnectCursor.AfterEventId = &afterEventID
	}
	if reconnect.AfterSequence != nil {
		value := int64(*reconnect.AfterSequence)
		response.ReconnectCursor.AfterSequence = &value
	}
	return response
}

// LogicalSessionKeyID derives the stable logical session key for sync preflight reads.
func LogicalSessionKeyID(session *factorysessions.LiveSession) string {
	return logicalSessionKeyID(session)
}

func logicalSessionKeyID(session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	folderPath := filepath.Clean(strings.TrimSpace(session.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	targetKind := strings.TrimSpace(string(session.Target.Kind))
	targetName := strings.TrimSpace(session.Target.Name)
	if targetKind == "" {
		targetKind = string(factorysessions.TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}

func stringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
