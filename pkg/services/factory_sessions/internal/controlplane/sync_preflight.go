package controlplane

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	LogicalSessionKeyID(session *factorysessions.LiveSession) string
	StreamGenerationID(session *factorysessions.LiveSession) string
	LiveSessionEvents(session *factorysessions.LiveSession) []interfaces.FactoryEvent
}

// GetLiveFactorySessionSyncPreflight validates reconnect cursors and session identity
// before live event-stream recovery.
func GetLiveFactorySessionSyncPreflight(
	_ context.Context,
	host SyncPreflightHost,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
	validateReconnectCursor factorysessions.ReconnectCursorValidator,
) (factorysessions.SyncPreflightResult, error) {
	response := newSyncPreflightResponse(sessionID, reconnect)
	if IsDurableExecutionSessionID(sessionID) {
		response.Reason = factorysessions.SyncPreflightReasonSessionNotFound
		return response, nil
	}
	if host == nil {
		return factorysessions.SyncPreflightResult{}, errors.New("factory session gateway is required")
	}

	resolved, err := host.ResolveSyncPreflightTarget(sessionID, logicalResolve)
	if err != nil {
		return factorysessions.SyncPreflightResult{}, err
	}
	if resolved.Unresolved {
		response.Reason = factorysessions.SyncPreflightReasonLogicalSessionUnresolved
		if logicalResolve != nil {
			response.BackendScopeID = stringPointer(logicalResolve.BackendScopeID)
			response.LogicalSessionKeyID = stringPointer(logicalResolve.LogicalSessionKeyID)
		}
		return response, nil
	}
	if resolved.Session == nil {
		response.Reason = factorysessions.SyncPreflightReasonSessionNotFound
		return response, nil
	}
	session := resolved.Session

	response.BackendScopeID = stringPointer(host.BackendScopeID())
	response.LogicalSessionKeyID = stringPointer(host.LogicalSessionKeyID(session))
	response.FactorySessionID = stringPointer(factorysessions.CanonicalFactorySessionID(session))
	response.StreamGenerationID = stringPointer(host.StreamGenerationID(session))
	if resolved.Remapped {
		response.Reason = factorysessions.SyncPreflightReasonLogicalSessionRemap
		return response, nil
	}

	if !response.ReconnectCursor.Provided {
		response.Reason = factorysessions.SyncPreflightReasonOK
		response.CheckpointReusable = true
		return response, nil
	}

	eventsSnapshot := host.LiveSessionEvents(session)
	if validateReconnectCursor == nil {
		return factorysessions.SyncPreflightResult{}, errors.New("recordings reconnect validator is required")
	}
	err = validateReconnectCursor(
		eventsSnapshot,
		*reconnect,
		interfaces.FactoryEventReconnectScope{SessionID: session.ID},
	)
	if err != nil {
		if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
			response.Reason = factorysessions.SyncPreflightReasonCursorStale
			return response, nil
		}
		return factorysessions.SyncPreflightResult{}, err
	}

	response.Reason = factorysessions.SyncPreflightReasonOK
	response.CheckpointReusable = true
	response.ReconnectCursor.ValidForStreamGeneration = true
	return response, nil
}

func newSyncPreflightResponse(
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) factorysessions.SyncPreflightResult {
	response := factorysessions.SyncPreflightResult{
		RequestedSessionID: strings.TrimSpace(sessionID),
		Reason:             factorysessions.SyncPreflightReasonSessionNotFound,
		ReconnectCursor: factorysessions.SyncPreflightReconnectCursor{
			Provided: reconnect != nil && (strings.TrimSpace(reconnect.AfterEventID) != "" || reconnect.AfterSequence != nil),
		},
	}
	if reconnect == nil {
		return response
	}
	if afterEventID := strings.TrimSpace(reconnect.AfterEventID); afterEventID != "" {
		response.ReconnectCursor.AfterEventID = &afterEventID
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
