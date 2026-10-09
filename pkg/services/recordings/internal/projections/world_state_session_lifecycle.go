package projections

import (
	"strings"
	"time"

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
	reducer    *factoryWorldReducer
	workerWork *workerSessionWorkIndex
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
	if projection.workerWork == nil {
		projection.workerWork = newWorkerSessionWorkIndex()
	}
	state := &projection.reducer.stateValue
	completedBefore, providersBefore := len(state.CompletedDispatches), len(state.ProviderSessions)
	if err := projection.reducer.apply(event); err != nil {
		return err
	}
	return projection.workerWork.apply(event, *state, completedBefore, providersBefore)
}

// workerSessionWorkIndex holds keys and canonical facts, not another lifecycle
// model. Completion/provider positions are admitted at the same append barrier
// as the world reducer, so selection never walks those growing slices.
type workerSessionWorkIndex struct {
	known              map[string]struct{}
	byWork             map[string]map[string]struct{}
	workByDispatch     map[string][]string
	dispatchByWorker   map[string]string
	associations       map[string]sessionprojectionfacts.WorkerSessionAssociationFacts
	requests           map[string]interfaces.FactoryWorldDispatch
	completions        map[string]int
	providers          map[string]int
	cursors            map[string]sessionprojectionfacts.CanonicalEventCursor
	responseTimes      map[string]time.Time
	responseCursors    map[string]sessionprojectionfacts.CanonicalEventCursor
	interruptions      map[string]interfaces.DispatchInterruptedEventPayload
	interruptedWorkIDs map[string][]string
}

func newWorkerSessionWorkIndex() *workerSessionWorkIndex {
	return &workerSessionWorkIndex{
		known: make(map[string]struct{}), byWork: make(map[string]map[string]struct{}),
		workByDispatch:   make(map[string][]string),
		dispatchByWorker: make(map[string]string),
		associations:     make(map[string]sessionprojectionfacts.WorkerSessionAssociationFacts),
		requests:         make(map[string]interfaces.FactoryWorldDispatch),
		completions:      make(map[string]int), providers: make(map[string]int),
		cursors:            make(map[string]sessionprojectionfacts.CanonicalEventCursor),
		responseTimes:      make(map[string]time.Time),
		responseCursors:    make(map[string]sessionprojectionfacts.CanonicalEventCursor),
		interruptions:      make(map[string]interfaces.DispatchInterruptedEventPayload),
		interruptedWorkIDs: make(map[string][]string),
	}
}

func (index *workerSessionWorkIndex) apply(event interfaces.FactoryEvent, state interfaces.FactoryWorldState, completedBefore, providersBefore int) error {
	for _, id := range sliceValue(event.Context.WorkIDs) {
		index.known[id] = struct{}{}
	}
	for position := completedBefore; position < len(state.CompletedDispatches); position++ {
		index.completions[state.CompletedDispatches[position].DispatchID] = position
	}
	for position := providersBefore; position < len(state.ProviderSessions); position++ {
		id := state.ProviderSessions[position].DispatchID
		if _, exists := index.providers[id]; !exists {
			index.providers[id] = position
		}
	}
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return nil
	}
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
		var payload struct {
			WorkerSessionID string `json:"workerSessionId"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoningEffort"`
		}
		if err := event.DecodePayload(&payload); err != nil {
			return nil //nolint:nilerr // Malformed optional association facts are ignored, matching the canonical reducer.
		}
		if payload.WorkerSessionID == "" {
			return nil
		}
		if previous := index.associations[dispatchID].WorkerSessionID; previous != "" && index.dispatchByWorker[previous] == dispatchID {
			delete(index.dispatchByWorker, previous)
		}
		index.dispatchByWorker[payload.WorkerSessionID] = dispatchID
		index.associations[dispatchID] = sessionprojectionfacts.WorkerSessionAssociationFacts{
			WorkerSessionID: payload.WorkerSessionID, TurnID: stringValue(event.Context.RequestID),
			Model: optionalWorkerFactString(payload.Model), ReasoningEffort: optionalWorkerFactString(payload.ReasoningEffort),
			AssociatedAt: event.Context.EventTime.UTC(),
		}
	case interfaces.FactoryEventTypeDispatchRequest:
		index.requests[dispatchID] = state.ActiveDispatches[dispatchID]
	case interfaces.FactoryEventTypeDispatchResponse:
		index.responseTimes[dispatchID] = event.Context.EventTime.UTC()
		index.responseCursors[dispatchID] = sessionprojectionfacts.CanonicalEventCursor{Sequence: sessionprojectionfacts.CanonicalEventSequence(event.Context.Sequence)}
	case interfaces.FactoryEventTypeDispatchInterrupted:
		var payload interfaces.DispatchInterruptedEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		if payload.InterruptedAt.IsZero() {
			payload.InterruptedAt = event.Context.EventTime
		}
		index.interruptions[dispatchID] = payload
		index.interruptedWorkIDs[dispatchID] = append([]string(nil), sliceValue(event.Context.WorkIDs)...)
	}
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchRequest, interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeDispatchResponse, interfaces.FactoryEventTypeDispatchInterrupted,
		interfaces.FactoryEventTypeDispatchQueued, interfaces.FactoryEventTypeDispatchReconciled:
		index.cursors[dispatchID] = sessionprojectionfacts.CanonicalEventCursor{Sequence: sessionprojectionfacts.CanonicalEventSequence(event.Context.Sequence)}
	}
	index.updateMembership(dispatchID, state)
	return nil
}

// WorkerSessionFacts selects one physical association without walking history
// or requiring capture correlation or a Work-bearing request.
func (projection *IncrementalSessionProjection) WorkerSessionFacts(workerID string) (sessionprojectionfacts.WorkerSessionWorkFacts, int) {
	var selected map[string]struct{}
	if projection != nil && projection.workerWork != nil {
		index := projection.workerWork
		if dispatchID, found := index.dispatchByWorker[workerID]; found && index.associations[dispatchID].WorkerSessionID == workerID {
			selected = map[string]struct{}{dispatchID: {}}
		}
	}
	return projection.selectedWorkerSessionFacts("", selected)
}

func optionalWorkerFactString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (index *workerSessionWorkIndex) updateMembership(dispatchID string, state interfaces.FactoryWorldState) {
	workIDs := index.requests[dispatchID].WorkItemIDs
	if active, ok := state.ActiveDispatches[dispatchID]; ok && len(active.WorkItemIDs) > 0 {
		workIDs = active.WorkItemIDs
	}
	if position, ok := index.completions[dispatchID]; ok {
		if completed := state.CompletedDispatches[position]; len(completed.WorkItemIDs) > 0 {
			workIDs = completed.WorkItemIDs
		}
	} else if interrupted := index.interruptedWorkIDs[dispatchID]; len(interrupted) > 0 {
		workIDs = interrupted
	}
	for _, id := range index.workByDispatch[dispatchID] {
		delete(index.byWork[id], dispatchID)
	}
	index.workByDispatch[dispatchID] = append([]string(nil), workIDs...)
	for _, id := range workIDs {
		index.known[id] = struct{}{}
		if index.byWork[id] == nil {
			index.byWork[id] = make(map[string]struct{})
		}
		index.byWork[id][dispatchID] = struct{}{}
	}
}

// WorkerSessionWorkFacts selects from the exact Work index. The ledger holds
// its read lock across selection and supplies the generation fence.
func (projection *IncrementalSessionProjection) WorkerSessionWorkFacts(workID string) (sessionprojectionfacts.WorkerSessionWorkFacts, int) {
	var selected map[string]struct{}
	if projection != nil && projection.workerWork != nil {
		selected = projection.workerWork.byWork[workID]
	}
	return projection.selectedWorkerSessionFacts(workID, selected)
}

func (projection *IncrementalSessionProjection) selectedWorkerSessionFacts(workID string, selected map[string]struct{}) (sessionprojectionfacts.WorkerSessionWorkFacts, int) {
	facts := sessionprojectionfacts.WorkerSessionWorkFacts{
		Associations:    make(map[string]sessionprojectionfacts.WorkerSessionAssociationFacts),
		Requests:        make(map[string]interfaces.FactoryWorldDispatch),
		StateCursors:    make(map[string]sessionprojectionfacts.CanonicalEventCursor),
		ResponseTimes:   make(map[string]time.Time),
		ResponseCursors: make(map[string]sessionprojectionfacts.CanonicalEventCursor),
		Interruptions:   make(map[string]interfaces.DispatchInterruptedEventPayload),
		World:           interfaces.FactoryWorldState{ActiveDispatches: make(map[string]interfaces.FactoryWorldDispatch)},
	}
	if projection == nil || projection.reducer == nil || projection.workerWork == nil {
		return facts, 0
	}
	index, state := projection.workerWork, projection.reducer.stateValue
	_, facts.KnownWork = index.known[workID]
	if item, ok := state.WorkItemsByID[workID]; ok {
		facts.KnownWork, facts.WorkName = true, item.DisplayName
	}
	visits := 0
	for dispatchID := range selected {
		visits++
		association, ok := index.associations[dispatchID]
		if !ok || association.WorkerSessionID == "" {
			continue
		}
		association.Model = cloneWorkerFactString(association.Model)
		association.ReasoningEffort = cloneWorkerFactString(association.ReasoningEffort)
		facts.Associations[dispatchID] = association
		facts.Requests[dispatchID] = cloneWorkerFactDispatch(index.requests[dispatchID])
		if cursor, ok := index.cursors[dispatchID]; ok {
			facts.StateCursors[dispatchID] = cursor
		}
		facts.ResponseTimes[dispatchID] = index.responseTimes[dispatchID]
		if cursor, ok := index.responseCursors[dispatchID]; ok {
			facts.ResponseCursors[dispatchID] = cursor
		}
		if interruption, ok := index.interruptions[dispatchID]; ok {
			facts.Interruptions[dispatchID] = cloneWorkerFactInterruption(interruption)
			facts.Requests[dispatchID] = index.interruptedRequest(dispatchID, facts.Requests[dispatchID])
		}
		if active, ok := state.ActiveDispatches[dispatchID]; ok {
			facts.World.ActiveDispatches[dispatchID] = cloneWorkerFactDispatch(active)
		}
		if position, ok := index.completions[dispatchID]; ok {
			facts.World.CompletedDispatches = append(facts.World.CompletedDispatches, interfaces.CloneFactoryWorldDispatchCompletion(state.CompletedDispatches[position]))
		}
		if position, ok := index.providers[dispatchID]; ok {
			facts.World.ProviderSessions = append(facts.World.ProviderSessions, interfaces.CloneFactoryWorldProviderSessionRecord(state.ProviderSessions[position]))
		}
	}
	return facts, visits
}

func (index *workerSessionWorkIndex) interruptedRequest(dispatchID string, request interfaces.FactoryWorldDispatch) interfaces.FactoryWorldDispatch {
	if _, completed := index.completions[dispatchID]; !completed && len(index.interruptedWorkIDs[dispatchID]) > 0 {
		request.WorkItemIDs = append([]string(nil), index.interruptedWorkIDs[dispatchID]...)
	}
	return request
}

func cloneWorkerFactString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneWorkerFactInterruption(value interfaces.DispatchInterruptedEventPayload) interfaces.DispatchInterruptedEventPayload {
	value.ProviderSessionRef = value.ProviderSessionRef.Clone()
	if value.CheckpointRef == nil {
		return value
	}
	checkpoint := *value.CheckpointRef
	checkpoint.Label = cloneWorkerFactString(checkpoint.Label)
	checkpoint.Summary = cloneWorkerFactString(checkpoint.Summary)
	if checkpoint.Timestamp != nil {
		timestamp := *checkpoint.Timestamp
		checkpoint.Timestamp = &timestamp
	}
	if checkpoint.ArtifactRef != nil {
		artifact := *checkpoint.ArtifactRef
		artifact.ContentHash = cloneWorkerFactString(artifact.ContentHash)
		if artifact.SizeBytes != nil {
			size := *artifact.SizeBytes
			artifact.SizeBytes = &size
		}
		checkpoint.ArtifactRef = &artifact
	}
	value.CheckpointRef = &checkpoint
	return value
}

func cloneWorkerFactDispatch(value interfaces.FactoryWorldDispatch) interfaces.FactoryWorldDispatch {
	value.WorkItemIDs = append([]string(nil), value.WorkItemIDs...)
	value.Inputs = interfaces.CloneWorkstationInputs(value.Inputs)
	value.Resources = append([]interfaces.FactoryResourceUnit(nil), value.Resources...)
	value.TraceIDs = append([]string(nil), value.TraceIDs...)
	value.PreviousChainingTraceIDs = append([]string(nil), value.PreviousChainingTraceIDs...)
	value.ExpectedArtifactContext = value.ExpectedArtifactContext.Clone()
	return value
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
