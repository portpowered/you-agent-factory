package token_transformer

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	factorypkg "github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	contentcontract "github.com/portpowered/infinite-you/pkg/work/content/contract"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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
	ArcIndex            int
	Arcs                []petri.Arc
	ConsumedTokens      []factorytoken.Token
	InputColors         []factorytoken.Color
	Output              string
	WorkPropagationMode interfaces.WorkPropagationMode
	WorkstationName     string
	WorkstationType     string
	Outcome             workerexecution.WorkOutcome
	TransitionID        string
	Error               string
	Feedback            string
	Now                 time.Time
	History             factorytoken.History
	ResourceTokenIndex  int
}

// InitialTokenFromSubmit converts a submit request into a token placed in the
// work type's initial place unless the request targets a specific state.
func (t *Transformer) InitialTokenFromSubmit(req work.SubmitRequest, now time.Time) (*factorytoken.Token, error) {
	placeID, err := t.submitPlaceID(req)
	if err != nil {
		return nil, err
	}

	workID := req.WorkID
	if workID == "" {
		workID = t.nextWorkID(req.WorkTypeID)
	}

	return &factorytoken.Token{
		ID:      t.nextSubmitTokenID(req.WorkTypeID),
		PlaceID: placeID,
		Color: factorytoken.Color{
			Name:                     req.Name,
			RequestID:                req.RequestID,
			WorkID:                   workID,
			WorkTypeID:               req.WorkTypeID,
			DataType:                 factorytoken.DataTypeWork,
			ChainingTraceDepth:       chainingTraceDepthForSubmit(req),
			CurrentChainingTraceID:   firstNonEmpty(req.CurrentChainingTraceID, req.TraceID),
			PreviousChainingTraceIDs: work.CanonicalChainingTraceIDs(req.PreviousChainingTraceIDs),
			TraceID:                  req.TraceID,
			ParentID:                 submitParentID(req.Relations),
			Tags:                     factorypkg.CloneRuntimeTags(req.Tags),
			Relations:                factorypkg.CloneRuntimeRelations(req.Relations),
			Content:                  cloneWorkContent(req.Content),
			Payload:                  factorypkg.CloneRuntimePayload(req.Payload),
			InvocationArguments:      work.CloneInvocationArguments(req.InvocationArguments),
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
func (t *Transformer) InitialTokenFromColor(color factorytoken.Color, tokenID string, now time.Time) (*factorytoken.Token, error) {
	initialPlaceID, err := t.initialPlaceID(color.WorkTypeID)
	if err != nil {
		return nil, err
	}

	return &factorytoken.Token{
		ID:        tokenID,
		PlaceID:   initialPlaceID,
		Color:     factorytoken.CloneColor(color),
		CreatedAt: now,
		EnteredAt: now,
		History:   newTokenHistory(),
	}, nil
}

// SpawnedToken converts a spawned work color into a token placed in that work
// type's initial place, with a transformer-assigned token ID.
func (t *Transformer) SpawnedToken(color factorytoken.Color, parentTransitionID string, now time.Time) (*factorytoken.Token, error) {
	return t.InitialTokenFromColor(color, t.nextTokenID("spawn-"+parentTransitionID), now)
}

// FanoutCountToken creates the synthetic count token used by fanout guards.
func (t *Transformer) FanoutCountToken(countPlaceID, transitionID, parentWorkID string, expectedCount int, now time.Time) *factorytoken.Token {
	return &factorytoken.Token{
		ID:      t.nextTokenID("fanout-count-" + transitionID),
		PlaceID: countPlaceID,
		Color: factorytoken.Color{
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
func (t *Transformer) ReleasedResourceToken(consumed factorytoken.Token, placeID string, now time.Time) *factorytoken.Token {
	released := factorytoken.Clone(consumed)
	released.PlaceID = placeID
	released.EnteredAt = now
	return &released
}

// OutputToken converts consumed input tokens plus an output arc into the token
// that should be created on that arc.
func (t *Transformer) OutputToken(in OutputTokenInput) (*factorytoken.Token, error) {
	if in.ArcIndex < 0 || in.ArcIndex >= len(in.Arcs) {
		return nil, fmt.Errorf("arc index %d out of range", in.ArcIndex)
	}

	arc := in.Arcs[in.ArcIndex]
	color, err := t.resolveOutputColor(in.ArcIndex, in.Arcs, in.InputColors)
	if err != nil {
		return nil, err
	}

	if color.DataType != factorytoken.DataTypeResource {
		place := t.places[arc.PlaceID]
		targetTypeID := ""
		if place != nil {
			targetTypeID = place.TypeID
		}
		if err := applyOutputPayloadPropagation(&color, in, targetTypeID, place, t.workTypes); err != nil {
			return nil, err
		}
	}

	if token := reuseConsumedResourceToken(in, arc, color); token != nil {
		return token, nil
	}

	token := &factorytoken.Token{
		ID:        color.WorkID,
		PlaceID:   arc.PlaceID,
		Color:     color,
		CreatedAt: createdAtForOutputToken(in.ConsumedTokens, color, in.Now),
		EnteredAt: in.Now,
		History:   factorytoken.CloneHistory(in.History),
	}

	applyOutputOutcome(token, in, t.places[arc.PlaceID], t.workTypes)
	return token, nil
}

func applyOutputPayloadPropagation(
	color *factorytoken.Color,
	in OutputTokenInput,
	targetTypeID string,
	place *petri.Place,
	workTypes map[string]*state.WorkType,
) error {
	if color == nil {
		return nil
	}
	applyOutputInvocationArguments(color, in.InputColors)
	mode := in.WorkPropagationMode
	if mode == "" {
		mode = interfaces.WorkPropagationModeOutputAsPayload
	}
	switch mode {
	case interfaces.WorkPropagationModePreserveInput:
		return ApplyPreservedInputToColor(color, in.InputColors, targetTypeID, in.WorkstationName)
	default:
		if shouldPreserveRequestWorkPayloadOnOutcome(in, place, workTypes, color.WorkTypeID) {
			return nil
		}
		if in.Output != "" {
			color.Payload = []byte(in.Output)
		}
		if shouldShapeWorkContentFromWorkerOutput(in, place, workTypes, color.WorkTypeID) {
			if err := applyWorkerOutputContent(color, in.Output); err != nil {
				return err
			}
		}
		return nil
	}
}

func shouldPreserveRequestWorkPayloadOnOutcome(
	in OutputTokenInput,
	place *petri.Place,
	workTypes map[string]*state.WorkType,
	workTypeID string,
) bool {
	switch in.Outcome {
	case workerexecution.OutcomeFailed:
		return true
	case workerexecution.OutcomeRejected:
		return isRejectedFailurePlace(place, workTypes, workTypeID)
	default:
		return false
	}
}

func shouldShapeWorkContentFromWorkerOutput(
	in OutputTokenInput,
	place *petri.Place,
	workTypes map[string]*state.WorkType,
	workTypeID string,
) bool {
	if in.WorkstationType == interfaces.WorkstationTypeClassify {
		return false
	}
	switch in.Outcome {
	case workerexecution.OutcomeAccepted, workerexecution.OutcomeContinue:
		return true
	case workerexecution.OutcomeRejected:
		return !isRejectedFailurePlace(place, workTypes, workTypeID)
	default:
		return false
	}
}

func applyWorkerOutputContent(color *factorytoken.Color, output string) error {
	if color == nil {
		return nil
	}
	content, err := contentcontract.ContentFromWorkerOutput(output)
	if err != nil {
		return fmt.Errorf("shape workstation response content: %w", err)
	}
	if len(content) > 0 {
		color.Content = work.CloneWorkContentParts(content)
	}
	return nil
}

func applyOutputInvocationArguments(color *factorytoken.Color, inputColors []factorytoken.Color) {
	if color == nil || color.InvocationArguments != nil {
		return
	}
	source := firstNonResourceInput(inputColors)
	if source == nil || source.InvocationArguments == nil {
		return
	}
	color.InvocationArguments = work.CloneInvocationArguments(source.InvocationArguments)
}

// PreserveInputApplicationError reports invalid PRESERVE_INPUT routing.
type PreserveInputApplicationError struct {
	WorkstationName string
}

func (e *PreserveInputApplicationError) Error() string {
	name := e.WorkstationName
	if name == "" {
		name = "workstation"
	}
	return fmt.Sprintf(
		`workstation %q cannot apply work propagation PRESERVE_INPUT: preserve-input requires consumed non-resource input work`,
		name,
	)
}

// ApplyPreservedInputToColor copies payload, content, and tags from the selected
// consumed input work onto a routed output color when they are not already set.
func ApplyPreservedInputToColor(
	color *factorytoken.Color,
	inputColors []factorytoken.Color,
	targetTypeID string,
	workstationName string,
) error {
	if color == nil {
		return nil
	}
	if len(color.Payload) > 0 {
		return nil
	}
	source := SelectedPreserveInputSource(inputColors, targetTypeID)
	if source == nil {
		return &PreserveInputApplicationError{WorkstationName: workstationName}
	}
	color.Payload = factorypkg.CloneRuntimePayload(source.Payload)
	if len(color.Content) == 0 && len(source.Content) > 0 {
		color.Content = work.CloneWorkContentParts(source.Content)
	}
	if len(color.Tags) == 0 && len(source.Tags) > 0 {
		color.Tags = factorypkg.CloneRuntimeTags(source.Tags)
	}
	if color.InvocationArguments == nil {
		color.InvocationArguments = work.CloneInvocationArguments(source.InvocationArguments)
	}
	return nil
}

// SelectedPreserveInputSource resolves the consumed input work used for preserve-input routing.
func SelectedPreserveInputSource(inputColors []factorytoken.Color, targetTypeID string) *factorytoken.Color {
	if source := findMatchingInput(inputColors, targetTypeID); source != nil {
		return source
	}
	return firstNonResourceInput(inputColors)
}

func reuseConsumedResourceToken(in OutputTokenInput, arc petri.Arc, color factorytoken.Color) *factorytoken.Token {
	if color.DataType != factorytoken.DataTypeResource {
		return nil
	}
	consumed := matchingConsumedResourceToken(in.ConsumedTokens, color.WorkTypeID, in.ResourceTokenIndex)
	if consumed == nil {
		return nil
	}
	token := factorytoken.Clone(*consumed)
	token.PlaceID = arc.PlaceID
	token.EnteredAt = in.Now
	return &token
}

func applyOutputOutcome(
	token *factorytoken.Token,
	in OutputTokenInput,
	place *petri.Place,
	workTypes map[string]*state.WorkType,
) {
	switch in.Outcome {
	case workerexecution.OutcomeContinue:
		setOutputFeedbackTag(token, "continue_feedback", in.Feedback)
	case workerexecution.OutcomeRejected:
		setOutputFeedbackTag(token, interfaces.RejectionFeedback, in.Feedback)
		if isRejectedFailurePlace(place, workTypes, token.Color.WorkTypeID) {
			appendOutputFailure(token, in.TransitionID, in.Feedback, in.Now)
		}
	case workerexecution.OutcomeFailed:
		appendOutputFailure(token, in.TransitionID, in.Error, in.Now)
	}
}

func setOutputFeedbackTag(token *factorytoken.Token, key, value string) {
	if token.Color.Tags == nil {
		token.Color.Tags = make(map[string]string)
	}
	token.Color.Tags[key] = value
}

func appendOutputFailure(token *factorytoken.Token, transitionID, message string, now time.Time) {
	token.History.LastError = message
	token.History.FailureLog = append(token.History.FailureLog, factorytoken.Failure{
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

func (t *Transformer) submitPlaceID(req work.SubmitRequest) (string, error) {
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

func (t *Transformer) resolveOutputColor(arcIdx int, arcs []petri.Arc, inputColors []factorytoken.Color) (factorytoken.Color, error) {
	arc := arcs[arcIdx]

	targetTypeID := ""
	if place, ok := t.places[arc.PlaceID]; ok && place != nil {
		targetTypeID = place.TypeID
	}

	if targetTypeID != "" {
		if _, isWorkType := t.workTypes[targetTypeID]; !isWorkType {
			for _, color := range inputColors {
				if color.WorkTypeID == targetTypeID {
					return factorytoken.Color{
						WorkTypeID: targetTypeID,
						WorkID:     color.WorkID,
						DataType:   factorytoken.DataTypeResource,
					}, nil
				}
			}
		}
	}

	if matched := findMatchingInput(inputColors, targetTypeID); matched != nil {
		color := factorytoken.CloneColor(*matched)
		ensureWorkOutputDataType(&color, targetTypeID, t.workTypes)
		return color, nil
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

	color := factorytoken.Color{
		WorkTypeID:               targetTypeID,
		WorkID:                   t.nextWorkID(targetTypeID),
		Name:                     name,
		RequestID:                requestID,
		ChainingTraceDepth:       factorytoken.ChainingTraceDepthFromColors(inputColors),
		CurrentChainingTraceID:   traceID,
		PreviousChainingTraceIDs: factorytoken.PreviousChainingTraceIDsFromColors(inputColors),
		TraceID:                  traceID,
		ParentID:                 parentID,
	}
	ensureWorkOutputDataType(&color, targetTypeID, t.workTypes)
	return color, nil
}

// ensureWorkOutputDataType marks routed outputs into registered work-type places as
// Work tokens. Cross-type transitions that clone a consumed Work token can inherit
// an empty DataType; mixed Work/resource dispatches must still emit Work lineage.
func ensureWorkOutputDataType(color *factorytoken.Color, targetTypeID string, workTypes map[string]*state.WorkType) {
	if color == nil || targetTypeID == "" || color.DataType == factorytoken.DataTypeResource {
		return
	}
	if _, isWorkType := workTypes[targetTypeID]; isWorkType {
		color.DataType = factorytoken.DataTypeWork
	}
}

func (t *Transformer) nextWorkID(workTypeID string) string {
	if t.workIDGen != nil {
		return t.workIDGen.Next(workTypeID)
	}
	return uuid.NewString()
}

func findMatchingInput(inputs []factorytoken.Color, targetTypeID string) *factorytoken.Color {
	for i := range inputs {
		if inputs[i].WorkTypeID == targetTypeID {
			return &inputs[i]
		}
	}
	return nil
}

func firstNonResourceInput(inputs []factorytoken.Color) *factorytoken.Color {
	for i := range inputs {
		if inputs[i].DataType != factorytoken.DataTypeResource && inputs[i].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			return &inputs[i]
		}
	}
	for i := range inputs {
		if inputs[i].DataType != factorytoken.DataTypeResource {
			return &inputs[i]
		}
	}
	return nil
}

func matchingConsumedResourceToken(consumedTokens []factorytoken.Token, resourceTypeID string, resourceTokenIndex int) *factorytoken.Token {
	if resourceTokenIndex < 0 {
		resourceTokenIndex = 0
	}
	matchIndex := 0
	for i := range consumedTokens {
		if consumedTokens[i].Color.DataType != factorytoken.DataTypeResource {
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

func createdAtForOutputToken(consumedTokens []factorytoken.Token, outputColor factorytoken.Color, now time.Time) time.Time {
	for _, consumed := range consumedTokens {
		if consumed.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if consumed.Color.WorkTypeID == outputColor.WorkTypeID && consumed.Color.WorkID == outputColor.WorkID {
			return consumed.CreatedAt
		}
	}
	return now
}

func cloneWorkContent(content []work.WorkContentPart) []work.WorkContentPart {
	if len(content) == 0 {
		return nil
	}
	clone := make([]work.WorkContentPart, len(content))
	copy(clone, content)
	return clone
}

func newTokenHistory() factorytoken.History {
	return factorytoken.History{
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

func chainingTraceDepthForSubmit(req work.SubmitRequest) int {
	if req.ChainingTraceDepth > 0 {
		return req.ChainingTraceDepth
	}
	if firstNonEmpty(req.CurrentChainingTraceID, req.TraceID) != "" {
		return 1
	}
	return 0
}

func submitParentID(relations []work.Relation) string {
	for _, relation := range relations {
		if relation.Type == work.RelationParentChild && relation.TargetWorkID != "" {
			return relation.TargetWorkID
		}
	}
	return ""
}
