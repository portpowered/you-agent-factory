package factorysessions

import (
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// PetriDispatchStatesFromSnapshot maps active and completed Petri dispatches from
// one engine snapshot into session projection inputs.
func PetriDispatchStatesFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) []interfaces.FactorySessionDispatchState {
	if snapshot == nil {
		return nil
	}
	dispatches := make([]interfaces.FactorySessionDispatchState, 0, len(snapshot.Dispatches)+len(snapshot.DispatchHistory))
	for dispatchID, entry := range snapshot.Dispatches {
		if entry == nil {
			continue
		}
		dispatches = append(dispatches, petriActiveDispatchState(dispatchID, *entry, snapshot.Topology))
	}
	for _, completed := range snapshot.DispatchHistory {
		dispatches = append(dispatches, petriCompletedDispatchState(completed, snapshot.Topology))
	}
	sort.Slice(dispatches, func(i, j int) bool {
		return dispatches[i].ID < dispatches[j].ID
	})
	return dispatches
}

func petriActiveDispatchState(
	dispatchID string,
	entry interfaces.DispatchEntry,
	topology *state.Net,
) interfaces.FactorySessionDispatchState {
	workerType := workerTypeForTransition(topology, entry.TransitionID)
	label := firstNonEmptyString(entry.WorkstationName, entry.TransitionID)
	return interfaces.FactorySessionDispatchState{
		ID:             dispatchID,
		DispatchKind:   string(factoryapi.FactoryDispatchKindPETRITRANSITION),
		Status:         string(factoryapi.FactoryDispatchStatusRUNNING),
		Label:          label,
		RelatedWorkIDs: workIDsFromTokens(entry.ConsumedTokens),
		Petri: &interfaces.FactorySessionDispatchPetriState{
			TransitionID:    entry.TransitionID,
			WorkstationName: entry.WorkstationName,
			WorkerType:      workerType,
		},
	}
}

func petriCompletedDispatchState(
	completed interfaces.CompletedDispatch,
	topology *state.Net,
) interfaces.FactorySessionDispatchState {
	status := factoryapi.FactoryDispatchStatusCOMPLETED
	if completed.Outcome == interfaces.OutcomeFailed || completed.Outcome == interfaces.OutcomeRejected {
		status = factoryapi.FactoryDispatchStatusFAILED
	}
	workerType := workerTypeForTransition(topology, completed.TransitionID)
	label := firstNonEmptyString(completed.WorkstationName, completed.TransitionID, completed.SelectedClassificationLabel)
	state := interfaces.FactorySessionDispatchState{
		ID:             completed.DispatchID,
		DispatchKind:   string(factoryapi.FactoryDispatchKindPETRITRANSITION),
		Status:         string(status),
		Label:          label,
		RelatedWorkIDs: workIDsFromTokens(completed.ConsumedTokens),
		Usage:          dispatchUsageFromCompleted(completed),
		Petri: &interfaces.FactorySessionDispatchPetriState{
			TransitionID:    completed.TransitionID,
			WorkstationName: completed.WorkstationName,
			WorkerType:      workerType,
		},
	}
	if completed.ProviderSession != nil {
		state.Provider = strings.TrimSpace(completed.ProviderSession.Provider)
	}
	if status == factoryapi.FactoryDispatchStatusFAILED {
		state.FailureDetail = failureDetailFromCompleted(completed)
	}
	return state
}

func dispatchUsageFromCompleted(completed interfaces.CompletedDispatch) *interfaces.FactorySessionDispatchUsage {
	if completed.Duration == 0 {
		return nil
	}
	return &interfaces.FactorySessionDispatchUsage{
		DurationMillis: completed.Duration.Milliseconds(),
	}
}

func failureDetailFromCompleted(completed interfaces.CompletedDispatch) *interfaces.FactorySessionDispatchFailureDetail {
	detail := &interfaces.FactorySessionDispatchFailureDetail{
		Message: firstNonEmptyString(completed.Reason),
	}
	if completed.FailureMetadata != nil {
		detail.Reason = string(completed.FailureMetadata.Type)
	}
	return detail
}

func workerTypeForTransition(topology *state.Net, transitionID string) string {
	if topology == nil || strings.TrimSpace(transitionID) == "" {
		return ""
	}
	transition, ok := topology.Transitions[transitionID]
	if !ok || transition == nil {
		return ""
	}
	return strings.TrimSpace(transition.WorkerType)
}

func workIDsFromTokens(tokens []interfaces.Token) []string {
	if len(tokens) == 0 {
		return nil
	}
	workIDs := make([]string, 0, len(tokens))
	for _, token := range tokens {
		workID := strings.TrimSpace(token.Color.WorkID)
		if workID == "" {
			continue
		}
		workIDs = appendUniqueString(workIDs, workID)
	}
	sort.Strings(workIDs)
	return workIDs
}

func projectedDispatches(
	sessionID string,
	orchestratorKind factoryapi.FactoryOrchestratorKind,
	states []interfaces.FactorySessionDispatchState,
) *[]factoryapi.FactoryDispatch {
	if len(states) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactoryDispatch, 0, len(states))
	for _, state := range states {
		projected = append(projected, projectedDispatch(sessionID, orchestratorKind, state))
	}
	return &projected
}

func projectedDispatch(
	sessionID string,
	orchestratorKind factoryapi.FactoryOrchestratorKind,
	state interfaces.FactorySessionDispatchState,
) factoryapi.FactoryDispatch {
	dispatch := factoryapi.FactoryDispatch{
		Id:               strings.TrimSpace(state.ID),
		SessionId:        sessionID,
		OrchestratorKind: orchestratorKind,
		DispatchKind:     factoryapi.FactoryDispatchKind(strings.TrimSpace(state.DispatchKind)),
		Status:           factoryapi.FactoryDispatchStatus(strings.TrimSpace(state.Status)),
	}
	if phase := strings.TrimSpace(state.Phase); phase != "" {
		dispatch.Phase = &phase
	}
	if label := strings.TrimSpace(state.Label); label != "" {
		dispatch.Label = &label
	}
	if runnerID := strings.TrimSpace(state.RunnerID); runnerID != "" {
		dispatch.RunnerId = &runnerID
	}
	if model := strings.TrimSpace(state.Model); model != "" {
		dispatch.Model = &model
	}
	if provider := strings.TrimSpace(state.Provider); provider != "" {
		dispatch.Provider = &provider
	}
	if promptDigest := strings.TrimSpace(state.PromptDigest); promptDigest != "" {
		dispatch.PromptDigest = &promptDigest
	}
	if schemaDigest := strings.TrimSpace(state.SchemaDigest); schemaDigest != "" {
		dispatch.SchemaDigest = &schemaDigest
	}
	if len(state.RelatedWorkIDs) > 0 {
		relatedWorkIDs := append([]string(nil), state.RelatedWorkIDs...)
		dispatch.RelatedWorkIds = &relatedWorkIDs
	}
	if len(state.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), state.ArtifactIDs...)
		dispatch.ArtifactIds = &artifactIDs
	}
	if state.Usage != nil {
		dispatch.Usage = projectedDispatchUsage(*state.Usage)
	}
	if warnings := projectedDispatchWarnings(state.Warnings); len(warnings) > 0 {
		dispatch.Warnings = &warnings
	}
	if state.FailureDetail != nil {
		dispatch.FailureDetail = projectedDispatchFailureDetail(*state.FailureDetail)
	}
	if state.Petri != nil {
		dispatch.Petri = projectedDispatchPetri(*state.Petri)
	}
	if state.JavaScript != nil {
		dispatch.Javascript = projectedDispatchJavaScript(*state.JavaScript)
	}
	return dispatch
}

func projectedDispatchUsage(usage interfaces.FactorySessionDispatchUsage) *factoryapi.FactoryDispatchUsage {
	projected := &factoryapi.FactoryDispatchUsage{}
	hasValue := false
	if usage.InputTokens > 0 {
		value := usage.InputTokens
		projected.InputTokens = &value
		hasValue = true
	}
	if usage.OutputTokens > 0 {
		value := usage.OutputTokens
		projected.OutputTokens = &value
		hasValue = true
	}
	if usage.TotalTokens > 0 {
		value := usage.TotalTokens
		projected.TotalTokens = &value
		hasValue = true
	}
	if usage.CostUSD > 0 {
		value := usage.CostUSD
		projected.CostUsd = &value
		hasValue = true
	}
	if usage.DurationMillis > 0 {
		value := usage.DurationMillis
		projected.DurationMillis = &value
		hasValue = true
	}
	if usage.RetryCount > 0 {
		value := int32(usage.RetryCount)
		projected.RetryCount = &value
		hasValue = true
	}
	if !hasValue {
		return nil
	}
	return projected
}

func projectedDispatchWarnings(warnings []interfaces.FactorySessionDispatchWarning) []factoryapi.FactoryDispatchWarning {
	if len(warnings) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactoryDispatchWarning, 0, len(warnings))
	for _, warning := range warnings {
		code := strings.TrimSpace(warning.Code)
		message := strings.TrimSpace(warning.Message)
		if code == "" || message == "" {
			continue
		}
		projected = append(projected, factoryapi.FactoryDispatchWarning{
			Code:    code,
			Message: message,
		})
	}
	return projected
}

func projectedDispatchFailureDetail(
	detail interfaces.FactorySessionDispatchFailureDetail,
) *factoryapi.FailureDetail {
	reason := strings.TrimSpace(detail.Reason)
	message := strings.TrimSpace(detail.Message)
	if reason == "" || message == "" {
		return nil
	}
	return &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureType(reason), Message: message}
}

func projectedDispatchPetri(
	petriState interfaces.FactorySessionDispatchPetriState,
) *factoryapi.FactoryDispatchPetriProjection {
	projected := &factoryapi.FactoryDispatchPetriProjection{
		TransitionId: strings.TrimSpace(petriState.TransitionID),
	}
	if workstation := strings.TrimSpace(petriState.WorkstationName); workstation != "" {
		projected.WorkstationName = &workstation
	}
	if workerType := strings.TrimSpace(petriState.WorkerType); workerType != "" {
		projected.WorkerType = &workerType
	}
	return projected
}

func projectedDispatchJavaScript(
	jsState interfaces.FactorySessionDispatchJavaScriptState,
) *factoryapi.FactoryDispatchJavaScriptProjection {
	projected := &factoryapi.FactoryDispatchJavaScriptProjection{
		TaskKind: factoryapi.FactoryDispatchJavaScriptTaskKind(strings.TrimSpace(jsState.TaskKind)),
	}
	if label := strings.TrimSpace(jsState.TaskLabel); label != "" {
		projected.TaskLabel = &label
	}
	return projected
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func javaScriptDispatchKind(taskKind string) factoryapi.FactoryDispatchKind {
	switch strings.ToUpper(strings.TrimSpace(taskKind)) {
	case "VERIFY":
		return factoryapi.FactoryDispatchKindJAVASCRIPTVERIFY
	case "SYNTHESIZE":
		return factoryapi.FactoryDispatchKindJAVASCRIPTSYNTHESIZE
	case "TOOL":
		return factoryapi.FactoryDispatchKindJAVASCRIPTTOOL
	case "SCRIPT":
		return factoryapi.FactoryDispatchKindJAVASCRIPTSCRIPT
	case "SYSTEM":
		return factoryapi.FactoryDispatchKindJAVASCRIPTSYSTEM
	default:
		return factoryapi.FactoryDispatchKindJAVASCRIPTAGENT
	}
}

func projectedJavaScriptDispatchStates(
	states []interfaces.FactorySessionDispatchState,
) []interfaces.FactorySessionDispatchState {
	if len(states) == 0 {
		return nil
	}
	projected := make([]interfaces.FactorySessionDispatchState, 0, len(states))
	for _, state := range states {
		item := state
		if item.DispatchKind == "" && item.JavaScript != nil {
			item.DispatchKind = string(javaScriptDispatchKind(item.JavaScript.TaskKind))
		}
		if item.DispatchKind == "" {
			item.DispatchKind = string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT)
		}
		projected = append(projected, item)
	}
	return projected
}

func projectedCheckpointArtifactStates(
	checkpoints []interfaces.JavaScriptCheckpointRecord,
) []interfaces.FactorySessionArtifactState {
	if len(checkpoints) == 0 {
		return nil
	}
	artifacts := make([]interfaces.FactorySessionArtifactState, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		artifactID := strings.TrimSpace(checkpoint.ArtifactID)
		if artifactID == "" {
			artifactID = checkpoint.ID
		}
		artifact := interfaces.FactorySessionArtifactState{
			ID:          artifactID,
			Kind:        interfaces.JavaScriptCheckpointArtifactKind,
			Visibility:  interfaces.JavaScriptCheckpointArtifactVisibility,
			Label:       checkpoint.Label,
			Summary:     checkpoint.Summary,
			AuditMode:   string(factoryapi.FactoryArtifactAuditModeFULL),
			ContentHash: checkpoint.ContentHash,
			SizeBytes:   checkpoint.SizeBytes,
			CapturedAt:  checkpoint.Timestamp,
		}
		if !checkpoint.Timestamp.IsZero() {
			artifact.CaptureMetadata = map[string]string{
				"capturedAt": checkpoint.Timestamp.UTC().Format(time.RFC3339),
			}
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

// ArtifactStatesFromJavaScriptRuntime merges explicit artifact states with
// checkpoint-derived internal artifacts for one JavaScript session.
func ArtifactStatesFromJavaScriptRuntime(
	checkpoints []interfaces.JavaScriptCheckpointRecord,
	states []interfaces.FactorySessionArtifactState,
) []interfaces.FactorySessionArtifactState {
	artifacts := append([]interfaces.FactorySessionArtifactState(nil), states...)
	artifacts = append(artifacts, projectedCheckpointArtifactStates(checkpoints)...)
	if len(artifacts) == 0 {
		return nil
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].ID < artifacts[j].ID
	})
	return artifacts
}

func projectedArtifacts(states []interfaces.FactorySessionArtifactState) *[]factoryapi.FactoryArtifact {
	if len(states) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactoryArtifact, 0, len(states))
	for _, state := range states {
		projected = append(projected, projectedArtifact(state))
	}
	return &projected
}

func projectedArtifact(state interfaces.FactorySessionArtifactState) factoryapi.FactoryArtifact {
	kind := factoryapi.FactoryArtifactKind(strings.TrimSpace(state.Kind))
	if kind == "" {
		kind = factoryapi.FactoryArtifactKindCHILDRESULT
	}
	visibility := factoryapi.FactoryArtifactVisibility(strings.TrimSpace(state.Visibility))
	if visibility == "" {
		visibility = factoryapi.FactoryArtifactVisibilityPUBLIC
	}
	artifact := factoryapi.FactoryArtifact{
		Id:         strings.TrimSpace(state.ID),
		Kind:       kind,
		Visibility: visibility,
	}
	if label := strings.TrimSpace(state.Label); label != "" {
		artifact.Label = &label
	}
	if summary := strings.TrimSpace(state.Summary); summary != "" {
		artifact.Summary = &summary
	}
	if auditMode := strings.TrimSpace(state.AuditMode); auditMode != "" {
		mode := factoryapi.FactoryArtifactAuditMode(auditMode)
		artifact.AuditMode = &mode
	}
	if hash := strings.TrimSpace(state.ContentHash); hash != "" {
		artifact.ContentHash = &hash
	}
	if state.SizeBytes > 0 {
		size := state.SizeBytes
		artifact.SizeBytes = &size
	}
	if redactions := projectedArtifactRedactionCounts(state.RedactionCounts); redactions != nil {
		artifact.RedactionCounts = redactions
	}
	if metadata := projectedArtifactCaptureMetadata(state); metadata != nil {
		artifact.CaptureMetadata = metadata
	}
	return artifact
}

func projectedArtifactRedactionCounts(
	counts map[string]int,
) *factoryapi.FactoryArtifactRedactionCounts {
	if len(counts) == 0 {
		return nil
	}
	projected := &factoryapi.FactoryArtifactRedactionCounts{}
	hasValue := false
	if value, ok := counts["secrets"]; ok && value > 0 {
		secrets := int32(value)
		projected.Secrets = &secrets
		hasValue = true
	}
	if value, ok := counts["paths"]; ok && value > 0 {
		paths := int32(value)
		projected.Paths = &paths
		hasValue = true
	}
	if value, ok := counts["tokens"]; ok && value > 0 {
		tokens := int32(value)
		projected.Tokens = &tokens
		hasValue = true
	}
	if !hasValue {
		return nil
	}
	return projected
}

func projectedArtifactCaptureMetadata(
	state interfaces.FactorySessionArtifactState,
) *factoryapi.FactoryArtifactCaptureMetadata {
	metadata := state.CaptureMetadata
	if len(metadata) == 0 && state.CapturedAt.IsZero() {
		return nil
	}
	projected := &factoryapi.FactoryArtifactCaptureMetadata{}
	hasValue := false
	if !state.CapturedAt.IsZero() {
		capturedAt := state.CapturedAt.UTC()
		projected.CapturedAt = &capturedAt
		hasValue = true
	}
	if dispatchID := strings.TrimSpace(metadata["sourceDispatchId"]); dispatchID != "" {
		projected.SourceDispatchId = &dispatchID
		hasValue = true
	}
	if mimeType := strings.TrimSpace(metadata["mimeType"]); mimeType != "" {
		projected.MimeType = &mimeType
		hasValue = true
	}
	if !hasValue {
		return nil
	}
	return projected
}

func artifactCaptureMetadata(
	capturedAt time.Time,
	sourceDispatchID string,
	mimeType string,
) map[string]string {
	metadata := make(map[string]string)
	if !capturedAt.IsZero() {
		metadata["capturedAt"] = capturedAt.UTC().Format(time.RFC3339)
	}
	if sourceDispatchID = strings.TrimSpace(sourceDispatchID); sourceDispatchID != "" {
		metadata["sourceDispatchId"] = sourceDispatchID
	}
	if mimeType = strings.TrimSpace(mimeType); mimeType != "" {
		metadata["mimeType"] = mimeType
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
