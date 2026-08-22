package factory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func hashLabel(path, text string) string {
	digest := sha256.Sum256([]byte(path + "\n" + text))
	return fmt.Sprintf("artifact-%s", hex.EncodeToString(digest[:8]))
}

// ProjectPrimaryResult maps one validated workflow result to WorkContent parts.
func ProjectPrimaryResult(sessionID string, value TypedValue, artifacts []interfaces.FactorySessionArtifactState) ([]work.WorkContentPart, Result) {
	validation := ValidateTypedValue(value)
	if validation.HasIssues() {
		return nil, validation
	}
	if len(value.JSON) == 0 {
		return nil, Result{}
	}
	var decoded any
	if err := json.Unmarshal(value.JSON, &decoded); err != nil {
		return nil, Result{Issues: []Issue{{
			Code:    CodeNonJSONValue,
			Message: "workflow result must be JSON-compatible: " + err.Error(),
			Path:    "$",
		}}}
	}
	parts, issues := projectDecodedValue(sessionID, decoded, artifacts, "$")
	if len(issues) > 0 {
		return nil, Result{Issues: issues}
	}
	if len(parts) == 0 {
		return nil, Result{}
	}
	return parts, Result{}
}

type sessionResultProjectionOperation struct{}

// NewSessionResultProjectionOperation constructs the Factory Runtime-owned
// canonical result projection operation.
func NewSessionResultProjectionOperation() SessionResultProjectionOperation {
	return sessionResultProjectionOperation{}
}

// ProjectSessionResults derives all customer-visible result facts once from a
// canonical input and returns mutually detached projections.
func (sessionResultProjectionOperation) ProjectSessionResults(input SessionResultInput) SessionResultProjection {
	durable := projectSessionResult(input)
	return SessionResultProjection{
		Live:    projectLiveSessionResult(input),
		Durable: durable,
		Updated: projectSessionResultUpdatedPayload(input.Status, durable),
	}
}

func projectLiveSessionResult(input SessionResultInput) LiveSessionResult {
	result := LiveSessionResult{
		SessionID: strings.TrimSpace(input.SessionID),
		Status:    input.Status,
	}
	if len(input.CheckpointRefs) > 0 {
		result.CheckpointRefs = cloneCheckpointRefs(input.CheckpointRefs)
	}
	if input.ResultArtifact != nil {
		copied := cloneArtifactRef(*input.ResultArtifact)
		result.ResultArtifactRef = &copied
	}
	return result
}

func projectSessionResult(input SessionResultInput) SessionResult {
	result := SessionResult{
		SessionID:    strings.TrimSpace(input.SessionID),
		ResultStatus: resultStatusFromSessionStatus(input.Status),
	}
	if parts, validation := ProjectPrimaryResult(input.SessionID, input.PrimaryValue, input.Artifacts); !validation.HasIssues() && len(parts) > 0 {
		result.PrimaryResult = work.CloneWorkContentParts(parts)
	}
	artifactIDs, artifactRefs := projectArtifactProjection(input)
	result.ArtifactIDs = artifactIDs
	result.ArtifactRefs = artifactRefs
	return result
}

func projectSessionResultUpdatedPayload(
	status interfaces.RuntimeStatus,
	sessionResult SessionResult,
) SessionResultUpdatedPayload {
	payload := SessionResultUpdatedPayload{
		ResultStatus: eventResultStatusFromSessionStatus(status),
	}
	payload.ResultSummary = work.CloneWorkContentParts(sessionResult.PrimaryResult)
	payload.ArtifactIDs = append([]string(nil), sessionResult.ArtifactIDs...)
	return payload
}

func cloneCheckpointRefs(
	refs []interfaces.FactorySessionJavaScriptCheckpointEventRef,
) []interfaces.FactorySessionJavaScriptCheckpointEventRef {
	cloned := make([]interfaces.FactorySessionJavaScriptCheckpointEventRef, len(refs))
	for index := range refs {
		cloned[index] = refs[index]
		if refs[index].Label != nil {
			value := *refs[index].Label
			cloned[index].Label = &value
		}
		if refs[index].Summary != nil {
			value := *refs[index].Summary
			cloned[index].Summary = &value
		}
		if refs[index].Timestamp != nil {
			value := *refs[index].Timestamp
			cloned[index].Timestamp = &value
		}
		if refs[index].ArtifactRef != nil {
			value := cloneArtifactRef(*refs[index].ArtifactRef)
			cloned[index].ArtifactRef = &value
		}
	}
	return cloned
}

func cloneArtifactRef(ref interfaces.FactoryArtifactRef) interfaces.FactoryArtifactRef {
	cloned := ref
	if ref.ContentHash != nil {
		value := *ref.ContentHash
		cloned.ContentHash = &value
	}
	if ref.SizeBytes != nil {
		value := *ref.SizeBytes
		cloned.SizeBytes = &value
	}
	return cloned
}

func eventResultStatusFromSessionStatus(status interfaces.RuntimeStatus) interfaces.FactorySessionResultStatus {
	if status == interfaces.RuntimeStatusFinished {
		return interfaces.FactorySessionResultStatusFinal
	}
	return interfaces.FactorySessionResultStatusPartial
}

func resultStatusFromSessionStatus(status interfaces.RuntimeStatus) ResultStatus {
	switch status {
	case interfaces.RuntimeStatusFinished:
		return ResultStatusFinal
	case interfaces.RuntimeStatusActive:
		return ResultStatusPartial
	default:
		return ResultStatusNotReady
	}
}

func projectArtifactProjection(input SessionResultInput) ([]string, []interfaces.FactoryArtifactRef) {
	seen := make(map[string]struct{})
	var artifactIDs []string
	var artifactRefs []interfaces.FactoryArtifactRef
	addArtifact := func(ref interfaces.FactoryArtifactRef) {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		artifactIDs = append(artifactIDs, id)
		artifactRefs = append(artifactRefs, cloneArtifactRef(ref))
	}
	if input.ResultArtifact != nil {
		addArtifact(*input.ResultArtifact)
	}
	for _, artifact := range input.Artifacts {
		id := strings.TrimSpace(artifact.ID)
		if id == "" {
			continue
		}
		addArtifact(interfaces.FactoryArtifactRef{
			ID:         id,
			Kind:       strings.ToUpper(strings.TrimSpace(artifact.Kind)),
			Visibility: strings.TrimSpace(artifact.Visibility),
		})
	}
	return artifactIDs, artifactRefs
}

func projectDecodedValue(
	sessionID string,
	value any,
	artifacts []interfaces.FactorySessionArtifactState,
	path string,
) ([]work.WorkContentPart, []Issue) {
	switch typed := value.(type) {
	case nil:
		return []work.WorkContentPart{jsonPart(nil, path)}, nil
	case bool, float64, json.Number:
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	case string:
		if issues := validateEmbeddedArtifactURI(sessionID, typed, path); len(issues) > 0 {
			return nil, issues
		}
		if artifact, ok := artifactForEmbeddedString(sessionID, typed, artifacts); ok {
			return []work.WorkContentPart{artifactBackedPart(sessionID, artifact, typed)}, nil
		}
		if len(typed) > DefaultMaxEmbeddedBytes {
			if artifact := syntheticLargeTextArtifact(typed, path); artifact.ID != "" {
				return []work.WorkContentPart{artifactBackedPart(sessionID, artifact, typed)}, nil
			}
		}
		return []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: typed,
		}}, nil
	case []any:
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	case map[string]any:
		if artifactID, ok := typed["artifactId"].(string); ok {
			if artifact, found := findArtifact(artifacts, artifactID); found {
				return []work.WorkContentPart{artifactBackedPart(sessionID, artifact, "")}, nil
			}
		}
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	default:
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	}
}

func validateEmbeddedArtifactURI(sessionID, value, path string) []Issue {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, ArtifactURIScheme+"://") {
		return nil
	}
	issues := ValidateArtifactURIForSession(trimmed, sessionID)
	if len(issues) == 0 {
		return nil
	}
	for index := range issues {
		if path != "" && path != "$" {
			issues[index].Path = path
		}
	}
	return issues
}

func jsonPart(value any, path string) work.WorkContentPart {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte("null")
	}
	part := work.WorkContentPart{
		Type: work.WorkContentPartTypeJSON,
		JSON: raw,
	}
	if path != "" && path != "$" {
		part.Label = strings.TrimPrefix(path, "$.")
	}
	return part
}

func artifactBackedPart(sessionID string, artifact interfaces.FactorySessionArtifactState, text string) work.WorkContentPart {
	artifactID := strings.TrimSpace(artifact.ID)
	uri := FormatArtifactURI(sessionID, artifactID)
	kind := strings.ToUpper(strings.TrimSpace(artifact.Kind))
	switch kind {
	case "IMAGE":
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeImage,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "AUDIO":
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeAudio,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "BINARY", "PATCH", "DATASET":
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeBinary,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "LOG", "FINDING", "CHECKPOINT", "CHILD_RESULT", "WORKTREE_SUMMARY":
		if strings.TrimSpace(text) != "" && len(text) <= DefaultMaxEmbeddedBytes {
			return work.WorkContentPart{
				Type:       work.WorkContentPartTypeText,
				Text:       text,
				ArtifactID: artifactID,
				Label:      artifact.Label,
			}
		}
		return work.WorkContentPart{
			Type:       work.WorkContentPartTypeText,
			Text:       uri,
			ArtifactID: artifactID,
			Label:      artifact.Label,
		}
	default:
		if strings.TrimSpace(text) != "" && len(text) <= DefaultMaxEmbeddedBytes {
			return work.WorkContentPart{
				Type:       work.WorkContentPartTypeText,
				Text:       text,
				ArtifactID: artifactID,
				Label:      firstNonEmpty(artifact.Label, "result"),
			}
		}
		return work.WorkContentPart{
			Type:       work.WorkContentPartTypeText,
			Text:       uri,
			ArtifactID: artifactID,
			Label:      firstNonEmpty(artifact.Label, "result"),
		}
	}
}

func artifactForEmbeddedString(sessionID, value string, artifacts []interfaces.FactorySessionArtifactState) (interfaces.FactorySessionArtifactState, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, ArtifactURIScheme+"://") {
		return interfaces.FactorySessionArtifactState{}, false
	}
	if issues := ValidateArtifactURIForSession(trimmed, sessionID); len(issues) > 0 {
		return interfaces.FactorySessionArtifactState{}, false
	}
	parsed, issues := ParseArtifactURI(trimmed)
	if len(issues) > 0 {
		return interfaces.FactorySessionArtifactState{}, false
	}
	artifact, ok := findArtifact(artifacts, parsed.ArtifactID)
	if !ok {
		return interfaces.FactorySessionArtifactState{ID: parsed.ArtifactID}, true
	}
	return artifact, true
}

func syntheticLargeTextArtifact(text, path string) interfaces.FactorySessionArtifactState {
	if len(text) <= DefaultMaxEmbeddedBytes {
		return interfaces.FactorySessionArtifactState{}
	}
	return interfaces.FactorySessionArtifactState{
		ID:         hashLabel(path, text),
		Kind:       "LOG",
		Visibility: "PUBLIC",
		Label:      strings.TrimPrefix(path, "$."),
		Summary:    "Large workflow text output projected as an artifact ref",
		SizeBytes:  int64(len(text)),
	}
}

func findArtifact(artifacts []interfaces.FactorySessionArtifactState, artifactID string) (interfaces.FactorySessionArtifactState, bool) {
	trimmed := strings.TrimSpace(artifactID)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ID) == trimmed {
			return artifact, true
		}
	}
	return interfaces.FactorySessionArtifactState{}, false
}

func artifactContentType(artifact interfaces.FactorySessionArtifactState) string {
	if contentType := strings.TrimSpace(artifact.CaptureMetadata["contentType"]); contentType != "" {
		return contentType
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// WorkArtifactProjectionInput contains the runtime-owned facts needed to
// project expected artifacts for a Work read. The topology stays behind the
// Factory Runtime root so Factory Sessions does not grow another Petri-shaped
// dependency while adapting the result to its Work read contract.
type WorkArtifactProjectionInput struct {
	Token           *workers.Token
	Topology        *RuntimeNet
	Dispatches      map[string]*DispatchEntry
	DispatchHistory []CompletedDispatch
	Results         []workers.WorkResult
}

// WorkArtifactProjection derives the customer-facing expected-artifact read
// model from runtime-owned topology and dispatch facts.
type WorkArtifactProjection struct{}

// Project derives the customer-facing expected-artifact read model from live
// topology, active/completed dispatch facts, and worker results. It is
// deterministic for a given runtime snapshot.
func (WorkArtifactProjection) Project(input WorkArtifactProjectionInput) []work.ExpectedArtifactReadModel {
	if input.Token == nil {
		return nil
	}
	runtimeToken := factorytoken.FromWorker(*input.Token)
	workTypeDeclarations := workTypeArtifactDeclarations(input.Topology, input.Token.Color.WorkTypeID)
	if dispatch, ok := activeArtifactDispatch(input.Token, input.Dispatches); ok {
		return projectWorkExpectedArtifacts(
			workTypeDeclarations,
			workstationArtifactDeclarations(input.Topology, dispatch.transitionID, dispatch.workstationName),
			expectedArtifactInputs(dispatch.inputs, input.Token),
			work.ExpectedArtifactObservation{},
			dispatch.templateContext,
		)
	}
	if dispatch, ok := completedArtifactDispatch(input.Token, input.DispatchHistory, input.Results); ok {
		return projectWorkExpectedArtifacts(
			workTypeDeclarations,
			workstationArtifactDeclarations(input.Topology, dispatch.transitionID, dispatch.workstationName),
			expectedArtifactInputs(dispatch.inputs, input.Token),
			dispatch.observation,
			dispatch.templateContext,
		)
	}
	return work.ExpectedArtifactReadModelProjector{}.Project(
		workTypeDeclarations,
		workstationArtifactDeclarationsForPlace(input.Topology, runtimeToken.PlaceID),
		[]work.ExpectedArtifactInput{expectedArtifactInput(*input.Token)},
		work.ExpectedArtifactObservation{},
	)
}

func projectWorkExpectedArtifacts(
	workTypeDeclarations, workstationDeclarations []work.ExpectedArtifactDeclaration,
	inputs []work.ExpectedArtifactInput,
	observation work.ExpectedArtifactObservation,
	templateContext *work.ExpectedArtifactTemplateContext,
) []work.ExpectedArtifactReadModel {
	if templateContext == nil {
		return work.ExpectedArtifactReadModelProjector{}.Project(workTypeDeclarations, workstationDeclarations, inputs, observation)
	}
	return work.ExpectedArtifactReadModelProjector{}.Project(
		workTypeDeclarations, workstationDeclarations, inputs, observation, *templateContext,
	)
}

type artifactDispatchFacts struct {
	transitionID    string
	workstationName string
	inputs          []workers.Token
	observation     work.ExpectedArtifactObservation
	templateContext *work.ExpectedArtifactTemplateContext
}

func activeArtifactDispatch(token *workers.Token, dispatches map[string]*DispatchEntry) (artifactDispatchFacts, bool) {
	ids := make([]string, 0, len(dispatches))
	for id := range dispatches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		dispatch := dispatches[id]
		if dispatch == nil || !dispatchContainsWork(dispatch.ConsumedTokens, token.Color.WorkID) {
			continue
		}
		return artifactDispatchFacts{
			transitionID:    dispatch.TransitionID,
			workstationName: dispatch.WorkstationName,
			inputs:          append([]workers.Token(nil), dispatch.ConsumedTokens...),
			templateContext: cloneExpectedArtifactTemplateContext(dispatch.ExpectedArtifactContext),
		}, true
	}
	return artifactDispatchFacts{}, false
}

func completedArtifactDispatch(token *workers.Token, dispatches []CompletedDispatch, results []workers.WorkResult) (artifactDispatchFacts, bool) {
	for index := len(dispatches) - 1; index >= 0; index-- {
		dispatch := dispatches[index]
		if !dispatchContainsWork(dispatch.ConsumedTokens, token.Color.WorkID) {
			continue
		}
		return artifactDispatchFacts{
			transitionID:    dispatch.TransitionID,
			workstationName: dispatch.WorkstationName,
			inputs:          append([]workers.Token(nil), dispatch.ConsumedTokens...),
			observation:     artifactObservation(dispatch, results),
			templateContext: cloneExpectedArtifactTemplateContext(dispatch.ExpectedArtifactContext),
		}, true
	}
	return artifactDispatchFacts{}, false
}

func dispatchContainsWork(tokens []workers.Token, workID string) bool {
	for _, token := range tokens {
		if token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func artifactObservation(dispatch CompletedDispatch, results []workers.WorkResult) work.ExpectedArtifactObservation {
	if dispatch.ArtifactVerification != nil {
		return expectedArtifactObservation(dispatch.ArtifactVerification)
	}
	for _, result := range results {
		if result.DispatchID != dispatch.DispatchID {
			continue
		}
		if result.ArtifactVerification == nil {
			if result.Outcome == workers.OutcomeAccepted {
				return work.ExpectedArtifactObservation{Verified: true}
			}
			return work.ExpectedArtifactObservation{}
		}
		return expectedArtifactObservation(result.ArtifactVerification)
	}
	if dispatch.Outcome == workers.OutcomeAccepted {
		return work.ExpectedArtifactObservation{Verified: true}
	}
	return work.ExpectedArtifactObservation{}
}

func expectedArtifactObservation(verification *workers.ExpectedArtifactVerification) work.ExpectedArtifactObservation {
	if verification == nil {
		return work.ExpectedArtifactObservation{}
	}
	observation := work.ExpectedArtifactObservation{Verified: true}
	for _, entry := range verification.Entries {
		observation.Entries = append(observation.Entries, work.ExpectedArtifactVerificationEntry{
			DeclarationIndex: entry.DeclarationIndex,
			Name:             entry.Name,
			Pattern:          entry.Pattern,
			Reason:           work.ExpectedArtifactVerificationReason(entry.Reason),
		})
	}
	return observation
}

func workTypeArtifactDeclarations(topology *RuntimeNet, workTypeID string) []work.ExpectedArtifactDeclaration {
	if topology == nil || topology.WorkTypes[workTypeID] == nil {
		return nil
	}
	return append([]work.ExpectedArtifactDeclaration(nil), topology.WorkTypes[workTypeID].ExpectedArtifacts...)
}

func workstationArtifactDeclarations(topology *RuntimeNet, transitionID, workstationName string) []work.ExpectedArtifactDeclaration {
	if topology == nil {
		return nil
	}
	for _, transition := range topology.Transitions {
		if transition == nil {
			continue
		}
		if transition.ID == transitionID || transition.Name == transitionID || transition.Name == workstationName {
			return append([]work.ExpectedArtifactDeclaration(nil), transition.ExpectedArtifacts...)
		}
	}
	return nil
}

func workstationArtifactDeclarationsForPlace(topology *RuntimeNet, placeID string) []work.ExpectedArtifactDeclaration {
	if topology == nil {
		return nil
	}
	transitionIDs := make([]string, 0, len(topology.Transitions))
	for transitionID := range topology.Transitions {
		transitionIDs = append(transitionIDs, transitionID)
	}
	sort.Strings(transitionIDs)
	var declarations []work.ExpectedArtifactDeclaration
	for _, transitionID := range transitionIDs {
		transition := topology.Transitions[transitionID]
		if transition == nil || !transitionConsumesPlace(transition.InputArcs, placeID) {
			continue
		}
		declarations = append(declarations, transition.ExpectedArtifacts...)
	}
	return declarations
}

func transitionConsumesPlace(arcs []PetriArc, placeID string) bool {
	for _, arc := range arcs {
		if arc.PlaceID == placeID {
			return true
		}
	}
	return false
}

func expectedArtifactInputs(tokens []workers.Token, fallback *workers.Token) []work.ExpectedArtifactInput {
	if len(tokens) == 0 && fallback != nil {
		tokens = []workers.Token{*fallback}
	}
	inputs := make([]work.ExpectedArtifactInput, 0, len(tokens))
	for _, token := range tokens {
		inputs = append(inputs, expectedArtifactInput(token))
	}
	return inputs
}

func expectedArtifactInput(token workers.Token) work.ExpectedArtifactInput {
	return work.ExpectedArtifactInput{
		Name:       firstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID),
		WorkID:     token.Color.WorkID,
		WorkTypeID: token.Color.WorkTypeID,
		DataType:   string(token.Color.DataType),
		TraceID:    token.Color.TraceID,
		ParentID:   token.Color.ParentID,
		Project:    token.Color.Tags[workers.ProjectTagKey],
		Tags:       work.CloneTags(token.Color.Tags),
		Payload:    string(token.Color.Payload),
	}
}

func cloneExpectedArtifactTemplateContext(context *work.ExpectedArtifactTemplateContext) *work.ExpectedArtifactTemplateContext {
	return context.Clone()
}
