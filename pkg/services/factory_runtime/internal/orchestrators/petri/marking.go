package petri

import (
	"maps"

	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Marking represents the complete state of tokens across places in a petri net.
// It is the single source of truth. The factory loop reads and writes markings.
type Marking struct {
	Tokens                   map[string]*factorytoken.Token    `json:"tokens"`       // token ID → token (all live tokens)
	PlaceTokens              map[string][]string               `json:"place_tokens"` // place ID → token IDs (index for fast lookup)
	ParentChildRegistrations ParentChildRegistrationProjection `json:"parent_child_registrations,omitempty"`
	TickCount                int                               `json:"tick_count"`
	WorkflowID               string                            `json:"workflow_id"`
	TraceContext             map[string]string                 `json:"trace_context"` // workflow-level trace metadata
}

// ParentChildRegistrationSet is the ordered runtime projection of the
// parent-scoped child work items admitted by canonical work-request facts.
// Complete is deliberately explicit: a fan-in guard must not infer that the
// currently visible children are the complete registered population.
type ParentChildRegistrationSet struct {
	Children []factorytoken.Token `json:"children"`
	Complete bool                 `json:"complete"`
}

// ParentChildRegistrationProjection stores registration facts by parent work
// ID. The child slice preserves canonical admission order for deterministic
// snapshots and guard evaluation.
type ParentChildRegistrationProjection map[string]ParentChildRegistrationSet

// NewMarking creates an empty Marking for the given workflow.
func NewMarking(workflowID string) *Marking {
	return &Marking{
		Tokens:                   make(map[string]*factorytoken.Token),
		PlaceTokens:              make(map[string][]string),
		ParentChildRegistrations: make(ParentChildRegistrationProjection),
		WorkflowID:               workflowID,
		TraceContext:             make(map[string]string),
	}
}

// RecordParentChildRegistration appends one admitted child to the ordered
// parent-scoped registration projection. Completion is recorded separately so
// callers can add every child from an atomic request before exposing the set to
// the scheduler.
func (m *Marking) RecordParentChildRegistration(token *factorytoken.Token) {
	if m == nil || token == nil || token.Color.ParentID == "" || token.Color.WorkID == "" || token.Color.DataType == factorytoken.DataTypeResource {
		return
	}
	if m.ParentChildRegistrations == nil {
		m.ParentChildRegistrations = make(ParentChildRegistrationProjection)
	}
	parentID := token.Color.ParentID
	set := m.ParentChildRegistrations[parentID]
	identity := registrationTokenIdentity(*token)
	for _, child := range set.Children {
		if registrationTokenIdentity(child) == identity {
			return
		}
	}
	set.Children = append(set.Children, deepCopyToken(token))
	set.Complete = false
	m.ParentChildRegistrations[parentID] = set
}

// CompleteParentChildRegistration publishes the complete registration fact
// for a parent after all children from the current canonical request have been
// recorded. A later request may append another ordered fact and temporarily
// reopen the set until that request is complete.
func (m *Marking) CompleteParentChildRegistration(parentID string) {
	if m == nil || parentID == "" || len(m.ParentChildRegistrations[parentID].Children) == 0 {
		return
	}
	set := m.ParentChildRegistrations[parentID]
	set.Complete = true
	m.ParentChildRegistrations[parentID] = set
}

// AddToken adds a token to the marking and updates the place index.
func (m *Marking) AddToken(token *factorytoken.Token) {
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
func (m *Marking) TokensInPlace(placeID string) []factorytoken.Token {
	ids := m.PlaceTokens[placeID]
	tokens := make([]factorytoken.Token, 0, len(ids))
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
	Tokens                   map[string]*factorytoken.Token    `json:"tokens"`
	PlaceTokens              map[string][]string               `json:"place_tokens"`
	ParentChildRegistrations ParentChildRegistrationProjection `json:"parent_child_registrations,omitempty"`
	TickCount                int                               `json:"tick_count"`
	WorkflowID               string                            `json:"workflow_id"`
	TraceContext             map[string]string                 `json:"trace_context"`
}

// Snapshot returns a deep copy of the marking as an immutable MarkingSnapshot.
func (m *Marking) Snapshot() MarkingSnapshot {
	tokens := make(map[string]*factorytoken.Token, len(m.Tokens))
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

	registrations := make(ParentChildRegistrationProjection, len(m.ParentChildRegistrations))
	for parentID, set := range m.ParentChildRegistrations {
		children := make([]factorytoken.Token, len(set.Children))
		for i := range set.Children {
			children[i] = deepCopyToken(&set.Children[i])
		}
		registrations[parentID] = ParentChildRegistrationSet{
			Children: children,
			Complete: set.Complete,
		}
	}

	return MarkingSnapshot{
		Tokens:                   tokens,
		PlaceTokens:              placeTokens,
		ParentChildRegistrations: registrations,
		TickCount:                m.TickCount,
		WorkflowID:               m.WorkflowID,
		TraceContext:             traceCtx,
	}
}

func registrationTokenIdentity(token factorytoken.Token) string {
	if token.Color.WorkID != "" {
		return "work:" + token.Color.WorkID
	}
	return "token:" + token.ID
}

// TokensInPlace returns all tokens in the given place from the snapshot.
func (s *MarkingSnapshot) TokensInPlace(placeID string) []factorytoken.Token {
	ids := s.PlaceTokens[placeID]
	tokens := make([]factorytoken.Token, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.Tokens[id]; ok {
			tokens = append(tokens, *t)
		}
	}
	return tokens
}

// deepCopyToken creates a deep copy of a Token.
func deepCopyToken(t *factorytoken.Token) factorytoken.Token {
	cp := *t

	// Deep copy Color
	if t.Color.Tags != nil {
		cp.Color.Tags = make(map[string]string, len(t.Color.Tags))
		maps.Copy(cp.Color.Tags, t.Color.Tags)
	}
	if t.Color.Relations != nil {
		cp.Color.Relations = make([]work.Relation, len(t.Color.Relations))
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
		cp.History.FailureLog = make([]factorytoken.Failure, len(t.History.FailureLog))
		copy(cp.History.FailureLog, t.History.FailureLog)
	}

	return cp
}
