package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"

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
		// Petri's persistence owner synthesizes these two lifecycle projections;
		// their identities are not identities in the Recordings event ledger.
		if strings.EqualFold(snapshot.Session.OrchestratorKind, "PETRI") &&
			((event.Type == factorydefinitions.FactoryEventTypeSessionStarted && event.Id == "session-started/"+id) ||
				(event.Type == factorydefinitions.FactoryEventTypeSessionResultUpdated && event.Id == "session-result-updated/"+id)) {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

// MatchCurrentBoardWork compares the persisted Work witness to a Recordings
// projection. Petri snapshots synthesize lifecycle events; those events are
// not recording identities and must never select a history by themselves.
func (s *JavaScriptRuntimeService) MatchCurrentBoardWork(ctx context.Context, sessionID string, state *factorydefinitions.FactoryWorldState) (bool, error) {
	if s == nil || state == nil {
		return false, ErrSessionNotFound
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	store, err := s.persistenceForRead()
	s.mu.RUnlock()
	if err != nil {
		return false, err
	}
	if store == nil {
		return false, errors.New("current board durable witness is unavailable")
	}
	data, err := store.Load(id)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var snapshot PersistedRuntimeSessionState
	if err != nil || json.Unmarshal(data, &snapshot) != nil || snapshot.Session.SessionID != id {
		return false, &ResumeError{Outcome: ResumeOutcomeCorruptedPersistence, SessionID: id, Message: "current board durable witness could not be read"}
	}
	_, _, mutations, summaries := runtimeHistoryFromPersistedSnapshot(snapshot)
	matched := 0
	for _, mutation := range mutations {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if mutation.Token == nil || mutation.Token.Color.DataType != workers.DataTypeWork {
			continue
		}
		token := mutation.Token
		item, ok := state.WorkItemsByID[token.Color.WorkID]
		if !ok || !currentBoardTokenMatches(*token, item) || !currentBoardRequestMatches(token.Color.RequestID, item.ID, state) ||
			!currentBoardRelationsMatch(token.Color.Relations, state.RelationsByWorkID[item.ID]) {
			return false, nil
		}
		matched++
	}
	for _, summary := range summaries {
		item, ok := state.WorkItemsByID[summary.WorkID]
		if !ok || item.WorkTypeID != summary.WorkTypeID || item.State != summary.State ||
			item.DisplayName != summary.Name || item.TraceID != summary.TraceID || item.ParentID != summary.ParentID ||
			!currentBoardRequestMatches(summary.RequestID, item.ID, state) {
			return false, nil
		}
		matched++
	}
	return matched > 0, nil
}

func currentBoardRelationsMatch(saved []work.Relation, recorded []work.FactoryRelation) bool {
	if len(saved) != len(recorded) {
		return false
	}
	for _, relation := range saved {
		found := false
		for _, candidate := range recorded {
			if string(relation.Type) == candidate.Type && relation.TargetWorkID == candidate.TargetWorkID && relation.RequiredState == candidate.RequiredState {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func currentBoardRequestMatches(requestID, workID string, state *factorydefinitions.FactoryWorldState) bool {
	request, ok := state.WorkRequestsByID[requestID]
	if !ok {
		return false
	}
	for _, item := range request.WorkItems {
		if item.ID == workID {
			return true
		}
	}
	return false
}

func currentBoardTokenMatches(token workers.Token, item work.FactoryWorkItem) bool {
	if item.ID != token.Color.WorkID || item.WorkTypeID != token.Color.WorkTypeID || item.State != token.State ||
		item.DisplayName != token.Color.Name || item.TraceID != token.Color.TraceID || item.ParentID != token.Color.ParentID ||
		!reflect.DeepEqual(item.Tags, token.Color.Tags) || !reflect.DeepEqual(item.Content, token.Color.Content) ||
		!reflect.DeepEqual(item.StructuredResult, token.Color.StructuredResult) || item.StructuredResultPresent != token.Color.StructuredResultPresent {
		return false
	}
	var payload strings.Builder
	for _, part := range item.Content {
		if part.Type.Normalized() == work.WorkContentPartTypeText {
			payload.WriteString(part.Text)
		}
	}
	return payload.String() == string(token.Color.Payload)
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
