package projections

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	sessionprojectionfacts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/sessionprojectionfacts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *factoryWorldReducer) applySessionLifecycleEvent(event interfaces.FactoryEvent) (bool, error) {
	switch event.Type {
	case interfaces.FactoryEventTypeSessionStarted:
		return true, r.applySessionStartedEvent(event)
	case interfaces.FactoryEventTypeSessionPaused:
		return true, r.applySessionPausedEvent(event)
	case interfaces.FactoryEventTypeSessionResumed:
		return true, r.applySessionResumedEvent(event)
	case interfaces.FactoryEventTypeSessionResultUpdated:
		return true, r.applySessionResultUpdatedEvent(event)
	case interfaces.FactoryEventTypeSessionCompleted:
		return true, r.applySessionCompletedEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applySessionStartedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.FactorySessionStartedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	if sessionID := stringValue(event.Context.SessionID); sessionID != "" {
		bracket.SessionID = sessionID
	}
	if kind := event.Context.OrchestratorKind; kind != nil {
		bracket.OrchestratorKind = string(*kind)
	}
	bracket.OrchestratorDialect = stringValue(event.Context.OrchestratorDialect)
	bracket.FactoryID = stringValue(payload.FactoryID)
	bracket.SourceRef = stringValue(payload.SourceRef)
	bracket.SourceHash = stringValue(payload.SourceHash)
	bracket.PolicyHash = stringValue(payload.PolicyHash)
	bracket.ArgsDigest = stringValue(payload.ArgsDigest)
	bracket.StartedAt = payload.StartedAt.UTC()
	return nil
}

func (r *factoryWorldReducer) applySessionPausedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.FactorySessionPausedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	mergeSessionBracketIdentity(bracket, event.Context)
	bracket.LifecycleControlStatus = string(payload.Status)
	bracket.PausedAt = payload.PausedAt.UTC()
	return nil
}

func (r *factoryWorldReducer) applySessionResumedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.FactorySessionResumedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	mergeSessionBracketIdentity(bracket, event.Context)
	bracket.LifecycleControlStatus = string(payload.Status)
	bracket.ResumedAt = payload.ResumedAt.UTC()
	return nil
}

func (r *factoryWorldReducer) applySessionResultUpdatedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.FactorySessionResultUpdatedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	mergeSessionBracketIdentity(bracket, event.Context)
	bracket.ResultStatus = string(payload.ResultStatus)
	bracket.ResultSummary = cloneWorkContentParts(payload.ResultSummary)
	bracket.ArtifactIDs = cloneStringSlice(payload.ArtifactIDs)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.PrimaryResult = cloneWorkContentParts(bracket.ResultSummary)
		runtime.ResultStatus = bracket.ResultStatus
		for _, artifactID := range bracket.ArtifactIDs {
			artifact, ok := findArtifactStateByID(r.stateValue.Artifacts, artifactID)
			if !ok {
				artifact = interfaces.FactorySessionArtifactState{ID: artifactID}
				r.stateValue.Artifacts = append(r.stateValue.Artifacts, artifact)
			}
			appendUniqueArtifactState(&runtime.Artifacts, artifact)
		}
	}
	return nil
}

func findArtifactStateByID(artifacts []interfaces.FactorySessionArtifactState, artifactID string) (interfaces.FactorySessionArtifactState, bool) {
	trimmed := strings.TrimSpace(artifactID)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ID) == trimmed {
			return artifact, true
		}
	}
	return interfaces.FactorySessionArtifactState{}, false
}

func appendUniqueArtifactState(artifacts *[]interfaces.FactorySessionArtifactState, artifact interfaces.FactorySessionArtifactState) {
	for _, existing := range *artifacts {
		if strings.TrimSpace(existing.ID) == strings.TrimSpace(artifact.ID) {
			return
		}
	}
	*artifacts = append(*artifacts, artifact)
}

func (r *factoryWorldReducer) applySessionCompletedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.FactorySessionCompletedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	mergeSessionBracketIdentity(bracket, event.Context)
	bracket.Terminal = true
	bracket.FinalStatus = string(payload.FinalStatus)
	bracket.CompletedAt = payload.CompletedAt.UTC()
	if payload.DurationMillis != nil {
		bracket.DurationMillis = *payload.DurationMillis
	}
	if payload.ResultStatus != nil {
		bracket.ResultStatus = string(*payload.ResultStatus)
	}
	bracket.ArtifactIDs = cloneStringSlice(payload.ArtifactIDs)
	if payload.DispatchCounts != nil {
		bracket.DispatchCounts = &interfaces.FactoryWorldJavaScriptChildDispatchCounts{
			Queued:    payload.DispatchCounts.Queued,
			Running:   payload.DispatchCounts.Running,
			Completed: payload.DispatchCounts.Completed,
		}
	}
	if payload.FailureDetail != nil {
		bracket.FailureDetail = &workerexecution.FailureDetail{
			Reason:  workerexecution.WorkFailureType(payload.FailureDetail.Reason),
			Message: payload.FailureDetail.Message,
		}
	}
	return nil
}

func (r *factoryWorldReducer) ensureSessionBracket() *interfaces.FactoryWorldSessionBracketState {
	if r.stateValue.SessionBracket == nil {
		r.stateValue.SessionBracket = &interfaces.FactoryWorldSessionBracketState{}
	}
	return r.stateValue.SessionBracket
}

func mergeSessionBracketIdentity(bracket *interfaces.FactoryWorldSessionBracketState, context interfaces.FactoryEventContext) {
	if bracket == nil {
		return
	}
	if sessionID := stringValue(context.SessionID); sessionID != "" {
		bracket.SessionID = sessionID
	}
	if kind := context.OrchestratorKind; kind != nil && bracket.OrchestratorKind == "" {
		bracket.OrchestratorKind = string(*kind)
	}
	if dialect := stringValue(context.OrchestratorDialect); dialect != "" && bracket.OrchestratorDialect == "" {
		bracket.OrchestratorDialect = dialect
	}
}

func buildFactoryWorldSessionBracketProjection(
	state interfaces.FactoryWorldState,
) *interfaces.FactoryWorldSessionBracketProjection {
	if state.SessionBracket == nil {
		return nil
	}
	bracket := state.SessionBracket
	if bracket.SessionID == "" && !bracket.Terminal && bracket.StartedAt.IsZero() {
		return nil
	}
	return &interfaces.FactoryWorldSessionBracketProjection{
		SessionID:              bracket.SessionID,
		OrchestratorKind:       bracket.OrchestratorKind,
		OrchestratorDialect:    bracket.OrchestratorDialect,
		FactoryID:              bracket.FactoryID,
		SourceRef:              bracket.SourceRef,
		StartedAt:              bracket.StartedAt,
		LifecycleControlStatus: bracket.LifecycleControlStatus,
		PausedAt:               bracket.PausedAt,
		ResumedAt:              bracket.ResumedAt,
		ResultStatus:           bracket.ResultStatus,
		ResultSummary:          cloneWorkContentParts(bracket.ResultSummary),
		ArtifactIDs:            cloneStringSlice(bracket.ArtifactIDs),
		Terminal:               bracket.Terminal,
		FinalStatus:            bracket.FinalStatus,
		CompletedAt:            bracket.CompletedAt,
		DurationMillis:         bracket.DurationMillis,
		FailureDetail:          workerexecution.CloneFailureDetail(bracket.FailureDetail),
	}
}

func cloneWorkContentParts(parts []work.WorkContentPart) []work.WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]work.WorkContentPart, len(parts))
	copy(cloned, parts)
	return cloned
}

func (r *factoryWorldReducer) applyOrchestratorProgressEvent(event interfaces.FactoryEvent) (bool, error) {
	switch event.Type {
	case interfaces.FactoryEventTypeOrchestratorPhaseChanged:
		return true, r.applyOrchestratorPhaseChangedEvent(event)
	case interfaces.FactoryEventTypeOrchestratorCheckpointWritten:
		return true, r.applyOrchestratorCheckpointWrittenEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applyOrchestratorPhaseChangedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.OrchestratorPhaseChangedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	runtime := r.ensureJavaScriptRuntime()
	currentPhase := stringValue(event.Context.PhaseName)
	if currentPhase == "" {
		currentPhase = stringValue(event.Context.PhaseID)
	}
	runtime.Phase = currentPhase
	runtime.Phases = appendOrchestratorPhaseHistory(
		runtime.Phases,
		stringValue(payload.PreviousPhaseName),
		stringValue(payload.PreviousPhaseID),
		currentPhase,
	)
	runtime.ScriptStatus = orchestratorPhaseStatusToScriptStatus(payload.PhaseStatus)
	return nil
}

func (r *factoryWorldReducer) applyOrchestratorCheckpointWrittenEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.OrchestratorCheckpointWrittenEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	checkpointID := stringValue(event.Context.CheckpointID)
	if checkpointID == "" && payload.ArtifactRef != nil {
		checkpointID = payload.ArtifactRef.ID
	}
	checkpoint := interfaces.FactorySessionJavaScriptCheckpointRef{
		ID:                 checkpointID,
		Label:              payload.Label,
		ResumabilityStatus: string(payload.ResumabilityStatus),
		Warnings:           projectOrchestratorCheckpointWarnings(payload.Warnings),
	}
	if payload.Timestamp != nil {
		checkpoint.Timestamp = payload.Timestamp.UTC()
	}
	if payload.ArtifactRef != nil {
		checkpoint.ArtifactRef = &interfaces.JavaScriptCheckpointArtifactRef{
			ID:         payload.ArtifactRef.ID,
			Kind:       payload.ArtifactRef.Kind,
			Visibility: payload.ArtifactRef.Visibility,
		}
		if payload.ArtifactRef.ContentHash != nil {
			checkpoint.ArtifactRef.ContentHash = *payload.ArtifactRef.ContentHash
		}
		if payload.ArtifactRef.SizeBytes != nil {
			checkpoint.ArtifactRef.SizeBytes = *payload.ArtifactRef.SizeBytes
		}
	}
	r.stateValue.JavaScriptCheckpoints = append(r.stateValue.JavaScriptCheckpoints, checkpoint)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.Checkpoints = append(runtime.Checkpoints, checkpoint)
	}
	return nil
}

func appendOrchestratorPhaseHistory(phases []string, previousName, previousID, currentPhase string) []string {
	if previous := orchestratorPhaseHistoryName(previousName, previousID); previous != "" {
		phases = appendPhaseHistoryEntry(phases, previous)
	}
	if current := strings.TrimSpace(currentPhase); current != "" {
		phases = appendPhaseHistoryEntry(phases, current)
	}
	return phases
}

func orchestratorPhaseHistoryName(name, id string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(id)
}

func appendPhaseHistoryEntry(phases []string, phase string) []string {
	if phase == "" {
		return phases
	}
	if len(phases) > 0 && phases[len(phases)-1] == phase {
		return phases
	}
	return append(phases, phase)
}

func orchestratorPhaseStatusToScriptStatus(status interfaces.OrchestratorPhaseStatus) string {
	switch status {
	case interfaces.OrchestratorPhaseStatusActive:
		return "RUNNING"
	case interfaces.OrchestratorPhaseStatusCompleted:
		return "FINISHED"
	case interfaces.OrchestratorPhaseStatusSkipped:
		return "SKIPPED"
	default:
		return string(status)
	}
}

func projectOrchestratorCheckpointWarnings(
	warnings []interfaces.FactoryDispatchWarning,
) []interfaces.FactorySessionDispatchWarning {
	if len(warnings) == 0 {
		return nil
	}
	projected := make([]interfaces.FactorySessionDispatchWarning, 0, len(warnings))
	for _, warning := range warnings {
		projected = append(projected, interfaces.FactorySessionDispatchWarning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return projected
}

// IncrementalSessionProjection applies canonical events in append order and
// retains only the event-derived facts needed by live Factory Session reads.
// The owning event ledger serializes Apply with canonical appends; callers of
// SnapshotSessionProjectionFacts receive detached values.
type IncrementalSessionProjection struct {
	reducer *factoryWorldReducer
}

// NewIncrementalSessionProjection creates an empty append-order projection.
func NewIncrementalSessionProjection() *IncrementalSessionProjection {
	return &IncrementalSessionProjection{reducer: newFactoryWorldReducer(0)}
}

// Apply incorporates one canonical event into the live session projection.
func (projection *IncrementalSessionProjection) Apply(event interfaces.FactoryEvent) error {
	if projection == nil {
		return nil
	}
	if projection.reducer == nil {
		projection.reducer = newFactoryWorldReducer(0)
	}
	return projection.reducer.apply(event)
}

// SnapshotSessionProjectionFacts returns detached event-derived session facts.
func (projection *IncrementalSessionProjection) SnapshotSessionProjectionFacts() sessionprojectionfacts.SessionProjectionFacts {
	if projection == nil || projection.reducer == nil {
		return sessionprojectionfacts.SessionProjectionFacts{}
	}
	state := projection.reducer.stateValue
	return sessionprojectionfacts.SessionProjectionFacts{
		PendingHumanApprovals: clonePendingHumanApprovals(state.PendingHumanApprovalsByID),
		JavaScriptRuntime:     cloneJavaScriptRuntimeState(state.JavaScriptRuntime),
		SessionBracket:        cloneSessionBracketState(state.SessionBracket),
	}
}

func clonePendingHumanApprovals(
	values map[string]interfaces.FactoryWorldHumanApproval,
) map[string]interfaces.FactoryWorldHumanApproval {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]interfaces.FactoryWorldHumanApproval, len(values))
	for id, value := range values {
		cloned[id] = cloneHumanApproval(value)
	}
	return cloned
}

func cloneHumanApproval(value interfaces.FactoryWorldHumanApproval) interfaces.FactoryWorldHumanApproval {
	cloned := value
	cloned.Decisions = append([]interfaces.HumanApprovalDecision(nil), value.Decisions...)
	cloned.WorkItemIDs = append([]string(nil), value.WorkItemIDs...)
	cloned.TraceIDs = append([]string(nil), value.TraceIDs...)
	if value.WorkstationDescription != nil {
		description := *value.WorkstationDescription
		description.Locales = append([]string(nil), value.WorkstationDescription.Locales...)
		if value.WorkstationDescription.Values != nil {
			description.Values = make(map[string]string, len(value.WorkstationDescription.Values))
			for locale, text := range value.WorkstationDescription.Values {
				description.Values[locale] = text
			}
		}
		cloned.WorkstationDescription = &description
	}
	return cloned
}

func cloneJavaScriptRuntimeState(
	state *interfaces.FactorySessionJavaScriptRuntimeState,
) *interfaces.FactorySessionJavaScriptRuntimeState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Phases = append([]string(nil), state.Phases...)
	cloned.Checkpoints = cloneJavaScriptCheckpoints(state.Checkpoints)
	cloned.Dispatches = cloneSessionDispatches(state.Dispatches)
	cloned.Artifacts = cloneSessionArtifacts(state.Artifacts)
	cloned.PrimaryResult = work.CloneWorkContentParts(state.PrimaryResult)
	return &cloned
}

func cloneJavaScriptCheckpoints(
	checkpoints []interfaces.FactorySessionJavaScriptCheckpointRef,
) []interfaces.FactorySessionJavaScriptCheckpointRef {
	if len(checkpoints) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactorySessionJavaScriptCheckpointRef, len(checkpoints))
	for index, checkpoint := range checkpoints {
		cloned[index] = checkpoint
		cloned[index].Warnings = append([]interfaces.FactorySessionDispatchWarning(nil), checkpoint.Warnings...)
		if checkpoint.ArtifactRef != nil {
			artifactRef := *checkpoint.ArtifactRef
			cloned[index].ArtifactRef = &artifactRef
		}
	}
	return cloned
}

func cloneSessionDispatches(
	dispatches []interfaces.FactorySessionDispatchState,
) []interfaces.FactorySessionDispatchState {
	if len(dispatches) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactorySessionDispatchState, len(dispatches))
	for index, dispatch := range dispatches {
		cloned[index] = dispatch
		cloned[index].RelatedWorkIDs = append([]string(nil), dispatch.RelatedWorkIDs...)
		cloned[index].ArtifactIDs = append([]string(nil), dispatch.ArtifactIDs...)
		cloned[index].Warnings = append([]interfaces.FactorySessionDispatchWarning(nil), dispatch.Warnings...)
		if dispatch.Usage != nil {
			usage := *dispatch.Usage
			cloned[index].Usage = &usage
		}
		if dispatch.FailureDetail != nil {
			failure := *dispatch.FailureDetail
			cloned[index].FailureDetail = &failure
		}
		if dispatch.Petri != nil {
			petri := *dispatch.Petri
			cloned[index].Petri = &petri
		}
		if dispatch.JavaScript != nil {
			javascript := *dispatch.JavaScript
			cloned[index].JavaScript = &javascript
		}
	}
	return cloned
}

func cloneSessionArtifacts(
	artifacts []interfaces.FactorySessionArtifactState,
) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactorySessionArtifactState, len(artifacts))
	for index, artifact := range artifacts {
		cloned[index] = artifact
		if artifact.RedactionCounts != nil {
			cloned[index].RedactionCounts = make(map[string]int, len(artifact.RedactionCounts))
			for key, count := range artifact.RedactionCounts {
				cloned[index].RedactionCounts[key] = count
			}
		}
		if artifact.CaptureMetadata != nil {
			cloned[index].CaptureMetadata = make(map[string]string, len(artifact.CaptureMetadata))
			for key, value := range artifact.CaptureMetadata {
				cloned[index].CaptureMetadata[key] = value
			}
		}
	}
	return cloned
}

func cloneSessionBracketState(
	bracket *interfaces.FactoryWorldSessionBracketState,
) *interfaces.FactoryWorldSessionBracketState {
	if bracket == nil {
		return nil
	}
	cloned := *bracket
	cloned.ResultSummary = work.CloneWorkContentParts(bracket.ResultSummary)
	cloned.ArtifactIDs = append([]string(nil), bracket.ArtifactIDs...)
	if bracket.DispatchCounts != nil {
		dispatchCounts := *bracket.DispatchCounts
		cloned.DispatchCounts = &dispatchCounts
	}
	cloned.FailureDetail = workerexecution.CloneFailureDetail(bracket.FailureDetail)
	return &cloned
}
