package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
)

// LoadCurrentBoardFacts reads the durable canonical witness only for legacy
// discovery. Ordinary reference-based startup retains the lightweight probe.
func (s *JavaScriptRuntimeService) LoadCurrentBoardFacts(ctx context.Context, sessionID string) ([]factorydefinitions.FactoryEvent, error) {
	if s == nil {
		return nil, ErrSessionNotFound
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	store, err := s.persistenceForRead()
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, errors.New("current board durable witness is unavailable")
	}
	data, readErr := store.Load(id)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var snapshot PersistedRuntimeSessionState
	if readErr != nil || json.Unmarshal(data, &snapshot) != nil || strings.TrimSpace(snapshot.Session.SessionID) != id {
		return nil, &ResumeError{Outcome: ResumeOutcomeCorruptedPersistence, SessionID: id, Message: "current board durable witness could not be read"}
	}
	raw, _, _, _ := runtimeHistoryFromPersistedSnapshot(snapshot)
	events := make([]factorydefinitions.FactoryEvent, 0, len(raw))
	for _, fact := range raw {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var event factorydefinitions.FactoryEvent
		if json.Unmarshal(fact, &event) != nil || event.Id == "" || event.Type == "" {
			return nil, &ResumeError{Outcome: ResumeOutcomeCorruptedPersistence, SessionID: id, Message: "current board durable witness contains an invalid canonical event"}
		}
		events = append(events, event)
	}
	return events, nil
}

// LoadCurrentBoard reads the reference through this opening's persistence
// owner. It does not consult the process-global Current Factory route.
func (s *JavaScriptRuntimeService) LoadCurrentBoard(ctx context.Context, factoryDirectory string) (string, error) {
	store, err := s.currentBoardStore()
	if err != nil {
		return "", err
	}
	return store.LoadCurrentBoard(ctx, factoryDirectory)
}

// SaveCurrentBoard publishes after reconstruction and before activation.
func (s *JavaScriptRuntimeService) SaveCurrentBoard(ctx context.Context, factoryDirectory, artifact string) error {
	store, err := s.currentBoardStore()
	if err != nil {
		return err
	}
	return store.SaveCurrentBoard(ctx, factoryDirectory, artifact)
}

func (s *JavaScriptRuntimeService) currentBoardStore() (runtimepersist.CurrentBoardStore, error) {
	if s == nil {
		return nil, ErrSessionNotFound
	}
	store, ok := s.persistence.(runtimepersist.CurrentBoardStore)
	if !ok {
		return nil, errors.New("current board reference persistence is unavailable")
	}
	return store, nil
}
