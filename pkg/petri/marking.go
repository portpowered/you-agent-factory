package petri

import (
	"maps"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// Marking represents the complete state of tokens across places in a petri net.
// It is the single source of truth. The factory loop reads and writes markings.
type Marking struct {
	Tokens       map[string]*interfaces.Token `json:"tokens"`       // token ID → token (all live tokens)
	PlaceTokens  map[string][]string          `json:"place_tokens"` // place ID → token IDs (index for fast lookup)
	TickCount    int                          `json:"tick_count"`
	WorkflowID   string                       `json:"workflow_id"`
	TraceContext map[string]string            `json:"trace_context"` // workflow-level trace metadata
}

// NewMarking creates an empty Marking for the given workflow.
func NewMarking(workflowID string) *Marking {
	return &Marking{
		Tokens:       make(map[string]*interfaces.Token),
		PlaceTokens:  make(map[string][]string),
		WorkflowID:   workflowID,
		TraceContext: make(map[string]string),
	}
}

// AddToken adds a token to the marking and updates the place index.
func (m *Marking) AddToken(token *interfaces.Token) {
	m.Tokens[token.ID] = token
	m.PlaceTokens[token.PlaceID] = append(m.PlaceTokens[token.PlaceID], token.ID)
}

// RemoveToken removes a token from the marking and updates the place index.
func (m *Marking) RemoveToken(tokenID string) {
	token, ok := m.Tokens[tokenID]
	if !ok {
		return
	}

	m.removeTokenFromPlaceIndex(token.PlaceID, tokenID)
	delete(m.Tokens, tokenID)
}

func (m *Marking) removeTokenFromPlaceIndex(placeID, tokenID string) {
	ids := m.PlaceTokens[placeID]
	for i, id := range ids {
		if id == tokenID {
			m.PlaceTokens[placeID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(m.PlaceTokens[placeID]) == 0 {
		delete(m.PlaceTokens, placeID)
	}
}

// TokensInPlace returns all tokens currently in the given place.
func (m *Marking) TokensInPlace(placeID string) []interfaces.Token {
	ids := m.PlaceTokens[placeID]
	tokens := make([]interfaces.Token, 0, len(ids))
	for _, id := range ids {
		if t, ok := m.Tokens[id]; ok {
			tokens = append(tokens, *t)
		}
	}
	return tokens
}

// MarkingSnapshot is an immutable deep copy of a Marking, used for
// subsystem reads and history.
type MarkingSnapshot struct {
	Tokens       map[string]*interfaces.Token `json:"tokens"`
	PlaceTokens  map[string][]string          `json:"place_tokens"`
	TickCount    int                          `json:"tick_count"`
	WorkflowID   string                       `json:"workflow_id"`
	TraceContext map[string]string            `json:"trace_context"`
}

// Snapshot returns a deep copy of the marking as an immutable MarkingSnapshot.
func (m *Marking) Snapshot() MarkingSnapshot {
	tokens := make(map[string]*interfaces.Token, len(m.Tokens))
	for id, t := range m.Tokens {
		cp := deepCopyToken(t)
		tokens[id] = &cp
	}

	placeTokens := make(map[string][]string, len(m.PlaceTokens))
	for placeID, ids := range m.PlaceTokens {
		cpIDs := make([]string, len(ids))
		copy(cpIDs, ids)
		placeTokens[placeID] = cpIDs
	}

	traceCtx := make(map[string]string, len(m.TraceContext))
	maps.Copy(traceCtx, m.TraceContext)

	return MarkingSnapshot{
		Tokens:       tokens,
		PlaceTokens:  placeTokens,
		TickCount:    m.TickCount,
		WorkflowID:   m.WorkflowID,
		TraceContext: traceCtx,
	}
}

// TokensInPlace returns all tokens in the given place from the snapshot.
func (s *MarkingSnapshot) TokensInPlace(placeID string) []interfaces.Token {
	ids := s.PlaceTokens[placeID]
	tokens := make([]interfaces.Token, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.Tokens[id]; ok {
			tokens = append(tokens, *t)
		}
	}
	return tokens
}

// deepCopyToken creates a deep copy of a Token.
func deepCopyToken(t *interfaces.Token) interfaces.Token {
	cp := *t

	// Deep copy Color
	if t.Color.Tags != nil {
		cp.Color.Tags = make(map[string]string, len(t.Color.Tags))
		maps.Copy(cp.Color.Tags, t.Color.Tags)
	}
	if t.Color.Relations != nil {
		cp.Color.Relations = make([]interfaces.Relation, len(t.Color.Relations))
		copy(cp.Color.Relations, t.Color.Relations)
	}
	if t.Color.Payload != nil {
		cp.Color.Payload = make([]byte, len(t.Color.Payload))
		copy(cp.Color.Payload, t.Color.Payload)
	}

	// Deep copy History
	if t.History.TotalVisits != nil {
		cp.History.TotalVisits = make(map[string]int, len(t.History.TotalVisits))
		maps.Copy(cp.History.TotalVisits, t.History.TotalVisits)
	}
	if t.History.ConsecutiveFailures != nil {
		cp.History.ConsecutiveFailures = make(map[string]int, len(t.History.ConsecutiveFailures))
		maps.Copy(cp.History.ConsecutiveFailures, t.History.ConsecutiveFailures)
	}
	if t.History.PlaceVisits != nil {
		cp.History.PlaceVisits = make(map[string]int, len(t.History.PlaceVisits))
		maps.Copy(cp.History.PlaceVisits, t.History.PlaceVisits)
	}
	if t.History.FailureLog != nil {
		cp.History.FailureLog = make([]interfaces.FailureRecord, len(t.History.FailureLog))
		copy(cp.History.FailureLog, t.History.FailureLog)
	}

	return cp
}
