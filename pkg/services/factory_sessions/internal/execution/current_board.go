package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

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
	return currentBoardFacts(ctx, id, snapshot)
}

func currentBoardFacts(ctx context.Context, id string, snapshot PersistedRuntimeSessionState) ([]factorydefinitions.FactoryEvent, error) {
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
	return matchCurrentBoardSnapshot(ctx, snapshot, state)
}

func matchCurrentBoardSnapshot(ctx context.Context, snapshot PersistedRuntimeSessionState, state *factorydefinitions.FactoryWorldState) (bool, error) {
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
		if !currentBoardWorkTokenMatches(*token, state) {
			return false, nil
		}
		matched++
	}
	if !currentBoardSummariesMatch(summaries, state) {
		return false, nil
	}
	matched += len(summaries)
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

// QuarantineCurrentBoard preserves only this acquired default board's evidence.
// It never consults the process-global or another session's persistence route.
func (s *JavaScriptRuntimeService) QuarantineCurrentBoard(ctx context.Context, at time.Time, identity string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	store, err := s.currentBoardStore()
	if err != nil {
		return "", "", err
	}
	quarantine, ok := store.(runtimepersist.CurrentBoardQuarantineStore)
	paths, hasPaths := store.(runtimepersist.SnapshotPathResolver)
	if !ok || !hasPaths {
		return "", "", errors.New("current board quarantine persistence is unavailable")
	}
	archive, err := quarantine.QuarantineCurrentBoard(ctx, at, identity)
	if err != nil {
		return "", "", err
	}
	return paths.SnapshotPath("~default"), archive, nil
}

func (s *JavaScriptRuntimeService) currentBoardStore() (runtimepersist.CurrentBoardStore, error) {
	if s == nil {
		return nil, ErrSessionNotFound
	}
	persistence := s.persistence
	if scoped, ok := persistence.(*ScopePersistence); ok {
		// The implicit board belongs to the acquired default session. Resolve
		// its registered store, never a peer or the process-global route.
		scope := scoped.scope("~default")
		if scope == nil || scope.retiring {
			return nil, ErrSessionNotFound
		}
		persistence = scope.store
	}
	store, ok := persistence.(runtimepersist.CurrentBoardStore)
	if !ok {
		return nil, errors.New("current board reference persistence is unavailable")
	}
	return store, nil
}

func currentBoardWorkTokenMatches(token workers.Token, state *factorydefinitions.FactoryWorldState) bool {
	item, ok := state.WorkItemsByID[token.Color.WorkID]
	return ok && currentBoardTokenMatches(token, item) && currentBoardRequestMatches(token.Color.RequestID, item.ID, state) &&
		currentBoardRelationsMatch(token.Color.Relations, state.RelationsByWorkID[item.ID])
}

func currentBoardSummariesMatch(summaries []PetriTokenSummary, state *factorydefinitions.FactoryWorldState) bool {
	for _, summary := range summaries {
		item, ok := state.WorkItemsByID[summary.WorkID]
		if !ok || item.WorkTypeID != summary.WorkTypeID || item.State != summary.State ||
			item.DisplayName != summary.Name || item.TraceID != summary.TraceID || item.ParentID != summary.ParentID ||
			!currentBoardRequestMatches(summary.RequestID, item.ID, state) {
			return false
		}
	}
	return true
}

// QuarantineCurrentBoardArtifact uses only the acquired default persistence
// scope, as for snapshot quarantine; no process-global routing is consulted.
func (s *JavaScriptRuntimeService) QuarantineCurrentBoardArtifact(ctx context.Context, at time.Time, identity, artifact string) (string, string, error) {
	store, err := s.currentBoardStore()
	if err != nil {
		return "", "", err
	}
	quarantine, ok := store.(interface {
		QuarantineCurrentBoardArtifact(context.Context, time.Time, string, string) (string, string, error)
	})
	if !ok {
		return "", "", errors.New("current board artifact quarantine persistence is unavailable")
	}
	return quarantine.QuarantineCurrentBoardArtifact(ctx, at, identity, artifact)
}
