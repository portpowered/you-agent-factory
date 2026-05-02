package token_transformer

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// Transformer centralizes token conversions for factory submit, routing, and spawn flows.
type Transformer struct {
	places    map[string]*petri.Place
	workTypes map[string]*state.WorkType
	workIDGen *petri.WorkIDGenerator
	mu        sync.Mutex
	tokenSeq  int
}

// Option configures a Transformer.
type Option func(*Transformer)

// WithWorkIDGenerator sets the generator used for new work-item IDs.
func WithWorkIDGenerator(g *petri.WorkIDGenerator) Option {
	return func(t *Transformer) {
		t.workIDGen = g
	}
}

// New creates a Transformer for the provided topology registries.
func New(places map[string]*petri.Place, workTypes map[string]*state.WorkType, opts ...Option) *Transformer {
	transformer := &Transformer{
		places:    places,
		workTypes: workTypes,
	}
	for _, opt := range opts {
		opt(transformer)
	}
	return transformer
}

// OutputTokenInput contains the data needed to convert consumed input tokens
// plus an output arc into a routed output token.
type OutputTokenInput struct {
	ArcIndex       int
	Arcs           []petri.Arc
	ConsumedTokens []interfaces.Token
	InputColors    []interfaces.TokenColor
	Output         string
	Outcome        interfaces.WorkOutcome
	TransitionID   string
	Error          string
	Feedback       string
	Now            time.Time
	History        interfaces.TokenHistory
}

// InitialTokenFromSubmit converts a submit request into a token placed in the
// work type's initial place unless the request targets a specific state.
func (t *Transformer) InitialTokenFromSubmit(req interfaces.SubmitRequest, now time.Time) (*interfaces.Token, error) {
	placeID, err := t.submitPlaceID(req)
	if err != nil {
		return nil, err
	}

	workID := req.WorkID
	if workID == "" {
		workID = t.nextWorkID(req.WorkTypeID)
	}

	return &interfaces.Token{
		ID:      t.nextSubmitTokenID(req.WorkTypeID),
		PlaceID: placeID,
		Color: interfaces.TokenColor{
			Name:                     req.Name,
			RequestID:                req.RequestID,
			WorkID:                   workID,
			WorkTypeID:               req.WorkTypeID,
			DataType:                 interfaces.DataTypeWork,
			CurrentChainingTraceID:   firstNonEmpty(req.CurrentChainingTraceID, req.TraceID),
			PreviousChainingTraceIDs: interfaces.CanonicalChainingTraceIDs(req.PreviousChainingTraceIDs),
			TraceID:                  req.TraceID,
			ParentID:                 submitParentID(req.Relations),
			Tags:                     cloneTags(req.Tags),
			Relations:                cloneRelations(req.Relations),
			Payload:                  clonePayload(req.Payload),
		},
		CreatedAt: now,
		EnteredAt: now,
		History:   newTokenHistory(),
	}, nil
}

func (t *Transformer) nextSubmitTokenID(workTypeID string) string {
	return t.nextTokenID("tok-" + workTypeID)
}

func (t *Transformer) nextTokenID(prefix string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tokenSeq++
	return fmt.Sprintf("%s-%d", prefix, t.tokenSeq)
}

// InitialTokenFromColor converts a token color into a token placed in that
// work type's initial place.
func (t *Transformer) InitialTokenFromColor(color interfaces.TokenColor, tokenID string, now time.Time) (*interfaces.Token, error) {
	initialPlaceID, err := t.initialPlaceID(color.WorkTypeID)
	if err != nil {
		return nil, err
	}

	return &interfaces.Token{
		ID:        tokenID,
		PlaceID:   initialPlaceID,
		Color:     cloneColor(color),
		CreatedAt: now,
		EnteredAt: now,
		History:   newTokenHistory(),
	}, nil
}

// SpawnedToken converts a spawned work color into a token placed in that work
// type's initial place, with a transformer-assigned token ID.
func (t *Transformer) SpawnedToken(color interfaces.TokenColor, parentTransitionID string, now time.Time) (*interfaces.Token, error) {
	return t.InitialTokenFromColor(color, t.nextTokenID("spawn-"+parentTransitionID), now)
}

// FanoutCountToken creates the synthetic count token used by fanout guards.
func (t *Transformer) FanoutCountToken(countPlaceID, transitionID, parentWorkID string, expectedCount int, now time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:      t.nextTokenID("fanout-count-" + transitionID),
		PlaceID: countPlaceID,
		Color: interfaces.TokenColor{
			ParentID: parentWorkID,
			Tags: map[string]string{
				"expected_count": fmt.Sprintf("%d", expectedCount),
			},
		},
		CreatedAt: now,
		EnteredAt: now,
		History:   newTokenHistory(),
	}
}

// ReleasedResourceToken recreates a consumed resource token in its release place
// while preserving the token's identity and metadata.
func (t *Transformer) ReleasedResourceToken(consumed interfaces.Token, placeID string, now time.Time) *interfaces.Token {
	released := cloneToken(consumed)
	released.PlaceID = placeID
	released.EnteredAt = now
	return &released
}

// OutputToken converts consumed input tokens plus an output arc into the token
// that should be created on that arc.
func (t *Transformer) OutputToken(in OutputTokenInput) (*interfaces.Token, error) {
	if in.ArcIndex < 0 || in.ArcIndex >= len(in.Arcs) {
		return nil, fmt.Errorf("arc index %d out of range", in.ArcIndex)
	}

	arc := in.Arcs[in.ArcIndex]
	color, err := t.resolveOutputColor(in.ArcIndex, in.Arcs, in.InputColors)
	if err != nil {
		return nil, err
	}

	if in.Output != "" && color.DataType != interfaces.DataTypeResource {
		color.Payload = []byte(in.Output)
	}

	if color.DataType == interfaces.DataTypeResource {
		if consumed := matchingConsumedResourceToken(in.ConsumedTokens, color.WorkTypeID); consumed != nil {
			token := cloneToken(*consumed)
			token.PlaceID = arc.PlaceID
			token.EnteredAt = in.Now
			return &token, nil
		}
	}

	token := &interfaces.Token{
		ID:        color.WorkID,
		PlaceID:   arc.PlaceID,
		Color:     color,
		CreatedAt: createdAtForOutputToken(in.ConsumedTokens, color, in.Now),
		EnteredAt: in.Now,
		History:   cloneHistory(in.History),
	}

	switch in.Outcome {
	case interfaces.OutcomeContinue:
		if token.Color.Tags == nil {
			token.Color.Tags = make(map[string]string)
		}
		token.Color.Tags["continue_feedback"] = in.Feedback
	case interfaces.OutcomeRejected:
		if token.Color.Tags == nil {
			token.Color.Tags = make(map[string]string)
		}
		token.Color.Tags[interfaces.RejectionFeedback] = in.Feedback
	case interfaces.OutcomeFailed:
		token.History.LastError = in.Error
		token.History.FailureLog = append(token.History.FailureLog, interfaces.FailureRecord{
			TransitionID: in.TransitionID,
			Error:        in.Error,
			Timestamp:    in.Now,
		})
	}

	if in.Outcome == interfaces.OutcomeRejected {
		place := t.places[arc.PlaceID]
		if place != nil && state.CategoryForState(t.workTypes, token.Color.WorkTypeID, place.State) == state.StateCategoryFailed {
			token.History.LastError = in.Feedback
			token.History.FailureLog = append(token.History.FailureLog, interfaces.FailureRecord{
				TransitionID: in.TransitionID,
				Error:        in.Feedback,
				Timestamp:    in.Now,
			})
		}
	}

	return token, nil
}

func (t *Transformer) initialPlaceID(workTypeID string) (string, error) {
	wt, ok := t.workTypes[workTypeID]
	if !ok {
		return "", fmt.Errorf("work type %q not found", workTypeID)
	}

	for _, s := range wt.States {
		if s.Category == state.StateCategoryInitial {
			return state.PlaceID(wt.ID, s.Value), nil
		}
	}

	return "", fmt.Errorf("initial place not found for work type %q", workTypeID)
}

func (t *Transformer) submitPlaceID(req interfaces.SubmitRequest) (string, error) {
	if req.TargetState == "" {
		return t.initialPlaceID(req.WorkTypeID)
	}

	if _, ok := t.workTypes[req.WorkTypeID]; !ok {
		return "", fmt.Errorf("work type %q not found", req.WorkTypeID)
	}

	placeID := state.PlaceID(req.WorkTypeID, req.TargetState)
	if _, ok := t.places[placeID]; !ok {
		return "", fmt.Errorf("target state %q not found for work type %q", req.TargetState, req.WorkTypeID)
	}
	return placeID, nil
}

func (t *Transformer) resolveOutputColor(arcIdx int, arcs []petri.Arc, inputColors []interfaces.TokenColor) (interfaces.TokenColor, error) {
	arc := arcs[arcIdx]

	targetTypeID := ""
	if place, ok := t.places[arc.PlaceID]; ok && place != nil {
		targetTypeID = place.TypeID
	}

	if targetTypeID != "" {
		if _, isWorkType := t.workTypes[targetTypeID]; !isWorkType {
			for _, color := range inputColors {
				if color.WorkTypeID == targetTypeID {
					return interfaces.TokenColor{
						WorkTypeID: targetTypeID,
						WorkID:     color.WorkID,
						DataType:   interfaces.DataTypeResource,
					}, nil
				}
			}
		}
	}

	if matched := findMatchingInput(inputColors, targetTypeID); matched != nil {
		return interfaces.TokenColor{
			WorkTypeID:               targetTypeID,
			WorkID:                   matched.WorkID,
			Name:                     matched.Name,
			RequestID:                matched.RequestID,
			CurrentChainingTraceID:   firstNonEmpty(matched.CurrentChainingTraceID, matched.TraceID),
			PreviousChainingTraceIDs: cloneStringSlice(matched.PreviousChainingTraceIDs),
			TraceID:                  matched.TraceID,
			ParentID:                 matched.ParentID,
			Tags:                     cloneTags(matched.Tags),
			Relations:                cloneRelations(matched.Relations),
			Payload:                  clonePayload(matched.Payload),
		}, nil
	}

	first := firstNonResourceInput(inputColors)

	name := ""
	traceID := ""
	requestID := ""
	parentID := ""
	if first != nil {
		unmatchedBefore := countUnmatchedBefore(arcIdx, arcs, inputColors, t.places)
		if unmatchedBefore > 0 {
			name = fmt.Sprintf("%s/%s/%d", first.Name, targetTypeID, unmatchedBefore)
		} else {
			name = first.Name
		}
		requestID = first.RequestID
		traceID = first.TraceID
		parentID = first.WorkID
	}

	return interfaces.TokenColor{
		WorkTypeID:               targetTypeID,
		WorkID:                   t.nextWorkID(targetTypeID),
		Name:                     name,
		RequestID:                requestID,
		CurrentChainingTraceID:   traceID,
		PreviousChainingTraceIDs: interfaces.PreviousChainingTraceIDsFromTokenColors(inputColors),
		TraceID:                  traceID,
		ParentID:                 parentID,
	}, nil
}

func (t *Transformer) nextWorkID(workTypeID string) string {
	if t.workIDGen != nil {
		return t.workIDGen.Next(workTypeID)
	}
	return uuid.NewString()
}

func findMatchingInput(inputs []interfaces.TokenColor, targetTypeID string) *interfaces.TokenColor {
	for i := range inputs {
		if inputs[i].WorkTypeID == targetTypeID {
			return &inputs[i]
		}
	}
	return nil
}

func firstNonResourceInput(inputs []interfaces.TokenColor) *interfaces.TokenColor {
	for i := range inputs {
		if inputs[i].DataType != interfaces.DataTypeResource && inputs[i].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			return &inputs[i]
		}
	}
	for i := range inputs {
		if inputs[i].DataType != interfaces.DataTypeResource {
			return &inputs[i]
		}
	}
	return nil
}

func matchingConsumedResourceToken(consumedTokens []interfaces.Token, resourceTypeID string) *interfaces.Token {
	for i := range consumedTokens {
		if consumedTokens[i].Color.DataType != interfaces.DataTypeResource {
			continue
		}
		if consumedTokens[i].Color.WorkTypeID == resourceTypeID {
			return &consumedTokens[i]
		}
	}
	return nil
}

func countUnmatchedBefore(arcIdx int, arcs []petri.Arc, inputs []interfaces.TokenColor, places map[string]*petri.Place) int {
	count := 0
	for i := 0; i < arcIdx; i++ {
		targetTypeID := ""
		if place, ok := places[arcs[i].PlaceID]; ok && place != nil {
			targetTypeID = place.TypeID
		}
		if findMatchingInput(inputs, targetTypeID) == nil {
			count++
		}
	}
	return count
}

func createdAtForOutputToken(consumedTokens []interfaces.Token, outputColor interfaces.TokenColor, now time.Time) time.Time {
	for _, consumed := range consumedTokens {
		if consumed.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if consumed.Color.WorkTypeID == outputColor.WorkTypeID && consumed.Color.WorkID == outputColor.WorkID {
			return consumed.CreatedAt
		}
	}
	return now
}

func newTokenHistory() interfaces.TokenHistory {
	return interfaces.TokenHistory{
		TotalVisits:         make(map[string]int),
		ConsecutiveFailures: make(map[string]int),
		PlaceVisits:         make(map[string]int),
	}
}

func cloneColor(color interfaces.TokenColor) interfaces.TokenColor {
	return interfaces.TokenColor{
		Name:                     color.Name,
		RequestID:                color.RequestID,
		WorkID:                   color.WorkID,
		WorkTypeID:               color.WorkTypeID,
		DataType:                 color.DataType,
		CurrentChainingTraceID:   color.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(color.PreviousChainingTraceIDs),
		TraceID:                  color.TraceID,
		ParentID:                 color.ParentID,
		Tags:                     cloneTags(color.Tags),
		Relations:                cloneRelations(color.Relations),
		Payload:                  clonePayload(color.Payload),
	}
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func submitParentID(relations []interfaces.Relation) string {
	for _, relation := range relations {
		if relation.Type == interfaces.RelationParentChild && relation.TargetWorkID != "" {
			return relation.TargetWorkID
		}
	}
	return ""
}

func cloneHistory(history interfaces.TokenHistory) interfaces.TokenHistory {
	cloned := interfaces.TokenHistory{
		TotalDuration: history.TotalDuration,
		LastError:     history.LastError,
	}
	if history.TotalVisits != nil {
		cloned.TotalVisits = cloneIntMap(history.TotalVisits)
	}
	if history.ConsecutiveFailures != nil {
		cloned.ConsecutiveFailures = cloneIntMap(history.ConsecutiveFailures)
	}
	if history.PlaceVisits != nil {
		cloned.PlaceVisits = cloneIntMap(history.PlaceVisits)
	}
	if history.FailureLog != nil {
		cloned.FailureLog = append([]interfaces.FailureRecord(nil), history.FailureLog...)
	}
	return cloned
}

func cloneIntMap(input map[string]int) map[string]int {
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		out[key] = value
	}
	return out
}

func cloneRelations(relations []interfaces.Relation) []interfaces.Relation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.Relation, len(relations))
	copy(out, relations)
	return out
}

func clonePayload(payload []byte) []byte {
	if payload == nil {
		return nil
	}
	return append([]byte(nil), payload...)
}

func cloneToken(token interfaces.Token) interfaces.Token {
	return interfaces.Token{
		ID:        token.ID,
		PlaceID:   token.PlaceID,
		Color:     cloneColor(token.Color),
		CreatedAt: token.CreatedAt,
		EnteredAt: token.EnteredAt,
		History:   cloneHistory(token.History),
	}
}
