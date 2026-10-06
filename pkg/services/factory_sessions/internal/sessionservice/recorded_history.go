package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// RecordedHistory reads the recorded inventory independently of live gateways.
type RecordedHistory interface {
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

// ListRecordedSessions keeps opening-time discovery scoped to the invocation's
// profile instead of the process-wide home resolver used for interactive lists.
func (h *recordedHistory) ListRecordedSessions(request recordings.RecordedSessionInventoryRequest) (recordings.RecordedSessionInventoryResult, error) {
	if h.inventory == nil {
		return recordings.RecordedSessionInventoryResult{}, fmt.Errorf("recorded session inventory is required")
	}
	return h.inventory.ListRecordedSessions(request)
}

func (a *Assembly) ListRecordedSessions(request recordings.RecordedSessionInventoryRequest) (recordings.RecordedSessionInventoryResult, error) {
	if a == nil {
		return recordings.RecordedSessionInventoryResult{}, fmt.Errorf("recorded session inventory is required")
	}
	inventory, ok := a.recordedHistory.(recordings.RecordedSessionInventory)
	if !ok {
		return recordings.RecordedSessionInventoryResult{}, fmt.Errorf("recorded session inventory is required")
	}
	return inventory.ListRecordedSessions(request)
}

type recordedHistory struct {
	resolveHome factorysessions.HomeDirectoryResolver
	inventory   recordings.RecordedSessionInventory
}

func NewRecordedHistory(resolveHome factorysessions.HomeDirectoryResolver, inventory recordings.RecordedSessionInventory) RecordedHistory {
	return &recordedHistory{resolveHome: resolveHome, inventory: inventory}
}

func (h *recordedHistory) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	scope := request.Scope
	if scope == "" {
		scope = factorysessions.DefaultSessionListScope
	}
	result := factorysessions.ListSessionsResult{Scope: scope}
	if !shouldIncludeRecordedHistory(scope, request.ExcludeRecordedHistory) {
		return result, nil
	}
	if h.inventory == nil {
		if scope == factorysessions.SessionListScopeHistory {
			return factorysessions.ListSessionsResult{}, fmt.Errorf("recorded session inventory is required")
		}
		return result, nil
	}
	root, err := h.recordingRoot()
	if err != nil {
		return factorysessions.ListSessionsResult{}, err
	}
	listed, err := h.inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
	if err != nil {
		return factorysessions.ListSessionsResult{}, fmt.Errorf("list recorded Factory Sessions: %w", err)
	}
	result.Warnings = append([]factorysessions.RecordedSessionDiagnostic(nil), listed.Warnings...)
	result.RecordedSessions = make([]factorysessions.RecordedSessionListSummary, 0, len(listed.Sessions))
	for _, session := range listed.Sessions {
		result.RecordedSessions = append(result.RecordedSessions, factorysessions.RecordedSessionListSummary{
			SessionID: session.FactorySessionID, Source: factorysessions.RecordedSessionListSourceHistory,
			ArtifactReference: session.ArtifactReference, Format: string(session.Format),
		})
	}
	sort.SliceStable(result.RecordedSessions, func(left, right int) bool {
		a, b := result.RecordedSessions[left], result.RecordedSessions[right]
		if a.SessionID != b.SessionID {
			return a.SessionID < b.SessionID
		}
		return a.ArtifactReference < b.ArtifactReference
	})
	return result, nil
}

func (h *recordedHistory) recordingRoot() (string, error) {
	if h.resolveHome == nil {
		return "", fmt.Errorf("recorded session home directory resolver is required")
	}
	home, err := h.resolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve recorded session home directory: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("resolve recorded session home directory: empty path")
	}
	return filepath.Join(home, ".you-agent-factory", "recordings"), nil
}
