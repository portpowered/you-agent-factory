package petri

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
	Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) (matched []factorytoken.Token, ok bool)
}

// ClockedGuard is implemented by guards whose result depends on runtime time.
type ClockedGuard interface {
	Guard
	EvaluateAt(now time.Time, candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) (matched []factorytoken.Token, ok bool)
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
func (g *MatchColorGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists {
		return nil, false
	}

	boundValue := tokenColorField(bound.Color, g.MatchField)

	var matched []factorytoken.Token
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
func (g *SameNameGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists || bound == nil || bound.Color.Name == "" {
		return nil, false
	}

	var matched []factorytoken.Token
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

// SameTraceIDGuard matches candidate tokens whose canonical trace identity
// equals the canonical trace identity of another bound input token. Canonical
// trace identity prefers CurrentChainingTraceID and falls back to legacy
// TraceID. Missing bindings or missing trace identity fail closed.
type SameTraceIDGuard struct {
	MatchBinding string // name of the bound arc to compare against (e.g., "planItem:ready:to:match-items")
}

var _ Guard = (*SameTraceIDGuard)(nil)

// Evaluate returns all candidates whose canonical trace identity equals the
// bound token's canonical trace identity. The guard fails when the binding is
// missing or either side has no usable trace identity.
func (g *SameTraceIDGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists || bound == nil {
		return nil, false
	}

	boundTraceID := canonicalTraceIdentity(bound.Color)
	if boundTraceID == "" {
		return nil, false
	}

	var matched []factorytoken.Token
	for _, candidate := range candidates {
		if canonicalTraceIdentity(candidate.Color) != boundTraceID {
			continue
		}
		matched = append(matched, candidate)
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
var _ RuntimeGuard = (*AllGuard)(nil)

func (g *AllGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
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

func (g *AllGuard) EvaluateRuntime(ctx RuntimeGuardContext, candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	current := candidates
	for _, guard := range g.Guards {
		if guard == nil {
			continue
		}
		var (
			matched []factorytoken.Token
			ok      bool
		)
		switch typed := guard.(type) {
		case RuntimeGuard:
			matched, ok = typed.EvaluateRuntime(ctx, current, bindings, marking)
		case ClockedGuard:
			matched, ok = typed.EvaluateAt(ctx.Now, current, bindings, marking)
		default:
			matched, ok = guard.Evaluate(current, bindings, marking)
		}
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

func (g *MatchesFieldsGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
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

	var matched []factorytoken.Token
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

// LogicalRoundTripPolicy describes the two workstation transitions whose raw
// visit counts form one logical review cycle. The pair is compiled only after
// Factory validation has established that it contains exactly two distinct
// workstation IDs.
type LogicalRoundTripPolicy struct {
	Transitions  [2]string
	MaxRawVisits int
}

// VisitCountLimit identifies the limit that made a visit-count guard match.
type VisitCountLimit string

const (
	VisitCountLimitLogical VisitCountLimit = "logical_visit_limit"
	VisitCountLimitRaw     VisitCountLimit = "absolute_raw_visit_limit"
)

// VisitCountDecision is the pure result of evaluating one token against a
// visit-count policy. Counts are retained so callers can report the selected
// limit without consulting mutable runtime state.
type VisitCountDecision struct {
	Matched       bool
	RawVisits     int
	LogicalVisits int
	MaxVisits     int
	MaxRawVisits  int
	Limit         VisitCountLimit
}

// VisitCountGuard checks that a candidate token's visit count for a specific
// transition has reached or exceeded a threshold. Used by EXHAUSTION transitions
// to route tokens that have been retried too many times.
type VisitCountGuard struct {
	TransitionID      string                  // which transition's visit count to check
	MaxVisits         int                     // fixed ceiling for the threshold
	MaxVisitsArgument string                  // optional invocation argument that tightens the ceiling
	LogicalRoundTrip  *LogicalRoundTripPolicy // optional paired logical-cycle policy
}

var _ Guard = (*VisitCountGuard)(nil)

// Evaluate returns all candidates whose visit-count policy is exhausted.
func (g *VisitCountGuard) Evaluate(candidates []factorytoken.Token, _ map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	var matched []factorytoken.Token
	for _, c := range candidates {
		if g.Decision(c).Matched {
			matched = append(matched, c)
		}
	}

	return matched, len(matched) > 0
}

// Decision evaluates one token without mutating the guard or token. In
// logical round-trip mode the smaller paired count is the number of completed
// logical cycles; the raw count remains the sum of both sides for the hard
// backstop. The logical limit wins when both limits are met on the same
// transition because the raw limit is the independent safety backstop.
func (g *VisitCountGuard) Decision(candidate factorytoken.Token) VisitCountDecision {
	maximum := g.effectiveMaxVisits(candidate)
	if g.LogicalRoundTrip == nil {
		visits := candidate.History.TotalVisits[g.TransitionID]
		return VisitCountDecision{
			Matched:       visits >= maximum,
			RawVisits:     visits,
			LogicalVisits: visits,
			MaxVisits:     maximum,
		}
	}

	first := candidate.History.TotalVisits[g.LogicalRoundTrip.Transitions[0]]
	second := candidate.History.TotalVisits[g.LogicalRoundTrip.Transitions[1]]
	rawVisits := first + second
	logicalVisits := first
	if second < logicalVisits {
		logicalVisits = second
	}
	decision := VisitCountDecision{
		RawVisits:     rawVisits,
		LogicalVisits: logicalVisits,
		MaxVisits:     maximum,
		MaxRawVisits:  g.LogicalRoundTrip.MaxRawVisits,
	}
	if logicalVisits >= maximum {
		decision.Matched = true
		decision.Limit = VisitCountLimitLogical
		return decision
	}
	if rawVisits >= g.LogicalRoundTrip.MaxRawVisits {
		decision.Matched = true
		decision.Limit = VisitCountLimitRaw
	}
	return decision
}

// LimitReason returns a stable, actionable reason for a matched visit-count
// decision. It returns an empty string while the policy remains below both
// thresholds.
func (g *VisitCountGuard) LimitReason(candidate factorytoken.Token) string {
	decision := g.Decision(candidate)
	switch decision.Limit {
	case VisitCountLimitLogical:
		return fmt.Sprintf("logical visit limit reached: %d >= %d", decision.LogicalVisits, decision.MaxVisits)
	case VisitCountLimitRaw:
		return fmt.Sprintf("absolute raw-visit backstop reached: %d >= %d", decision.RawVisits, decision.MaxRawVisits)
	default:
		return ""
	}
}

func (g *VisitCountGuard) effectiveMaxVisits(candidate factorytoken.Token) int {
	maximum := g.MaxVisits
	argumentName := strings.TrimSpace(g.MaxVisitsArgument)
	if argumentName == "" || candidate.Color.InvocationArguments == nil {
		return maximum
	}
	argument, ok := candidate.Color.InvocationArguments.Arguments[argumentName]
	if !ok || len(argument.Values) != 1 {
		return maximum
	}
	value, err := strconv.Atoi(strings.TrimSpace(argument.Values[0]))
	if err != nil || value <= 0 || value > maximum {
		return maximum
	}
	return value
}

// AllWithParentGuard matches all candidates whose ParentID matches a bound token's WorkID.
// Used with CardinalityAll arcs to collect all child tokens for a parent work item
// (e.g., collecting all completed code-change tokens for a request).
type AllWithParentGuard struct {
	MatchBinding string // name of the bound arc holding the parent token (e.g., "work")
}

var _ Guard = (*AllWithParentGuard)(nil)
var _ RuntimeGuard = (*AllWithParentGuard)(nil)

// Evaluate returns all candidates whose ParentID equals the WorkID of the
// bound token identified by MatchBinding.
func (g *AllWithParentGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	return g.evaluate(RuntimeGuardContext{}, candidates, bindings, marking)
}

// EvaluateRuntime checks the complete parent-scoped child population before
// returning the terminal candidates used by the fan-in transition. The
// terminal input place is only the binding surface; it is not the population
// denominator because processing children may be in another place or in an
// active dispatch.
func (g *AllWithParentGuard) EvaluateRuntime(ctx RuntimeGuardContext, candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	if ctx.ParentChildRegistrations == nil {
		return nil, false
	}
	return g.evaluate(ctx, candidates, bindings, marking)
}

func (g *AllWithParentGuard) evaluate(ctx RuntimeGuardContext, candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists || bound == nil || bound.Color.WorkID == "" {
		return nil, false
	}

	parentWorkID := bound.Color.WorkID
	matched := matchingParentChildren(candidates, parentWorkID, "")
	if len(matched) == 0 {
		return nil, false
	}

	if !allTokensTerminal(ctx, matched) {
		return nil, false
	}

	if ctx.ParentChildRegistrations != nil {
		return evaluateRegisteredParentChildren(ctx, parentWorkID, matched, marking)
	}

	return evaluateVisibleParentChildren(ctx, parentWorkID, firstWorkTypeID(matched), matched, marking)
}

func evaluateRegisteredParentChildren(ctx RuntimeGuardContext, parentWorkID string, matched []factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	registration, known := ctx.ParentChildRegistrations[parentWorkID]
	if !known || !registration.Complete || len(registration.Children) == 0 {
		return nil, false
	}

	registered := parentChildTokens(marking, ctx.ActiveDispatches, parentWorkID, "")
	registeredIDs := tokenIdentitySet(registration.Children)
	if len(registered) != len(registeredIDs) {
		return nil, false
	}

	targetPlaces := tokenPlaceSet(matched)
	if !registeredChildrenTerminal(ctx, registered, registeredIDs, targetPlaces) {
		return nil, false
	}
	if !tokensRegistered(matched, registeredIDs) {
		return nil, false
	}

	// The guarded arc is the binding surface for the transition and may
	// expose only one of several terminal places. The registration projection
	// above is the completeness denominator; requiring its identities to equal
	// matchedIDs would reject valid fan-in when another registered child is
	// terminal in a different place.
	return matched, true
}

func evaluateVisibleParentChildren(ctx RuntimeGuardContext, parentWorkID, childWorkTypeID string, matched []factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	registered := parentChildTokens(marking, ctx.ActiveDispatches, parentWorkID, childWorkTypeID)
	if len(registered) == 0 {
		// Preserve direct guard use with no runtime snapshot. The scheduler
		// always supplies a marking, where the full-population check below is
		// authoritative.
		if marking == nil {
			return matched, true
		}
		return nil, false
	}

	matchedIDs := tokenIdentitySet(matched)
	targetPlaces := tokenPlaceSet(matched)
	if !visibleChildrenTerminal(ctx, registered, matchedIDs, targetPlaces) {
		return nil, false
	}

	return matched, true
}

func allTokensTerminal(ctx RuntimeGuardContext, tokens []factorytoken.Token) bool {
	if ctx.StateCategoryForPlace == nil {
		return true
	}
	for _, token := range tokens {
		if ctx.StateCategoryForPlace(token.PlaceID) != runtimeStateCategoryTerminal {
			return false
		}
	}
	return true
}

func registeredChildrenTerminal(ctx RuntimeGuardContext, registered map[string]factorytoken.Token, registeredIDs, targetPlaces map[string]bool) bool {
	for identity, child := range registered {
		if !registeredIDs[identity] || !childIsTerminal(ctx, child, targetPlaces) {
			return false
		}
	}
	return true
}

func visibleChildrenTerminal(ctx RuntimeGuardContext, registered map[string]factorytoken.Token, matchedIDs, targetPlaces map[string]bool) bool {
	if len(matchedIDs) != len(registered) {
		return false
	}
	for identity, child := range registered {
		if !matchedIDs[identity] || !childIsTerminal(ctx, child, targetPlaces) {
			return false
		}
	}
	return true
}

func childIsTerminal(ctx RuntimeGuardContext, child factorytoken.Token, targetPlaces map[string]bool) bool {
	if ctx.StateCategoryForPlace != nil {
		return ctx.StateCategoryForPlace(child.PlaceID) == runtimeStateCategoryTerminal
	}
	return targetPlaces[child.PlaceID]
}

func tokensRegistered(tokens []factorytoken.Token, registeredIDs map[string]bool) bool {
	for _, token := range tokens {
		if !registeredIDs[tokenIdentity(token)] {
			return false
		}
	}
	return true
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
func (g *AnyWithParentGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	bound, exists := bindings[g.MatchBinding]
	if !exists {
		return nil, false
	}

	parentWorkID := bound.Color.WorkID

	for _, c := range candidates {
		if c.Color.ParentID == parentWorkID {
			return []factorytoken.Token{c}, true
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
func (g *DependencyGuard) Evaluate(candidates []factorytoken.Token, _ map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	if marking == nil {
		return nil, false
	}

	workIndex := dependencyWorkIndex(marking)

	var matched []factorytoken.Token
	for _, c := range candidates {
		if g.allDependenciesMet(c, workIndex) {
			matched = append(matched, c)
		}
	}

	return matched, len(matched) > 0
}

// AllDependenciesMet verifies DEPENDS_ON relations on every token in a
// complete transition binding. It is evaluated after peer guards have selected
// all joined inputs, so a secondary input cannot bypass its own dependency.
func (g *DependencyGuard) AllDependenciesMet(bindings map[string][]factorytoken.Token, marking *MarkingSnapshot) bool {
	if marking == nil {
		return false
	}

	workIndex := dependencyWorkIndex(marking)
	for _, tokens := range bindings {
		for _, token := range tokens {
			if !g.allDependenciesMet(token, workIndex) {
				return false
			}
		}
	}
	return true
}

func dependencyWorkIndex(marking *MarkingSnapshot) map[string]*factorytoken.Token {
	workIndex := make(map[string]*factorytoken.Token, len(marking.Tokens))
	for _, tok := range marking.Tokens {
		workIndex[tok.Color.WorkID] = tok
	}
	return workIndex
}

// allDependenciesMet checks that every DEPENDS_ON relation on the token is
// satisfied: the target token exists and is in the required state place.
func (g *DependencyGuard) allDependenciesMet(tok factorytoken.Token, workIndex map[string]*factorytoken.Token) bool {
	for _, rel := range tok.Color.Relations {
		if rel.Type != work.RelationDependsOn {
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
func (g *FanoutCountGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
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
	var matched []factorytoken.Token
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

const runtimeStateCategoryTerminal = "TERMINAL"

func matchingParentChildren(candidates []factorytoken.Token, parentWorkID, childWorkTypeID string) []factorytoken.Token {
	matched := make([]factorytoken.Token, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Color.ParentID != parentWorkID {
			continue
		}
		if childWorkTypeID != "" && candidate.Color.WorkTypeID != childWorkTypeID {
			continue
		}
		matched = append(matched, candidate)
	}
	return matched
}

func firstWorkTypeID(tokens []factorytoken.Token) string {
	for _, token := range tokens {
		if token.Color.WorkTypeID != "" {
			return token.Color.WorkTypeID
		}
	}
	return ""
}

func parentChildTokens(marking *MarkingSnapshot, activeDispatches map[string]*interfaces.DispatchEntry, parentWorkID, childWorkTypeID string) map[string]factorytoken.Token {
	children := make(map[string]factorytoken.Token)
	if marking != nil {
		for _, token := range marking.Tokens {
			if token == nil || !isRegisteredChild(*token, parentWorkID, childWorkTypeID) {
				continue
			}
			children[tokenIdentity(*token)] = *token
		}
	}
	for _, dispatch := range activeDispatches {
		if dispatch == nil {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			runtimeToken := factorytoken.FromWorker(token)
			if !isRegisteredChild(runtimeToken, parentWorkID, childWorkTypeID) {
				continue
			}
			identity := tokenIdentity(runtimeToken)
			if _, exists := children[identity]; !exists {
				children[identity] = runtimeToken
			}
		}
	}
	return children
}

func isRegisteredChild(token factorytoken.Token, parentWorkID, childWorkTypeID string) bool {
	if token.Color.DataType == factorytoken.DataTypeResource || token.Color.ParentID != parentWorkID || token.Color.WorkID == "" {
		return false
	}
	return childWorkTypeID == "" || token.Color.WorkTypeID == childWorkTypeID
}

func tokenIdentity(token factorytoken.Token) string {
	if token.Color.WorkID != "" {
		return "work:" + token.Color.WorkID
	}
	return "token:" + token.ID
}

func tokenIdentitySet(tokens []factorytoken.Token) map[string]bool {
	identities := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		identities[tokenIdentity(token)] = true
	}
	return identities
}

func tokenPlaceSet(tokens []factorytoken.Token) map[string]bool {
	places := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		places[token.PlaceID] = true
	}
	return places
}

// tokenColorField returns the value of a named field on a TokenColor.
// Supported fields: work_id, work_type_id, trace_id, parent_id.
// Returns empty string for unknown fields.
func tokenColorField(color factorytoken.Color, field string) string {
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

func resolveTokenSelector(token factorytoken.Token, selector string) (string, bool) {
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

func canonicalTraceIdentity(color factorytoken.Color) string {
	if color.CurrentChainingTraceID != "" {
		return color.CurrentChainingTraceID
	}
	return color.TraceID
}
