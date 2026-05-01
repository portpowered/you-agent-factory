package petri

import (
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// Guard is a predicate evaluated against tokens in a place to determine
// whether a transition is enabled.  Guards are used to trigger events.
//
// Guards receive named bindings from ALL input arcs of the transition,
// so they can reference tokens from other arcs.
type Guard interface {
	// Evaluate returns the matched tokens and whether the guard is satisfied.
	// candidates are tokens from THIS arc's place.
	// bindings are tokens already matched by other input arcs (arc name → token).
	// marking is the full marking snapshot, providing world state for advanced guards.
	Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, marking *MarkingSnapshot) (matched []interfaces.Token, ok bool)
}

// ClockedGuard is implemented by guards whose result depends on runtime time.
type ClockedGuard interface {
	Guard
	EvaluateAt(now time.Time, candidates []interfaces.Token, bindings map[string]*interfaces.Token, marking *MarkingSnapshot) (matched []interfaces.Token, ok bool)
}

// MatchColorGuard matches a field on candidate tokens against a field on a bound token.
// For example, matching candidate.ParentID == bindings["work"].WorkID.
//
// The guard checks a specific field on each candidate token against a specific field
// on a token already bound by another input arc. This is the primary mechanism for
// correlating related tokens across places.
type MatchColorGuard struct {
	Field        string // field on the candidate token to check (e.g., "parent_id")
	MatchBinding string // name of the bound arc to compare against (e.g., "work")
	MatchField   string // field on the bound token to compare (e.g., "work_id")
}

var _ Guard = (*MatchColorGuard)(nil)

// Evaluate returns all candidates whose Field value equals the MatchField value
// on the bound token identified by MatchBinding.
func (g *MatchColorGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists {
		return nil, false
	}

	boundValue := tokenColorField(bound.Color, g.MatchField)

	var matched []interfaces.Token
	for _, c := range candidates {
		if tokenColorField(c.Color, g.Field) == boundValue {
			matched = append(matched, c)
		}
	}

	return matched, len(matched) > 0
}

// SameNameGuard matches candidate tokens whose authored work name equals the
// authored work name of another bound input token. Missing bindings or empty
// names fail closed.
type SameNameGuard struct {
	MatchBinding string // name of the bound arc to compare against (e.g., "planItem:ready:to:match-items")
}

var _ Guard = (*SameNameGuard)(nil)

// Evaluate returns all candidates whose authored name equals the bound token's
// authored name. The guard fails when the binding is missing or either side has
// no usable authored name.
func (g *SameNameGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists || bound == nil || bound.Color.Name == "" {
		return nil, false
	}

	var matched []interfaces.Token
	for _, candidate := range candidates {
		if candidate.Color.Name == "" {
			continue
		}
		if candidate.Color.Name == bound.Color.Name {
			matched = append(matched, candidate)
		}
	}

	return matched, len(matched) > 0
}

// AllGuard applies multiple guard predicates to the same candidate set.
// Each guard filters the candidates produced by the previous guard; all guards
// must succeed for the overall result to pass.
type AllGuard struct {
	Guards []Guard
}

var _ Guard = (*AllGuard)(nil)

func (g *AllGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, marking *MarkingSnapshot) ([]interfaces.Token, bool) {
	current := candidates
	for _, guard := range g.Guards {
		if guard == nil {
			continue
		}
		matched, ok := guard.Evaluate(current, bindings, marking)
		if !ok {
			return nil, false
		}
		current = matched
	}
	return current, len(current) > 0
}

// MatchesFieldsGuard resolves a configured selector against candidate inputs.
// When MatchBinding is empty, the guard only requires the selector to resolve on
// the candidate token. When MatchBinding is set, the selector must resolve on
// both tokens and the resulting values must match exactly.
type MatchesFieldsGuard struct {
	InputKey     string
	MatchBinding string
}

var _ Guard = (*MatchesFieldsGuard)(nil)

func (g *MatchesFieldsGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	selector := strings.TrimSpace(g.InputKey)
	if selector == "" {
		return nil, false
	}

	var boundValue string
	if g.MatchBinding != "" {
		bound, exists := bindings[g.MatchBinding]
		if !exists || bound == nil {
			return nil, false
		}
		resolved, ok := resolveTokenSelector(*bound, selector)
		if !ok {
			return nil, false
		}
		boundValue = resolved
	}

	var matched []interfaces.Token
	for _, candidate := range candidates {
		resolved, ok := resolveTokenSelector(candidate, selector)
		if !ok {
			continue
		}
		if g.MatchBinding == "" || resolved == boundValue {
			matched = append(matched, candidate)
		}
	}

	return matched, len(matched) > 0
}

// VisitCountGuard checks that a candidate token's visit count for a specific
// transition has reached or exceeded a threshold. Used by EXHAUSTION transitions
// to route tokens that have been retried too many times.
type VisitCountGuard struct {
	TransitionID string // which transition's visit count to check
	MaxVisits    int    // threshold — guard passes when TotalVisits[TransitionID] >= MaxVisits
}

var _ Guard = (*VisitCountGuard)(nil)

// Evaluate returns all candidates whose TotalVisits for the configured transition
// meets or exceeds MaxVisits.
func (g *VisitCountGuard) Evaluate(candidates []interfaces.Token, _ map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	var matched []interfaces.Token
	for _, c := range candidates {
		visits := c.History.TotalVisits[g.TransitionID]
		if visits >= g.MaxVisits {
			matched = append(matched, c)
		}
	}

	return matched, len(matched) > 0
}

// AllWithParentGuard matches all candidates whose ParentID matches a bound token's WorkID.
// Used with CardinalityAll arcs to collect all child tokens for a parent work item
// (e.g., collecting all completed code-change tokens for a request).
type AllWithParentGuard struct {
	MatchBinding string // name of the bound arc holding the parent token (e.g., "work")
}

var _ Guard = (*AllWithParentGuard)(nil)

// Evaluate returns all candidates whose ParentID equals the WorkID of the
// bound token identified by MatchBinding.
func (g *AllWithParentGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists {
		return nil, false
	}

	parentWorkID := bound.Color.WorkID

	var matched []interfaces.Token
	for _, c := range candidates {
		if c.Color.ParentID == parentWorkID {
			matched = append(matched, c)
		}
	}

	return matched, len(matched) > 0
}

// AnyWithParentGuard matches the first candidate whose ParentID matches a bound token's WorkID.
// Unlike AllWithParentGuard which collects ALL matching children, this guard fires as soon as
// any single child token is found — used for "any child failed" style routing.
type AnyWithParentGuard struct {
	MatchBinding string // name of the bound arc holding the parent token (e.g., "work")
}

var _ Guard = (*AnyWithParentGuard)(nil)

// Evaluate returns the first candidate whose ParentID equals the WorkID of the
// bound token identified by MatchBinding.
func (g *AnyWithParentGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists {
		return nil, false
	}

	parentWorkID := bound.Color.WorkID

	for _, c := range candidates {
		if c.Color.ParentID == parentWorkID {
			return []interfaces.Token{c}, true
		}
	}

	return nil, false
}

// DependencyGuard blocks a candidate token from transitioning until all its
// DEPENDS_ON relations are satisfied — i.e., each dependency token exists in
// the marking and resides in the place matching its RequiredState.
//
// Place IDs follow the convention "{work_type_id}:{state_value}".
type DependencyGuard struct{}

var _ Guard = (*DependencyGuard)(nil)

// Evaluate returns candidates whose DEPENDS_ON relations are all satisfied.
func (g *DependencyGuard) Evaluate(candidates []interfaces.Token, _ map[string]*interfaces.Token, marking *MarkingSnapshot) ([]interfaces.Token, bool) {
	if marking == nil {
		return nil, false
	}

	// Build a lookup from WorkID → token for fast dependency resolution.
	workIndex := make(map[string]*interfaces.Token, len(marking.Tokens))
	for _, tok := range marking.Tokens {
		workIndex[tok.Color.WorkID] = tok
	}

	var matched []interfaces.Token
	for _, c := range candidates {
		if g.allDependenciesMet(c, workIndex) {
			matched = append(matched, c)
		}
	}

	return matched, len(matched) > 0
}

// allDependenciesMet checks that every DEPENDS_ON relation on the token is
// satisfied: the target token exists and is in the required state place.
func (g *DependencyGuard) allDependenciesMet(tok interfaces.Token, workIndex map[string]*interfaces.Token) bool {
	for _, rel := range tok.Color.Relations {
		if rel.Type != interfaces.RelationDependsOn {
			continue
		}
		dep, ok := workIndex[rel.TargetWorkID]
		if !ok {
			return false // dependency token not found
		}
		// Construct expected place ID: "{work_type_id}:{required_state}"
		expectedPlaceID := dep.Color.WorkTypeID + ":" + rel.RequiredState
		if dep.PlaceID != expectedPlaceID {
			return false
		}
	}
	return true
}

// FanoutCountGuard validates that the number of child tokens matching a parent
// equals the expected count carried by a guard (count) token. This enables
// dynamic fanout where the child count is determined at runtime.
//
// The guard reads the expected count from the count token's Tags["expected_count"]
// (bound via CountBinding). It then matches all candidates whose ParentID equals
// the parent token's WorkID (bound via MatchBinding). The guard passes only when
// len(matched) == expectedCount.
//
// For 0-child fanout, the guard returns ([], true) — an empty but successful match.
type FanoutCountGuard struct {
	MatchBinding string // name of the bound arc holding the parent token (e.g., "parent")
	CountBinding string // name of the bound arc holding the count token (e.g., "fanout-count")
}

var _ Guard = (*FanoutCountGuard)(nil)

// Evaluate returns all candidates whose ParentID matches the parent token's WorkID,
// but only if the total count equals the expected count from the count token.
func (g *FanoutCountGuard) Evaluate(candidates []interfaces.Token, bindings map[string]*interfaces.Token, _ *MarkingSnapshot) ([]interfaces.Token, bool) {
	parent, exists := bindings[g.MatchBinding]
	if !exists {
		return nil, false
	}

	countToken, exists := bindings[g.CountBinding]
	if !exists {
		return nil, false
	}

	expectedStr, ok := countToken.Color.Tags["expected_count"]
	if !ok {
		return nil, false
	}
	expectedCount, err := strconv.Atoi(expectedStr)
	if err != nil {
		return nil, false
	}

	parentWorkID := parent.Color.WorkID

	var matched []interfaces.Token
	for _, c := range candidates {
		if c.Color.ParentID == parentWorkID {
			matched = append(matched, c)
		}
	}

	if len(matched) != expectedCount {
		return nil, false
	}

	return matched, true
}

// tokenColorField returns the value of a named field on a TokenColor.
// Supported fields: work_id, work_type_id, trace_id, parent_id.
// Returns empty string for unknown fields.
func tokenColorField(color interfaces.TokenColor, field string) string {
	switch field {
	case interfaces.WorkID:
		return color.WorkID
	case interfaces.WorkTypeID:
		return color.WorkTypeID
	case interfaces.TraceID:
		return color.TraceID
	case interfaces.ParentID:
		return color.ParentID
	default:
		return ""
	}
}

func resolveTokenSelector(token interfaces.Token, selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" || selector[0] != '.' {
		return "", false
	}

	if tagKey, ok := parseTagSelector(selector); ok {
		if token.Color.Tags == nil {
			return "", false
		}
		value, exists := token.Color.Tags[tagKey]
		if !exists {
			return "", false
		}
		return value, true
	}

	switch selector {
	case ".Name":
		return token.Color.Name, true
	case ".RequestID":
		return token.Color.RequestID, true
	case ".WorkID":
		return token.Color.WorkID, true
	case ".WorkTypeID":
		return token.Color.WorkTypeID, true
	case ".DataType":
		return string(token.Color.DataType), true
	case ".TraceID":
		return token.Color.TraceID, true
	case ".ParentID":
		return token.Color.ParentID, true
	case ".Payload":
		return string(token.Color.Payload), true
	default:
		return "", false
	}
}

func parseTagSelector(selector string) (string, bool) {
	if !strings.HasPrefix(selector, `.Tags["`) || !strings.HasSuffix(selector, `"]`) {
		return "", false
	}
	key := selector[len(`.Tags["`) : len(selector)-len(`"]`)]
	if key == "" {
		return "", false
	}
	return key, true
}
