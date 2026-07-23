// backendsizecheck:ignore-file checkpoint resume validation, projection reconciliation, and persistence remain co-located on the runtime resume seam.
// pkgmaintcheck:ignore-file-lines checkpoint resume validation, projection reconciliation, and persistence remain co-located on the runtime resume seam.
package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"os"
	"strings"
	"time"
)

// ResumeInterruptedSession reconstructs one interrupted checkpointed session from
// persisted checkpoint summaries plus durable session state and continues execution.
func (s *JavaScriptRuntimeService) ResumeInterruptedSession(
	ctx context.Context,
	sessionID string,
	req ResumeSessionRequest,
) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return AsyncStartResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return AsyncStartResult{}, err
	}
	if strings.TrimSpace(req.RequestID) == "" {
		return AsyncStartResult{}, NewValidationError("requestId", "requestId is required")
	}

	state, err := s.loadResumeSessionState(id)
	if err != nil {
		return AsyncStartResult{}, err
	}
	if err := s.validateResumeSessionState(id, state); err != nil {
		return AsyncStartResult{}, err
	}

	s.mu.Lock()
	if existing, ok := s.sessions[id]; ok && existing.runCancel != nil {
		s.mu.Unlock()
		return AsyncStartResult{}, &ControlError{
			Operation: LifecycleControlResume,
			Outcome:   LifecycleControlOutcomeInvalidState,
			Status:    existing.session.Status,
			Message:   "session is already active in this runtime instance",
			Links:     LifecycleControlLinksForSession(id, true),
		}
	}
	resumingAt := s.now()
	state.session.Status = LifecycleStatusResuming
	state.result.SessionStatus = LifecycleStatusResuming
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	state.session.Lifecycle.ResumedAt = &resumingAt
	resumed := cloneRuntimeSessionState(&state)
	resumed.events = rebuildRuntimeSessionCanonicalEvents(&resumed)
	s.sessions[id] = &resumed
	s.mu.Unlock()

	normalized := *state.startRequest
	resolved := state.resolvedSource
	sourceContent := state.sourceContent
	policyResolution := workflowresult.JavaScriptPolicyResolution{
		Policy: workflowPolicyFromSessionPolicy(state.session.Policy),
		Hash:   strings.TrimSpace(state.session.Policy.EffectiveHash),
	}

	runCtx, runCancel := workflowRunContext(context.Background(), policyResolution.Policy)
	s.mu.Lock()
	if active, ok := s.sessions[id]; ok {
		active.runCancel = runCancel
	}
	s.mu.Unlock()

	go s.runResumedAsyncSession(
		runCtx,
		id,
		normalized,
		resolved,
		sourceContent,
		policyResolution,
		state.checkpointSummary,
		state.runtimeRecords,
		resumingAt,
	)

	snapshot, err := s.snapshotSessionState(id)
	if err != nil {
		return AsyncStartResult{}, err
	}
	return s.asyncStartFromState(snapshot), nil
}

func (s *JavaScriptRuntimeService) loadResumeSessionState(sessionID string) (runtimeSessionState, error) {
	s.mu.RLock()
	if state, ok := s.sessions[sessionID]; ok {
		cloned := cloneRuntimeSessionState(state)
		s.mu.RUnlock()
		return cloned, nil
	}
	persistence := s.persistence
	s.mu.RUnlock()

	if persistence == nil {
		return runtimeSessionState{}, ErrSessionNotFound
	}

	snapshot, err := persistence.Load(sessionID)
	if err != nil {
		if os.IsNotExist(err) {
			return runtimeSessionState{}, ErrSessionNotFound
		}
		return runtimeSessionState{}, &ResumeError{
			Outcome:   ResumeOutcomeCorruptedPersistence,
			SessionID: sessionID,
			Message:   "persisted session snapshot could not be read",
		}
	}
	var persisted PersistedRuntimeSessionState
	if err := json.Unmarshal(snapshot, &persisted); err != nil {
		return runtimeSessionState{}, &ResumeError{
			Outcome:   ResumeOutcomeCorruptedPersistence,
			SessionID: sessionID,
			Message:   "persisted session snapshot is corrupted and cannot be resumed",
		}
	}
	loaded := runtimeStateFromPersistedSnapshot(persisted)

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionID]; ok {
		return cloneRuntimeSessionState(existing), nil
	}
	cached := loaded
	s.sessions[sessionID] = &cached
	return cloneRuntimeSessionState(&cached), nil
}

func (s *JavaScriptRuntimeService) validateResumeSessionState(
	sessionID string,
	state runtimeSessionState,
) error {
	if state.session.Status != LifecycleStatusInterrupted {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Status:    state.session.Status,
			SessionID: sessionID,
			Message:   "session is not interrupted and cannot be resumed from checkpoint summaries",
		}
	}
	if state.checkpointSummary == nil {
		return &ResumeError{
			Outcome:   ResumeOutcomeMissingCheckpoint,
			Field:     "checkpointSummary",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary is required to resume an interrupted session",
		}
	}
	if err := validateCheckpointSummaryForResume(state.checkpointSummary, sessionID); err != nil {
		return err
	}
	if err := validateDurableResumeFacts(s.checkpointSummaries, sessionID, state); err != nil {
		return err
	}
	if state.startRequest == nil || strings.TrimSpace(state.sourceContent) == "" {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Field:     "session",
			SessionID: sessionID,
			Message:   "persisted start metadata is required to resume an interrupted session",
		}
	}
	return nil
}

// validateDurableResumeFacts reconciles the checkpoint skip-list with the
// durable dispatch, artifact, and canonical event history before execution can
// contact a child provider.
// pkgmaintcheck:ignore-cyclomatic-complexity each branch rejects one independently corrupted durable resume fact before provider IO.
func validateDurableResumeFacts(
	summaries workflowresult.JavaScriptCheckpointSummaries,
	sessionID string,
	state runtimeSessionState,
) error {
	if summaries == nil {
		return invalidDurableResumeFact(
			sessionID,
			"checkpointSummary",
			"Factory Runtime checkpoint summaries are unavailable",
		)
	}
	derived := summaries.Latest(workflowresult.JavaScriptCheckpointSummaryInput{
		SessionID: sessionID, Records: state.runtimeRecords,
	})
	if derived == nil || derived.CheckpointID != state.checkpointSummary.CheckpointID {
		return invalidDurableResumeFact(sessionID, "checkpointSummary.checkpointId", "persisted checkpoint does not match durable runtime history")
	}
	completed := stringSet(derived.CompletedDispatchIDs)
	for _, dispatchID := range state.checkpointSummary.CompletedDispatchIDs {
		if _, ok := completed[dispatchID]; !ok {
			return invalidDurableResumeFact(sessionID, "checkpointSummary.completedDispatchIds", "persisted checkpoint references a dispatch that is not durably completed")
		}
	}

	artifacts := stringSet(derived.ArtifactIDs)
	for _, artifactID := range state.checkpointSummary.ArtifactIDs {
		if _, ok := artifacts[artifactID]; !ok {
			return invalidDurableResumeFact(sessionID, "checkpointSummary.artifactIds", "persisted checkpoint references an artifact that is not durable")
		}
	}

	if len(state.events) == 0 {
		return invalidDurableResumeFact(sessionID, "events", "persisted canonical event history is required for resume")
	}
	lastSequence := 0
	lastType := ""
	for _, raw := range state.events {
		var event canonicalFactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return invalidDurableResumeFact(sessionID, "events", "persisted canonical event history is malformed")
		}
		if event.Context.Sequence <= lastSequence {
			return invalidDurableResumeFact(sessionID, "events.sequence", fmt.Sprintf("persisted canonical event cursor regressed from %d (%s) to %d (%s)", lastSequence, lastType, event.Context.Sequence, event.Type))
		}
		lastSequence = event.Context.Sequence
		lastType = event.Type
		if eventSessionID := stringValuePtr(event.Context.SessionID); eventSessionID != "" && eventSessionID != sessionID {
			return invalidDurableResumeFact(sessionID, "events.sessionId", "persisted canonical event belongs to a different Factory Session")
		}
	}
	replayed, _, err := ReplaySessionProjection(state.events)
	if err != nil || replayed.SessionID != sessionID || replayed.Status != state.session.Status {
		return invalidDurableResumeFact(sessionID, "events", "persisted canonical event projection does not match the durable session")
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func invalidDurableResumeFact(sessionID, field, message string) error {
	return &ResumeError{
		Outcome: ResumeOutcomeInvalidState, Field: field,
		SessionID: sessionID, Message: message,
	}
}

func validateCheckpointSummaryForResume(summary *workflowresult.JavaScriptCheckpointSummary, sessionID string) error {
	if summary == nil {
		return &ResumeError{
			Outcome:   ResumeOutcomeMissingCheckpoint,
			Field:     "checkpointSummary",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary is required to resume an interrupted session",
		}
	}
	if kind := strings.TrimSpace(summary.Kind); kind != "" && kind != workflowresult.JavaScriptCheckpointSummaryKind {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Field:     "checkpointSummary.kind",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary has an invalid kind",
		}
	}
	if summary.SchemaVersion != 0 && summary.SchemaVersion != workflowresult.JavaScriptCheckpointSummarySchemaVersion {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Field:     "checkpointSummary.schemaVersion",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary has an unsupported schema version",
		}
	}
	if strings.TrimSpace(summary.CheckpointID) == "" {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Field:     "checkpointSummary.checkpointId",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary is missing checkpointId",
		}
	}
	if persistedSessionID := strings.TrimSpace(summary.SessionID); persistedSessionID != "" && persistedSessionID != sessionID {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Field:     "checkpointSummary.sessionId",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary sessionId does not match the interrupted session",
		}
	}
	if strings.TrimSpace(summary.ResumeStrategy) != workflowresult.JavaScriptResumeStrategy {
		return &ResumeError{
			Outcome:   ResumeOutcomeInvalidState,
			Field:     "checkpointSummary.resumeStrategy",
			SessionID: sessionID,
			Message:   "persisted checkpoint summary is not approved for resume",
		}
	}
	return nil
}

func (s *JavaScriptRuntimeService) runResumedAsyncSession(
	runCtx context.Context,
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowresult.JavaScriptPolicyResolution,
	checkpointSummary *workflowresult.JavaScriptCheckpointSummary,
	priorRecords []workflowresult.JavaScriptRuntimeRecord,
	startedAt time.Time,
) {
	defer func() {
		s.mu.Lock()
		if state, ok := s.sessions[sessionID]; ok {
			state.runCancel = nil
		}
		s.mu.Unlock()
	}()

	resumeContext := s.workflowRuntime.ResumeContext(
		workflowresult.JavaScriptCompletedCheckpointSummary{
			CompletedDispatchIDs: checkpointSummary.CompletedDispatchIDs,
			CheckpointState:      checkpointSummary.CheckpointState,
		},
		priorRecords,
	)
	outcome, err := s.invokeWorkflowRuntimeWithResume(
		runCtx,
		normalized,
		resolved,
		sourceContent,
		policyResolution,
		sessionID,
		&resumeContext,
	)

	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	mergedRecords := mergeRuntimeRecords(priorRecords, outcome.Records)
	outcome.Records = mergedRecords

	if err != nil {
		failureOutcome := workflowresult.JavaScriptRuntimeOutcome{
			OK: false,
			Failure: workflowresult.JavaScriptRuntimeFailure{
				Code:    workflowresult.JavaScriptRuntimeCodeScriptError,
				Message: err.Error(),
			},
			Records: outcome.Records,
		}
		terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, failureOutcome, startedAt)
		candidate := s.buildTerminalRuntimeCandidate(state, terminal, failureOutcome, startedAt)
		candidate.runtimeRecords = cloneRuntimeRecords(mergedRecords)
		candidate.checkpointSummary = cloneCheckpointSummary(checkpointSummary)
		s.publishAsyncTerminalCandidate(state, candidate, normalized, resolved, policyResolution, startedAt)
		return
	}

	terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt)
	candidate := s.buildTerminalRuntimeCandidate(state, terminal, outcome, startedAt)
	candidate.runtimeRecords = cloneRuntimeRecords(mergedRecords)
	candidate.checkpointSummary = cloneCheckpointSummary(checkpointSummary)
	if candidate.checkpointSummary == nil {
		candidate.checkpointSummary = latestCheckpointSummaryFromRuntime(
			s.checkpointSummaries,
			sessionID,
			&candidate,
			candidate.runtimeRecords,
		)
	}
	s.publishAsyncTerminalCandidate(state, candidate, normalized, resolved, policyResolution, startedAt)
}

func (s *JavaScriptRuntimeService) invokeWorkflowRuntimeWithResume(
	ctx context.Context,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowresult.JavaScriptPolicyResolution,
	sessionID string,
	resume *workflowresult.JavaScriptResumeContext,
) (workflowresult.JavaScriptRuntimeOutcome, error) {
	argsJSON, err := marshalStartArgs(normalized.Args)
	if err != nil {
		return workflowresult.JavaScriptRuntimeOutcome{}, err
	}
	return s.workflowRuntime.Run(ctx, workflowresult.JavaScriptRuntimeRequest{
		Source:     sourceContent,
		SourceRef:  resolved.SourceRef,
		SessionID:  sessionID,
		Args:       argsJSON,
		ArgsSchema: resolved.ArgsSchema,
		Metadata:   workflowMetadataFromResolved(resolved, normalized),
		Policy:     policyResolution.Policy,
		Resume:     resume,
	}, s.childExecutorHooks(resolveChildExecutorMode(s.childExecutorMode, normalized), sessionID))
}

func mergeRuntimeRecords(existing, resumed []workflowresult.JavaScriptRuntimeRecord) []workflowresult.JavaScriptRuntimeRecord {
	if len(existing) == 0 {
		return cloneRuntimeRecords(resumed)
	}
	if len(resumed) == 0 {
		return cloneRuntimeRecords(existing)
	}
	merged := make([]workflowresult.JavaScriptRuntimeRecord, 0, len(existing)+len(resumed))
	merged = append(merged, cloneRuntimeRecords(existing)...)
	merged = append(merged, cloneRuntimeRecords(resumed)...)
	return merged
}

func latestCheckpointSummaryFromRuntime(
	summaries workflowresult.JavaScriptCheckpointSummaries,
	sessionID string,
	state *runtimeSessionState,
	records []workflowresult.JavaScriptRuntimeRecord,
) *workflowresult.JavaScriptCheckpointSummary {
	if summaries == nil || state == nil {
		return nil
	}
	return summaries.Latest(workflowresult.JavaScriptCheckpointSummaryInput{
		SessionID:  sessionID,
		Phase:      state.session.Phase,
		SourceHash: state.session.SourceHash,
		PolicyHash: state.session.Policy.EffectiveHash,
		Records:    records,
	})
}

func cloneCheckpointSummary(summary *workflowresult.JavaScriptCheckpointSummary) *workflowresult.JavaScriptCheckpointSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	if len(summary.CompletedDispatchIDs) > 0 {
		cloned.CompletedDispatchIDs = append([]string(nil), summary.CompletedDispatchIDs...)
	}
	if len(summary.PendingDispatchIDs) > 0 {
		cloned.PendingDispatchIDs = append([]string(nil), summary.PendingDispatchIDs...)
	}
	if len(summary.ArtifactIDs) > 0 {
		cloned.ArtifactIDs = append([]string(nil), summary.ArtifactIDs...)
	}
	if len(summary.CheckpointState) > 0 {
		cloned.CheckpointState = make(map[string]any, len(summary.CheckpointState))
		for key, value := range summary.CheckpointState {
			cloned.CheckpointState[key] = value
		}
	}
	return &cloned
}

func workflowPolicyFromSessionPolicy(policy PolicyProjection) workflowresult.JavaScriptPolicy {
	if len(policy.Effective) == 0 {
		return workflowresult.DefaultJavaScriptPolicy()
	}
	encoded, err := json.Marshal(policy.Effective)
	if err != nil {
		return workflowresult.DefaultJavaScriptPolicy()
	}
	var effective workflowresult.JavaScriptPolicy
	if err := json.Unmarshal(encoded, &effective); err != nil {
		return workflowresult.DefaultJavaScriptPolicy()
	}
	return effective
}
func applyRuntimeSessionFields(target *runtimeSessionState, source runtimeSessionState) {
	preservedResume := preserveRuntimeResumeState(*target)
	target.session = source.session
	target.result = source.result
	target.dispatches = cloneDispatchSummaries(source.dispatches)
	target.dispatchJavaScript = cloneDispatchJavaScriptProjections(source.dispatchJavaScript)
	target.dispatchStatusTransitions = cloneDispatchStatusTransitions(source.dispatchStatusTransitions)
	target.artifacts = cloneArtifactSummaries(source.artifacts)
	target.events = source.events
	restoreRuntimeResumeState(target, preservedResume)
}

type preservedRuntimeResumeState struct {
	runtimeRecords    []workflowresult.JavaScriptRuntimeRecord
	checkpointSummary *workflowresult.JavaScriptCheckpointSummary
	startRequest      *StartRequest
	resolvedSource    ResolvedSource
	sourceContent     string
	lifecycle         *LifecycleTimestamps
}

func preserveRuntimeResumeState(state runtimeSessionState) preservedRuntimeResumeState {
	return preservedRuntimeResumeState{
		runtimeRecords:    cloneRuntimeRecords(state.runtimeRecords),
		checkpointSummary: cloneCheckpointSummary(state.checkpointSummary),
		startRequest:      cloneStartRequestPtr(state.startRequest),
		resolvedSource:    state.resolvedSource,
		sourceContent:     state.sourceContent,
		lifecycle:         cloneLifecycleTimestamps(state.session.Lifecycle),
	}
}

func restoreRuntimeResumeState(state *runtimeSessionState, preserved preservedRuntimeResumeState) {
	if state == nil {
		return
	}
	state.runtimeRecords = mergeRuntimeRecords(preserved.runtimeRecords, state.runtimeRecords)
	if preserved.checkpointSummary != nil {
		state.checkpointSummary = cloneCheckpointSummary(preserved.checkpointSummary)
	}
	if preserved.startRequest != nil {
		state.startRequest = cloneStartRequestPtr(preserved.startRequest)
	}
	if preserved.resolvedSource.SourceRef != "" {
		state.resolvedSource = preserved.resolvedSource
	}
	if preserved.sourceContent != "" {
		state.sourceContent = preserved.sourceContent
	}
	mergeResumeLifecycleLineage(state, preserved.lifecycle)
}

func cloneLifecycleTimestamps(lifecycle *LifecycleTimestamps) *LifecycleTimestamps {
	if lifecycle == nil {
		return nil
	}
	cloned := &LifecycleTimestamps{}
	if lifecycle.QueuedAt != nil {
		cloned.QueuedAt = timePtr(lifecycle.QueuedAt.UTC())
	}
	if lifecycle.AwaitingApprovalAt != nil {
		cloned.AwaitingApprovalAt = timePtr(lifecycle.AwaitingApprovalAt.UTC())
	}
	if lifecycle.StartedAt != nil {
		cloned.StartedAt = timePtr(lifecycle.StartedAt.UTC())
	}
	if lifecycle.PausedAt != nil {
		cloned.PausedAt = timePtr(lifecycle.PausedAt.UTC())
	}
	if lifecycle.ResumedAt != nil {
		cloned.ResumedAt = timePtr(lifecycle.ResumedAt.UTC())
	}
	if lifecycle.FinishedAt != nil {
		cloned.FinishedAt = timePtr(lifecycle.FinishedAt.UTC())
	}
	if lifecycle.InterruptedAt != nil {
		cloned.InterruptedAt = timePtr(lifecycle.InterruptedAt.UTC())
	}
	if lifecycle.TerminatedAt != nil {
		cloned.TerminatedAt = timePtr(lifecycle.TerminatedAt.UTC())
	}
	if lifecycle.UpdatedAt != nil {
		cloned.UpdatedAt = timePtr(lifecycle.UpdatedAt.UTC())
	}
	return cloned
}

func mergeResumeLifecycleLineage(state *runtimeSessionState, preserved *LifecycleTimestamps) {
	if state == nil || preserved == nil {
		return
	}
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	if preserved.StartedAt != nil {
		state.session.Lifecycle.StartedAt = timePtr(preserved.StartedAt.UTC())
	}
	if preserved.InterruptedAt != nil {
		state.session.Lifecycle.InterruptedAt = timePtr(preserved.InterruptedAt.UTC())
	}
	if preserved.ResumedAt != nil {
		state.session.Lifecycle.ResumedAt = timePtr(preserved.ResumedAt.UTC())
	}
	if preserved.PausedAt != nil {
		state.session.Lifecycle.PausedAt = timePtr(preserved.PausedAt.UTC())
	}
}

func applyTerminalRuntimeProjection(
	state *runtimeSessionState,
	terminal runtimeSessionState,
	outcome workflowresult.JavaScriptRuntimeOutcome,
) {
	priorSession := cloneSessionRead(state.session)
	priorResult := cloneResultRead(state.result)
	priorStatus := state.session.Status
	preserved := snapshotInterruptedDispatches(state)
	preservedEvents := extractDispatchInterruptedEvents(state.events)
	applyRuntimeSessionFields(state, terminal)
	if len(preserved) == 0 {
		return
	}
	if outcome.OK && priorStatus == LifecycleStatusResuming {
		state.session.StaleLease = false
		state.events = mergePreservedDispatchInterruptedEvents(
			rebuildRuntimeSessionCanonicalEvents(state),
			preservedEvents,
		)
		return
	}
	restoreInterruptedDispatchResultSuppression(state, preserved)
	finalizeInterruptedTerminalSession(state, priorSession, priorResult)
	state.events = mergePreservedDispatchInterruptedEvents(
		BuildCanonicalRuntimeSessionEvents(
			state.session,
			state.result,
			runtimeDispatchEventInputFromState(state),
		),
		preservedEvents,
	)
}

func applyRuntimeExecutionRecordProjection(
	state *runtimeSessionState,
	sessionID string,
	records []workflowresult.JavaScriptRuntimeRecord,
	finishedAt time.Time,
) {
	recordProjection := ProjectRuntimeExecutionRecords(sessionID, records, finishedAt)
	if recordProjection.Phase != "" {
		state.session.Phase = recordProjection.Phase
	}
	state.session.PhaseSummaries = append([]PhaseSummary(nil), recordProjection.PhaseSummaries...)
	checkpointPhase := ""
	for _, record := range records {
		if record.Kind == workflowresult.JavaScriptRecordKindPhase && record.Phase != nil {
			checkpointPhase = strings.TrimSpace(record.Phase.Name)
		}
		if record.Kind == workflowresult.JavaScriptRecordKindCheckpoint && record.Checkpoint != nil {
			state.session.LatestCheckpoint = &CheckpointRef{
				ID: strings.TrimSpace(record.Checkpoint.ID), Label: strings.TrimSpace(record.Checkpoint.Label), Phase: checkpointPhase,
			}
		}
	}
	state.dispatches = cloneDispatchSummaries(recordProjection.Dispatches)
	state.dispatchJavaScript = cloneDispatchJavaScriptProjections(recordProjection.DispatchJavaScript)
	state.dispatchStatusTransitions = cloneDispatchStatusTransitions(recordProjection.DispatchStatusTransitions)
	state.artifacts = cloneArtifactSummaries(recordProjection.Artifacts)
	state.session.Progress = &recordProjection.Progress
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.session.ArtifactCount = len(state.session.ArtifactRefs)
}

func applyRuntimeSuccessProjection(
	state *runtimeSessionState,
	sessionID string,
	outcome workflowresult.JavaScriptRuntimeOutcome,
	finishedAt time.Time,
) {
	applyRuntimeExecutionRecordProjection(state, sessionID, outcome.Records, finishedAt)

	projected, resultSummary, err := projectRuntimeSuccessResult(sessionID, outcome.Value, state.artifacts)
	if err != nil {
		state.session.Status = LifecycleStatusFailed
		state.session.Failure = &FailureSummary{
			Reason:  "WORKFLOW_RUNTIME_INVALID_RESULT",
			Message: err.Error(),
		}
		state.session.ResultSummary = &ResultSummary{
			ResultStatus: string(ResultStatusUnavailable),
		}
		state.result = ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusUnavailable,
			SessionStatus: LifecycleStatusFailed,
			Failure:       cloneFailureSummary(state.session.Failure),
			Availability:  defaultUnavailableAvailability(),
		}
		return
	}
	state.session.Status = LifecycleStatusSucceeded
	state.session.ResultSummary = resultSummary
	state.result = projected
}

func projectRuntimeSuccessResult(
	sessionID string,
	value workflowresult.TypedValue,
	artifacts []ArtifactSummary,
) (ResultReadResult, *ResultSummary, error) {
	parts, validation := workflowresult.ProjectPrimaryResult(sessionID, value, artifactStatesFromSummaries(artifacts))
	if validation.HasIssues() {
		return ResultReadResult{}, nil, fmt.Errorf("project primary result: %v", validation.Issues)
	}

	primaryJSON := workContentJSONFromParts(parts)
	result := ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: primaryJSON,
		ArtifactIDs:   artifactIDsFromSummaries(artifacts),
	}
	summary := &ResultSummary{
		ResultStatus: string(ResultStatusFinal),
		Summary:      resultSummaryTextFromParts(parts),
	}
	return result, summary, nil
}

func finalizeInterruptedTerminalSession(
	state *runtimeSessionState,
	priorSession SessionReadResult,
	priorResult ResultReadResult,
) {
	if state == nil {
		return
	}
	interruptedAt := interruptedTerminalTimestamp(state.session, priorSession)
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	if interruptedAt != nil {
		state.session.Lifecycle.InterruptedAt = timePtr(*interruptedAt)
		if state.session.Lifecycle.FinishedAt == nil {
			state.session.Lifecycle.FinishedAt = timePtr(*interruptedAt)
		}
	}
	state.session.Status = LifecycleStatusInterrupted
	state.session.Failure = nil

	canonicalStatus := canonicalResultStatus(priorResult, priorSession)
	switch canonicalStatus {
	case ResultStatusPartial, ResultStatusFailedWithPartial:
		if priorSession.ResultSummary != nil {
			summary := *priorSession.ResultSummary
			state.session.ResultSummary = &summary
		} else {
			state.session.ResultSummary = nil
		}
		if state.session.ResultSummary == nil {
			state.session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusPartial)}
		} else {
			state.session.ResultSummary.ResultStatus = string(ResultStatusPartial)
		}
		state.result = cloneResultRead(priorResult)
		state.result.ResultStatus = ResultStatusPartial
		state.result.SessionStatus = LifecycleStatusInterrupted
		state.result.Mode = ResultModeFinal
		state.result.Availability = nil
	case ResultStatusFinal:
		fallthrough
	case ResultStatusNotReady, ResultStatusUnavailable:
		fallthrough
	default:
		state.session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusUnavailable)}
		state.result = ResultReadResult{
			SessionID:     state.session.SessionID,
			ResultStatus:  ResultStatusUnavailable,
			SessionStatus: LifecycleStatusInterrupted,
			Mode:          ResultModeFinal,
			Availability: &ResultAvailabilityDetail{
				Reason:    "SESSION_INTERRUPTED",
				Message:   "Session was interrupted before a final result was available.",
				Retryable: false,
			},
		}
	}
}

func interruptedTerminalTimestamp(session, prior SessionReadResult) *time.Time {
	if session.Lifecycle != nil {
		if session.Lifecycle.InterruptedAt != nil {
			return timePtr(session.Lifecycle.InterruptedAt.UTC())
		}
		if session.Lifecycle.FinishedAt != nil {
			return timePtr(session.Lifecycle.FinishedAt.UTC())
		}
		if session.Lifecycle.UpdatedAt != nil {
			return timePtr(session.Lifecycle.UpdatedAt.UTC())
		}
	}
	if prior.Lifecycle != nil {
		if prior.Lifecycle.InterruptedAt != nil {
			return timePtr(prior.Lifecycle.InterruptedAt.UTC())
		}
		if prior.Lifecycle.UpdatedAt != nil {
			return timePtr(prior.Lifecycle.UpdatedAt.UTC())
		}
		if prior.Lifecycle.StartedAt != nil {
			return timePtr(prior.Lifecycle.StartedAt.UTC())
		}
	}
	return nil
}

// PersistedRuntimeSessionState is a JSON-serializable durable runtime session snapshot
// used to reload terminal or recoverable JavaScript runtime sessions across CLI invocations.
type PersistedRuntimeSessionState struct {
	Session                   SessionReadResult
	Result                    ResultReadResult
	Dispatches                []DispatchSummary
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	DispatchStatusTransitions map[string][]DispatchStatus
	Artifacts                 []ArtifactSummary
	Events                    []json.RawMessage
	RuntimeRecords            []workflowresult.JavaScriptRuntimeRecord
	// Records is the tagged, lossless durable history. Events and RuntimeRecords
	// remain populated for compatibility with snapshots written before this union.
	Records           []DurableSessionRecord
	CheckpointSummary *workflowresult.JavaScriptCheckpointSummary
	StartRequest      *StartRequest
	ResolvedSource    ResolvedSource
	SourceContent     string
}

func persistedSnapshotFromRuntimeState(state runtimeSessionState) PersistedRuntimeSessionState {
	snapshot := PersistedRuntimeSessionState{
		Session:           cloneSessionRead(state.session),
		Result:            cloneResultRead(state.result),
		Dispatches:        cloneDispatchSummaries(state.dispatches),
		Artifacts:         cloneArtifactSummaries(state.artifacts),
		RuntimeRecords:    cloneRuntimeRecords(state.runtimeRecords),
		Records:           durableRecordsFromRuntimeState(state),
		CheckpointSummary: cloneCheckpointSummary(state.checkpointSummary),
		StartRequest:      cloneStartRequestPtr(state.startRequest),
		ResolvedSource:    state.resolvedSource,
		SourceContent:     state.sourceContent,
	}
	if len(state.dispatchJavaScript) > 0 {
		snapshot.DispatchJavaScript = cloneDispatchJavaScriptProjections(state.dispatchJavaScript)
	}
	if len(state.dispatchStatusTransitions) > 0 {
		snapshot.DispatchStatusTransitions = cloneDispatchStatusTransitions(state.dispatchStatusTransitions)
	}
	if len(state.events) > 0 {
		snapshot.Events = make([]json.RawMessage, len(state.events))
		for i, event := range state.events {
			snapshot.Events[i] = append(json.RawMessage(nil), event...)
		}
	}
	return snapshot
}

func runtimeStateFromPersistedSnapshot(snapshot PersistedRuntimeSessionState) runtimeSessionState {
	events, runtimeRecords, petriMutations := runtimeHistoryFromPersistedSnapshot(snapshot)
	state := runtimeSessionState{
		session:           cloneSessionRead(snapshot.Session),
		result:            cloneResultRead(snapshot.Result),
		dispatches:        cloneDispatchSummaries(snapshot.Dispatches),
		artifacts:         cloneArtifactSummaries(snapshot.Artifacts),
		runtimeRecords:    cloneRuntimeRecords(runtimeRecords),
		petriMutations:    clonePetriMutations(petriMutations),
		checkpointSummary: cloneCheckpointSummary(snapshot.CheckpointSummary),
		startRequest:      cloneStartRequestPtr(snapshot.StartRequest),
		resolvedSource:    snapshot.ResolvedSource,
		sourceContent:     snapshot.SourceContent,
	}
	if len(snapshot.DispatchJavaScript) > 0 {
		state.dispatchJavaScript = cloneDispatchJavaScriptProjections(snapshot.DispatchJavaScript)
	}
	if len(snapshot.DispatchStatusTransitions) > 0 {
		state.dispatchStatusTransitions = cloneDispatchStatusTransitions(snapshot.DispatchStatusTransitions)
	}
	if len(events) > 0 {
		state.events = make([]json.RawMessage, len(events))
		for i, event := range events {
			state.events[i] = append(json.RawMessage(nil), event...)
		}
	}
	return state
}

func (s *JavaScriptRuntimeService) persistTerminalSessionState(state runtimeSessionState) error {
	return s.persistSessionSnapshot(state)
}

func (s *JavaScriptRuntimeService) persistSessionSnapshot(state runtimeSessionState) error {
	if s.persistence == nil {
		return nil
	}
	sessionID := strings.TrimSpace(state.session.SessionID)
	if sessionID == "" {
		return nil
	}
	if !shouldPersistSessionSnapshot(state) {
		return nil
	}
	snapshot := persistedSnapshotFromRuntimeState(state)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal durable session snapshot: %w", err)
	}
	if err := s.persistence.Save(sessionID, encoded); err != nil {
		return fmt.Errorf("persist durable session snapshot: %w", err)
	}
	return nil
}

func shouldPersistSessionSnapshot(state runtimeSessionState) bool {
	if len(state.petriMutations) > 0 {
		return true
	}
	if state.session.Status == LifecycleStatusPaused {
		return true
	}
	if IsTerminalLifecycleStatus(state.session.Status) {
		return true
	}
	if state.session.Status == LifecycleStatusInterrupted && state.checkpointSummary != nil {
		return true
	}
	return false
}

func cloneStartRequest(req StartRequest) *StartRequest {
	cloned := req
	cloned.Source = cloneStartSource(req.Source)
	cloned.Args = cloneArgs(req.Args)
	cloned.RequestedPolicy = cloneArgs(req.RequestedPolicy)
	if req.Orchestrator != nil {
		orchestrator := *req.Orchestrator
		cloned.Orchestrator = &orchestrator
	}
	if req.Runtime != nil {
		runtime := *req.Runtime
		cloned.Runtime = &runtime
	}
	if req.Wait != nil {
		wait := *req.Wait
		cloned.Wait = &wait
	}
	return &cloned
}

func cloneStartRequestPtr(req *StartRequest) *StartRequest {
	if req == nil {
		return nil
	}
	return cloneStartRequest(*req)
}

func cloneStartSource(source Source) Source {
	cloned := source
	if source.InlineWorkflow != nil {
		inline := *source.InlineWorkflow
		inline.Metadata = cloneStringStringMap(source.InlineWorkflow.Metadata)
		inline.Agents = cloneJavaScriptAgents(source.InlineWorkflow.Agents)
		inline.ArgsSchema = append(json.RawMessage(nil), source.InlineWorkflow.ArgsSchema...)
		inline.DefaultPolicy = append(json.RawMessage(nil), source.InlineWorkflow.DefaultPolicy...)
		cloned.InlineWorkflow = &inline
	}
	if len(source.FactoryInline) > 0 {
		cloned.FactoryInline = append(json.RawMessage(nil), source.FactoryInline...)
	}
	return cloned
}

func cloneStringStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (s *JavaScriptRuntimeService) resumeInterruptedSessionViaLifecycleControl(
	ctx context.Context,
	sessionID string,
	req ControlRequest,
) (LifecycleControlResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, true, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return LifecycleControlResult{}, true, err
	}
	control, err := NormalizeControlRequest(req)
	if err != nil {
		return LifecycleControlResult{}, true, err
	}

	status, statusErr := s.peekSessionStatusForResume(id)
	if statusErr != nil {
		if errors.Is(statusErr, ErrSessionNotFound) {
			return LifecycleControlResult{}, false, nil
		}
		return LifecycleControlResult{}, true, mapResumeFailureToControlError(id, statusErr)
	}
	if status != LifecycleStatusInterrupted {
		return LifecycleControlResult{}, false, nil
	}

	requestID := control.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("lifecycle-resume-%s", id)
	}
	started, resumeErr := s.ResumeInterruptedSession(ctx, id, ResumeSessionRequest{
		RequestID: requestID,
	})
	if resumeErr != nil {
		return LifecycleControlResult{}, true, mapResumeFailureToControlError(id, resumeErr)
	}
	return lifecycleControlResultFromInterruptedResume(id, started), true, nil
}

func (s *JavaScriptRuntimeService) peekSessionStatusForResume(sessionID string) (LifecycleStatus, error) {
	s.mu.RLock()
	if state, ok := s.sessions[sessionID]; ok {
		status := state.session.Status
		s.mu.RUnlock()
		return status, nil
	}
	persistence := s.persistence
	s.mu.RUnlock()

	if persistence == nil {
		return "", ErrSessionNotFound
	}
	state, err := s.loadResumeSessionState(sessionID)
	if err != nil {
		return "", err
	}
	return state.session.Status, nil
}

func lifecycleControlResultFromInterruptedResume(
	sessionID string,
	started AsyncStartResult,
) LifecycleControlResult {
	status := LifecycleStatus(strings.TrimSpace(started.Status))
	if status == "" {
		status = LifecycleStatusResuming
	}
	return LifecycleControlResult{
		SessionID: sessionID,
		Operation: LifecycleControlResume,
		Outcome:   LifecycleControlOutcomeAccepted,
		Status:    status,
		Links:     LifecycleControlLinksForSession(sessionID, true),
	}
}

func mapResumeFailureToControlError(sessionID string, err error) error {
	if err == nil {
		return nil
	}
	var controlErr *ControlError
	if errors.As(err, &controlErr) {
		return controlErr
	}
	var resumeErr *ResumeError
	if errors.As(err, &resumeErr) {
		status := resumeErr.Status
		if status == "" {
			status = LifecycleStatusInterrupted
		}
		return &ControlError{
			Operation: LifecycleControlResume,
			Outcome:   LifecycleControlOutcomeInvalidState,
			Status:    status,
			Message:   resumeErr.Error(),
			Links:     LifecycleControlLinksForSession(sessionID, true),
		}
	}
	return err
}
