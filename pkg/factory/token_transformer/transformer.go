package token_transformer

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	factorypkg "github.com/portpowered/infinite-you/pkg/factory"
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
	ArcIndex           int
	Arcs               []petri.Arc
	ConsumedTokens     []interfaces.Token
	InputColors        []interfaces.TokenColor
	Output             string
	Outcome            interfaces.WorkOutcome
	TransitionID       string
	Error              string
	Feedback           string
	Now                time.Time
	History            interfaces.TokenHistory
	ResourceTokenIndex int
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
			ChainingTraceDepth:       chainingTraceDepthForSubmit(req),
			CurrentChainingTraceID:   firstNonEmpty(req.CurrentChainingTraceID, req.TraceID),
			PreviousChainingTraceIDs: interfaces.CanonicalChainingTraceIDs(req.PreviousChainingTraceIDs),
			TraceID:                  req.TraceID,
			ParentID:                 submitParentID(req.Relations),
			Tags:                     factorypkg.CloneRuntimeTags(req.Tags),
			Relations:                factorypkg.CloneRuntimeRelations(req.Relations),
			Content:                  cloneWorkContent(req.Content),
			Payload:                  factorypkg.CloneRuntimePayload(req.Payload),
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
		Color:     interfaces.CloneTokenColor(color),
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
	released := interfaces.CloneToken(consumed)
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

	if token := reuseConsumedResourceToken(in, arc, color); token != nil {
		return token, nil
	}

	token := &interfaces.Token{
		ID:        color.WorkID,
		PlaceID:   arc.PlaceID,
		Color:     color,
		CreatedAt: createdAtForOutputToken(in.ConsumedTokens, color, in.Now),
		EnteredAt: in.Now,
		History:   interfaces.CloneTokenHistory(in.History),
	}

	applyOutputOutcome(token, in, t.places[arc.PlaceID], t.workTypes)
	return token, nil
}

func reuseConsumedResourceToken(in OutputTokenInput, arc petri.Arc, color interfaces.TokenColor) *interfaces.Token {
	if color.DataType != interfaces.DataTypeResource {
		return nil
	}
	consumed := matchingConsumedResourceToken(in.ConsumedTokens, color.WorkTypeID, in.ResourceTokenIndex)
	if consumed == nil {
		return nil
	}
	token := interfaces.CloneToken(*consumed)
	token.PlaceID = arc.PlaceID
	token.EnteredAt = in.Now
	return &token
}

func applyOutputOutcome(
	token *interfaces.Token,
	in OutputTokenInput,
	place *petri.Place,
	workTypes map[string]*state.WorkType,
) {
	switch in.Outcome {
	case interfaces.OutcomeContinue:
		setOutputFeedbackTag(token, "continue_feedback", in.Feedback)
	case interfaces.OutcomeRejected:
		setOutputFeedbackTag(token, interfaces.RejectionFeedback, in.Feedback)
		if isRejectedFailurePlace(place, workTypes, token.Color.WorkTypeID) {
			appendOutputFailure(token, in.TransitionID, in.Feedback, in.Now)
		}
	case interfaces.OutcomeFailed:
		appendOutputFailure(token, in.TransitionID, in.Error, in.Now)
	}
}

func setOutputFeedbackTag(token *interfaces.Token, key, value string) {
	if token.Color.Tags == nil {
		token.Color.Tags = make(map[string]string)
	}
	token.Color.Tags[key] = value
}

func appendOutputFailure(token *interfaces.Token, transitionID, message string, now time.Time) {
	token.History.LastError = message
	token.History.FailureLog = append(token.History.FailureLog, interfaces.FailureRecord{
		TransitionID: transitionID,
		Error:        message,
		Timestamp:    now,
	})
}

func isRejectedFailurePlace(place *petri.Place, workTypes map[string]*state.WorkType, workTypeID string) bool {
	if place == nil {
		return false
	}
	return state.CategoryForState(workTypes, workTypeID, place.State) == state.StateCategoryFailed
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
		return interfaces.CloneTokenColor(*matched), nil
	}

	first := firstNonResourceInput(inputColors)

	name := ""
	traceID := ""
	requestID := ""
	parentID := ""
	if first != nil {
		name = first.Name
		requestID = first.RequestID
		traceID = first.TraceID
		parentID = first.WorkID
	}

	return interfaces.TokenColor{
		WorkTypeID:               targetTypeID,
		WorkID:                   t.nextWorkID(targetTypeID),
		Name:                     name,
		RequestID:                requestID,
		ChainingTraceDepth:       interfaces.ChainingTraceDepthFromTokenColors(inputColors),
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

func matchingConsumedResourceToken(consumedTokens []interfaces.Token, resourceTypeID string, resourceTokenIndex int) *interfaces.Token {
	if resourceTokenIndex < 0 {
		resourceTokenIndex = 0
	}
	matchIndex := 0
	for i := range consumedTokens {
		if consumedTokens[i].Color.DataType != interfaces.DataTypeResource {
			continue
		}
		if consumedTokens[i].Color.WorkTypeID == resourceTypeID {
			if matchIndex == resourceTokenIndex {
				return &consumedTokens[i]
			}
			matchIndex++
		}
	}
	return nil
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

func cloneWorkContent(content []interfaces.WorkContentPart) []interfaces.WorkContentPart {
	if len(content) == 0 {
		return nil
	}
	clone := make([]interfaces.WorkContentPart, len(content))
	copy(clone, content)
	return clone
}

func newTokenHistory() interfaces.TokenHistory {
	return interfaces.TokenHistory{
		TotalVisits:         make(map[string]int),
		ConsecutiveFailures: make(map[string]int),
		PlaceVisits:         make(map[string]int),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func chainingTraceDepthForSubmit(req interfaces.SubmitRequest) int {
	if req.ChainingTraceDepth > 0 {
		return req.ChainingTraceDepth
	}
	if firstNonEmpty(req.CurrentChainingTraceID, req.TraceID) != "" {
		return 1
	}
	return 0
}

func submitParentID(relations []interfaces.Relation) string {
	for _, relation := range relations {
		if relation.Type == interfaces.RelationParentChild && relation.TargetWorkID != "" {
			return relation.TargetWorkID
		}
	}
	return ""
}
